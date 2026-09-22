// internal/ai/session.go
package ai

import (
	"sync"

	"github.com/volcengine/ark-runtime-go/arkruntime/model/chat"
)

// defaultMaxHistory 单个会话默认保留的最近消息条数（不含 system 消息），
// 控制上下文长度，避免长对话 token 爆炸，适配游戏场景低内存占用。
const defaultMaxHistory = 20

// Session 单个会话的完整对话上下文，同一 sessionID 的多轮提问共享同一份历史
type Session struct {
	mu       sync.Mutex
	id       string
	messages []chat.ChatCompletionRequestMessage
	maxKeep  int // 除 system 消息外最多保留多少条
}

// newSession 创建会话并写入 system 消息
func newSession(id, systemPrompt string) *Session {
	s := &Session{
		id:      id,
		maxKeep: defaultMaxHistory,
	}
	s.reset(systemPrompt)
	return s
}

// reset 清空历史并重新写入 system 消息
func (s *Session) reset(systemPrompt string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.messages = []chat.ChatCompletionRequestMessage{newSystemMessage(systemPrompt)}
}

// append 追加一条消息（调用方持有会话概念上的串行顺序，内部仍加锁保护）
func (s *Session) append(msg chat.ChatCompletionRequestMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = append(s.messages, msg)
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
}

// NewSessionManager 初始化内存会话管理器（后续接入 SQLite 存储时可在这一层扩展持久化）
func NewSessionManager() *Manager {
	return &Manager{
		sessions: make(map[string]*Session),
	}
}

// GetOrCreate 获取已有会话，不存在则用 systemPrompt 初始化一个新会话
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
	s = newSession(sessionID, systemPrompt)
	m.sessions[sessionID] = s
	return s
}

// Delete 清空并删除一个会话
func (m *Manager) Delete(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, sessionID)
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
