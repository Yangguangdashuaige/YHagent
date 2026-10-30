// internal/ai/session.go
package ai

import (
	"log"
	"strings"
	"sync"
	"time"

	"github.com/volcengine/ark-runtime-go/arkruntime/model/chat"
)

const (
	// defaultMaxHistory 单个会话默认保留的最近消息条数（不含 system 消息），
	// 控制上下文长度，避免长对话 token 爆炸，适配游戏场景低内存占用。
	defaultMaxHistory = 20
	// sessionIdleTTL 会话空闲过期时间：超过该时长没有新消息的会话会被后台清理，
	// 玩家下次提问时无感重建。2 小时覆盖一次连续游戏开黑的对话长度。
	sessionIdleTTL = 2 * time.Hour
	// sessionCleanupInterval 后台扫描过期会话的间隔
	sessionCleanupInterval = 10 * time.Minute
	// maxSessions 内存会话数硬上限：防止异常客户端不断换新 ID 导致 map 无限膨胀。
	// 超出后新建会话时淘汰一个空闲会话（正在问答中的会话受轮转锁保护不会被淘汰）。
	maxSessions = 5000
)

// Session 单个会话的完整对话上下文，同一 sessionID 的多轮提问共享同一份历史
type Session struct {
	mu sync.Mutex
	// turnMu 整轮对话（一次用户提问到最终回答落盘）级别的独占锁。
	// mu 只保护单次切片读写；同一 sessionID 的两个并发请求若交错追加，
	// tool_calls 与 tool 结果会错配（API 直接 400），因此整轮必须互斥。
	// 两把锁分离：整轮持锁期间仍会多次调用 mu 的 Lock/Unlock，Go 互斥锁不可重入。
	turnMu            sync.Mutex
	id                string
	messages          []chat.ChatCompletionRequestMessage
	maxKeep           int           // 除 system 消息外最多保留多少条
	lastActive        time.Time     // 最近活跃（内存空闲清理依据）
	compactBackoffTil time.Time     // 摘要压缩失败后的退避截止时间（期间跳过压缩）
	store             SessionStore  // 持久化后端；nil = 纯内存（未启用 SESSION_DB）
}

// persist 把当前完整历史落库（整轮结束时调用，失败只记日志不阻断对话）
func (s *Session) persist() {
	if s.store == nil {
		return
	}
	s.mu.Lock()
	msgs := make([]chat.ChatCompletionRequestMessage, len(s.messages))
	copy(msgs, s.messages)
	s.mu.Unlock()
	if err := s.store.Save(s.id, persistableMessages(msgs)); err != nil {
		log.Printf("[session] 会话 %s 落盘失败（本轮历史丢失风险）: %v", s.id, err)
	}
}

// clearPersisted 删除持久化记录（重置语义：内存与磁盘一起清空）
func (s *Session) clearPersisted() {
	if s.store == nil {
		return
	}
	if err := s.store.Delete(s.id); err != nil {
		log.Printf("[session] 会话 %s 持久化记录删除失败: %v", s.id, err)
	}
}

// newSessionFromStored 从落库历史恢复会话：system 消息重写为当前提示词（跨版本升级即时生效），
// 其余消息按原样恢复。stored 为空时等价于新会话。
func newSessionFromStored(id, systemPrompt string, stored []chat.ChatCompletionRequestMessage, store SessionStore) *Session {
	s := &Session{
		id:         id,
		maxKeep:    defaultMaxHistory,
		lastActive: time.Now(),
		store:      store,
	}
	s.messages = make([]chat.ChatCompletionRequestMessage, 0, len(stored)+1)
	s.messages = append(s.messages, newSystemMessage(systemPrompt))
	s.messages = append(s.messages, stored...)
	return s
}

// tryBeginTurn 尝试开始一整轮对话。已有一轮在跑时返回 false，
// 调用方应立即拒绝请求（快速失败），而不是排队——排队会让后一个请求
// 长时间挂起（单轮含工具轮询可能数分钟），SSE 客户端侧早已超时。
func (s *Session) tryBeginTurn() bool {
	return s.turnMu.TryLock()
}

// endTurn 结束整轮对话，释放轮转锁
func (s *Session) endTurn() {
	s.turnMu.Unlock()
}

// newSession 创建会话并写入 system 消息
func newSession(id, systemPrompt string) *Session {
	s := &Session{
		id:         id,
		maxKeep:    defaultMaxHistory,
		lastActive: time.Now(),
	}
	s.reset(systemPrompt)
	return s
}

// reset 清空历史并重新写入 system 消息
func (s *Session) reset(systemPrompt string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.messages = []chat.ChatCompletionRequestMessage{newSystemMessage(systemPrompt)}
	s.lastActive = time.Now()
}

// append 追加一条消息（调用方持有会话概念上的串行顺序，内部仍加锁保护）
func (s *Session) append(msg chat.ChatCompletionRequestMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = append(s.messages, msg)
	s.lastActive = time.Now()
}

// ID 返回会话标识（日志/压缩记录用）
func (s *Session) ID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.id
}

// compactBlocked 摘要压缩失败退避期内返回 true（避免反复白等超时）
func (s *Session) compactBlocked() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return time.Now().Before(s.compactBackoffTil)
}

// noteCompactResult 记录一次压缩结果：失败进入退避期，成功清除退避
func (s *Session) noteCompactResult(ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ok {
		s.compactBackoffTil = time.Time{}
	} else {
		s.compactBackoffTil = time.Now().Add(compactBackoff)
	}
}

// historyAfterSystem 返回不含首条 system 消息的历史副本（供 token 预算评估）
func (s *Session) historyAfterSystem() []chat.ChatCompletionRequestMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.messages) <= 1 {
		return nil
	}
	out := make([]chat.ChatCompletionRequestMessage, len(s.messages)-1)
	copy(out, s.messages[1:])
	return out
}

// replacePrefix 把 system 之后的前 k 条历史替换为一条摘要消息（压缩落盘动作）。
// 保留段至少一条消息；k 非法时静默忽略（compact 侧已保证边界，这里兜底）。
func (s *Session) replacePrefix(k int, summary chat.ChatCompletionRequestMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if k < 0 || k+1 > len(s.messages)-1 {
		return
	}
	kept := make([]chat.ChatCompletionRequestMessage, len(s.messages)-k-1)
	copy(kept, s.messages[k+1:])
	s.messages = append(s.messages[:1:1], summary)
	s.messages = append(s.messages, kept...)
	s.lastActive = time.Now()
}

// idleFor 返回会话已经空闲多久（距最后一条消息的时长）
func (s *Session) idleFor(now time.Time) time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return now.Sub(s.lastActive)
}

// popLast 移除最后一条消息，用于整轮调用失败时回滚刚追加的用户消息
func (s *Session) popLast() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.messages) > 1 {
		s.messages = s.messages[:len(s.messages)-1]
	}
}

// snapshot 返回发送给模型的消息窗口：
// system 消息始终保留，后面只取最近 maxKeep 条，避免历史无限增长。
func (s *Session) snapshot() []chat.ChatCompletionRequestMessage {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.messages) <= s.maxKeep+1 {
		out := make([]chat.ChatCompletionRequestMessage, len(s.messages))
		copy(out, s.messages)
		return out
	}
	// 保留第 0 条 system + 最近 maxKeep 条
	out := make([]chat.ChatCompletionRequestMessage, 0, s.maxKeep+1)
	out = append(out, s.messages[0])
	out = append(out, s.messages[len(s.messages)-s.maxKeep:]...)
	return out
}

// Manager 会话管理器，按 sessionID 维护多个独立会话（Wails 每个窗口/标签页一个ID即可）
type Manager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	store    SessionStore // 可选持久化后端；nil = 纯内存（重启即失忆的旧行为）
}

// NewSessionManager 初始化内存会话管理器，并启动后台 goroutine 定期清理空闲会话，
// 避免只增不减的 map 在长期运行后缓慢堆积。
func NewSessionManager() *Manager {
	m := &Manager{
		sessions: make(map[string]*Session),
	}
	go m.cleanupLoop()
	return m
}

// WithStore 挂载持久化后端（须在首个请求前调用），返回自身便于链式初始化
func (m *Manager) WithStore(store SessionStore) *Manager {
	m.store = store
	return m
}

// GetOrCreate 获取已有会话，不存在则用 systemPrompt 初始化一个新会话。
// 挂载了持久化后端时先尝试从库恢复（重启/淘汰后无感续聊）：
// 恢复的历史不含落库时的 system 消息——统一重写为当前提示词，升级即时生效。
func (m *Manager) GetOrCreate(sessionID, systemPrompt string) *Session {
	m.mu.RLock()
	s, ok := m.sessions[sessionID]
	m.mu.RUnlock()
	if ok {
		return s
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	// 双重检查，防止并发场景下重复创建
	if s, ok = m.sessions[sessionID]; ok {
		return s
	}
	if len(m.sessions) >= maxSessions {
		m.evictOneIdleLocked()
	}
	if m.store != nil {
		stored, ok, err := m.store.Load(sessionID)
		if err != nil {
			// 读库失败按新会话兜底：可用性优先，宁可丢历史也不让用户开不了对话
			log.Printf("[session] 会话 %s 恢复失败，按新会话处理: %v", sessionID, err)
		} else if ok {
			s = newSessionFromStored(sessionID, systemPrompt, restoreMessages(stored), m.store)
			m.sessions[sessionID] = s
			log.Printf("[session] 会话 %s 已从持久化恢复（%d 条历史）", sessionID, len(s.messages)-1)
			return s
		}
	}
	s = newSession(sessionID, systemPrompt)
	s.store = m.store
	m.sessions[sessionID] = s
	return s
}

// evictOneLocked 在会话数达到上限时淘汰一个空闲会话。
// 用 tryBeginTurn 探测：抢不到轮转锁说明该会话正在问答中，跳过它找下一个；
// 抢到说明空闲（此刻不会有别人能再拿到该 Session 指针之外的引用去发起新轮次——
// 新建路径都在 m.mu 写锁内排队），立即释放并删除。极端情况下全部会话都在
// 问答中则本次不淘汰，允许短暂超限（问答结束后随空闲清理回收）。
func (m *Manager) evictOneIdleLocked() {
	for id, s := range m.sessions {
		if !s.tryBeginTurn() {
			continue
		}
		s.endTurn()
		delete(m.sessions, id)
		log.Printf("[session] 会话数达到上限 %d，淘汰空闲会话 %s", maxSessions, id)
		return
	}
}

// Delete 清空并删除一个会话（内存 + 持久化记录一并清除）
func (m *Manager) Delete(sessionID string) {
	m.mu.Lock()
	s, ok := m.sessions[sessionID]
	delete(m.sessions, sessionID)
	store := m.store
	m.mu.Unlock()
	if ok && s.store != nil {
		store = s.store
	}
	if store != nil {
		if err := store.Delete(sessionID); err != nil {
			log.Printf("[session] 会话 %s 持久化记录删除失败: %v", sessionID, err)
		}
	}
}

// cleanupLoop 后台定期清理空闲超过 sessionIdleTTL 的会话。
// 正在进行中的请求不受影响：调用方持有 *Session 指针，map 删除条目不影响本轮对话。
func (m *Manager) cleanupLoop() {
	ticker := time.NewTicker(sessionCleanupInterval)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		m.mu.Lock()
		removed := 0
		for id, s := range m.sessions {
			if s.idleFor(now) > sessionIdleTTL {
				delete(m.sessions, id)
				removed++
			}
		}
		m.mu.Unlock()
		if removed > 0 {
			log.Printf("[session] 空闲清理：移除 %d 个过期会话", removed)
		}
	}
}

// ---- 豆包消息构造辅助 ----

func newSystemMessage(content string) chat.ChatCompletionRequestMessage {
	sum := chat.ChatCompletionRequestMessageSum{}
	sum.SetChatCompletionRequestSystemMessage(chat.ChatCompletionRequestSystemMessage{
		Role:    chat.ChatCompletionRequestSystemMessageRoleSystem,
		Content: chat.NewStringChatCompletionMessageContent(content),
	})
	return chat.ChatCompletionRequestMessage{OneOf: sum}
}

// NewUserMessage 构造用户消息，供 Agent 内部及未来外部模块复用
func NewUserMessage(content string) chat.ChatCompletionRequestMessage {
	sum := chat.ChatCompletionRequestMessageSum{}
	sum.SetChatCompletionRequestUserMessage(chat.ChatCompletionRequestUserMessage{
		Role:    chat.ChatCompletionRequestUserMessageRoleUser,
		Content: chat.NewStringChatCompletionMessageContent(content),
	})
	return chat.ChatCompletionRequestMessage{OneOf: sum}
}

// UserImage 用户随消息发送的一张图片（二期：截图识别）。
// DataURL 必须是 data:image/...;base64,xxxx 形态的完整 data URL。
type UserImage struct {
	DataURL string
}

// NewUserMessageWithImages 构造"文字 + 图片"的多模态用户消息；
// 图片为空时退化为纯文本消息，纯文字路径零开销。
func NewUserMessageWithImages(content string, images []UserImage) chat.ChatCompletionRequestMessage {
	if len(images) == 0 {
		return NewUserMessage(content)
	}

	parts := make([]chat.ChatCompletionContentPart, 0, len(images)+1)
	if strings.TrimSpace(content) != "" {
		var tSum chat.ChatCompletionContentPartSum
		tSum.SetChatCompletionContentPartText(chat.ChatCompletionContentPartText{
			Type: chat.ChatCompletionContentPartTextTypeText,
			Text: content,
		})
		parts = append(parts, chat.ChatCompletionContentPart{OneOf: tSum})
	}
	for _, img := range images {
		if strings.TrimSpace(img.DataURL) == "" {
			continue
		}
		var iSum chat.ChatCompletionContentPartSum
		iSum.SetChatCompletionContentPartImage(chat.ChatCompletionContentPartImage{
			Type: chat.ChatCompletionContentPartImageTypeImageURL,
			ImageURL: chat.ChatCompletionContentPartImageImageUrl{
				URL: chat.NewOptString(img.DataURL),
			},
		})
		parts = append(parts, chat.ChatCompletionContentPart{OneOf: iSum})
	}

	sum := chat.ChatCompletionRequestMessageSum{}
	sum.SetChatCompletionRequestUserMessage(chat.ChatCompletionRequestUserMessage{
		Role:    chat.ChatCompletionRequestUserMessageRoleUser,
		Content: chat.NewChatCompletionContentPartArrayChatCompletionMessageContent(parts),
	})
	return chat.ChatCompletionRequestMessage{OneOf: sum}
}

// NewAssistantTextMessage 构造普通文本助手消息
func NewAssistantTextMessage(content string) chat.ChatCompletionRequestMessage {
	sum := chat.ChatCompletionRequestMessageSum{}
	msg := chat.ChatCompletionRequestAssistantMessage{
		Role: chat.ChatCompletionRequestAssistantMessageRoleAssistant,
	}
	if content != "" {
		opt := chat.OptChatCompletionMessageContent{}
		opt.SetTo(chat.NewStringChatCompletionMessageContent(content))
		msg.Content = opt
	}
	sum.SetChatCompletionRequestAssistantMessage(msg)
	return chat.ChatCompletionRequestMessage{OneOf: sum}
}

// NewAssistantToolCallMessage 构造携带工具调用请求的助手消息（多轮 Function Call 关键一环）
func NewAssistantToolCallMessage(calls []chat.ChatCompletionMessageToolCall) chat.ChatCompletionRequestMessage {
	sum := chat.ChatCompletionRequestMessageSum{}
	sum.SetChatCompletionRequestAssistantMessage(chat.ChatCompletionRequestAssistantMessage{
		Role:      chat.ChatCompletionRequestAssistantMessageRoleAssistant,
		ToolCalls: calls,
	})
	return chat.ChatCompletionRequestMessage{OneOf: sum}
}

// NewToolResultMessage 构造工具执行结果消息，toolCallID 必须与助手请求的工具调用ID对应
func NewToolResultMessage(toolCallID, toolName, content string) chat.ChatCompletionRequestMessage {
	sum := chat.ChatCompletionRequestMessageSum{}
	msg := chat.ChatCompletionRequestToolMessage{
		Role:       chat.ChatCompletionRequestToolMessageRoleTool,
		Content:    chat.NewStringChatCompletionMessageContent(content),
		ToolCallID: toolCallID,
	}
	if toolName != "" {
		msg.Name = chat.NewOptString(toolName)
	}
	sum.SetChatCompletionRequestToolMessage(msg)
	return chat.ChatCompletionRequestMessage{OneOf: sum}
}
