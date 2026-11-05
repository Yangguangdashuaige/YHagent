// app.go
// Wails 前端绑定层：前端 JS 能调用的 Go 方法全部声明在这里。
// 这一层只负责“接收请求 + 通过事件把流式回答推给前端”，不包含任何业务逻辑。
// 对话能力由 chatService 接口抽象，当前唯一实现是 internal/remote 的 SSE 客户端。
package main

import (
	"context"
	"errors"
	"log"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"yhagent-client/internal/remote"
)

// 推给前端的事件名约定（前端 runtime.EventsOn 监听同名事件）
const (
	evChatChunk       = "chat:chunk"       // 流式文本片段，payload: string（打字机效果逐片到达）
	evChatDone        = "chat:done"        // 本轮回答正常结束，无 payload
	evChatError       = "chat:error"       // 本轮回答失败，payload: 中文错误信息
	evChatInterrupted = "chat:interrupted" // 本轮回答被中断，payload: {reason,turn_id}（已生成内容保留，前端渲染「重新生成」）
)

// chatService 客户端需要的对话能力（由 internal/remote.Client 实现）。
// 接口定义在本包内，客户端不依赖服务端任何代码，两边仅靠 SSE 事件协议对话。
type chatService interface {
	ChatWithSession(ctx context.Context, sessionID, text string, onChunk func(string)) error
	Reset(sessionID string)
}

// App 暴露给前端的绑定对象（前端调用路径：window.go.main.App.xxx）
type App struct {
	ctx  context.Context
	chat chatService
	// busy 保证同一窗口同一时间只有一轮回答在跑，避免上下文被并发写花
	busy atomic.Bool
}

// NewApp 创建绑定层实例，chat 由 main 注入（远端 SSE 客户端）
func NewApp(chat chatService) *App {
	return &App{chat: chat}
}

// startup Wails 生命周期回调：保存运行时上下文，事件推送依赖它
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	// 远端模式：注册 ask_user 澄清回调（Wails 事件透传给前端渲染选项卡）。
	// 本地模式无此能力，静默跳过（chat 不实现该接口）。
	if c, ok := a.chat.(*remote.Client); ok {
		c.SetOnClarify(func(question string, options []remote.ClarifyOpt) {
			wailsruntime.EventsEmit(a.ctx, "chat:clarify", map[string]any{
				"text":    question,
				"options": options,
			})
		})
	}
}

// newSessionTimeout 服务端签发会话ID的超时：签发接口极廉价，10s 只可能是网络问题
const newSessionTimeout = 10 * time.Second

// sessionMinter 由 internal/remote.Client 实现（接口在使用处声明，按需断言）
type sessionMinter interface {
	NewSession(ctx context.Context) (string, error)
}

// NewSession 返回会话ID：优先由服务端签发（服务端做格式白名单校验，
// 防止自造ID撞号越权）；服务不可达时兜底为本地 UUID v4——服务端过渡期
// 仍接受该格式，不阻塞进入界面，真正发消息时若网络不通会有明确错误。
func (a *App) NewSession() string {
	if m, ok := a.chat.(sessionMinter); ok {
		ctx, cancel := context.WithTimeout(context.Background(), newSessionTimeout)
		defer cancel()
		if id, err := m.NewSession(ctx); err == nil && id != "" {
			return id
		} else if err != nil {
			log.Printf("[app] 服务端签发会话ID失败，回退本地UUIDv4: %v", err)
		}
	}
	return uuid.NewString()
}

// Chat 接收用户提问后立即返回，真正的多轮对话在后台 goroutine 中执行，
// 回答片段 / 结束 / 失败分别通过 chat:chunk、chat:done、chat:error 事件异步推给前端。
func (a *App) Chat(sessionID, text string) error {
	if !a.busy.CompareAndSwap(false, true) {
		return errors.New("上一个问题还在回答中，请稍候")
	}

	go func() {
		defer a.busy.Store(false)

		err := a.chat.ChatWithSession(context.Background(), sessionID, text, func(chunk string) {
			wailsruntime.EventsEmit(a.ctx, evChatChunk, chunk)
		})
		if err != nil {
			log.Printf("[app] 对话失败: %v", err)
			// 回合被中断（断线/服务重启）：已生成内容已逐片推给前端，渲染「重新生成」而非报错
			var ie *remote.InterruptedError
			if errors.As(err, &ie) {
				wailsruntime.EventsEmit(a.ctx, evChatInterrupted, map[string]any{
					"reason":  ie.Reason,
					"turn_id": ie.TurnID,
				})
				return
			}
			wailsruntime.EventsEmit(a.ctx, evChatError, err.Error())
			return
		}
		wailsruntime.EventsEmit(a.ctx, evChatDone)
	}()

	return nil
}

// ResetSession 清空指定会话的历史上下文（前端“清空对话”按钮调用）
func (a *App) ResetSession(sessionID string) {
	a.chat.Reset(sessionID)
}

// SessionInfo 多会话列表项（前端可直接消费的序列化形态）
type SessionInfo struct {
	SessionID string `json:"session_id"`
	Title     string `json:"title"`
	UpdatedAt int64  `json:"updated_at"`
	MsgCount  int    `json:"msg_count"`
}

// sessionLister 由 internal/remote.Client 实现（接口在使用处声明，按需断言）
type sessionLister interface {
	ListSessions(ctx context.Context, sids []string) ([]remote.SessionSummary, error)
}

// ListSessions 批量拉取会话摘要（侧边栏会话列表）。前端传本地记录的 sid 集合，
// 服务端返回标题/最后活跃/消息数。失败返回空列表，UI 静默降级。
func (a *App) ListSessions(sids []string) []SessionInfo {
	l, ok := a.chat.(sessionLister)
	if !ok {
		return []SessionInfo{}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	sums, err := l.ListSessions(ctx, sids)
	if err != nil {
		log.Printf("[app] 会话列表获取失败: %v", err)
		return []SessionInfo{}
	}
	out := make([]SessionInfo, 0, len(sums))
	for _, s := range sums {
		out = append(out, SessionInfo{
			SessionID: s.SessionID, Title: s.Title, UpdatedAt: s.UpdatedAt, MsgCount: s.MsgCount,
		})
	}
	return out
}

// sessionDeleter 由 internal/remote.Client 实现
type sessionDeleter interface {
	DeleteSession(sessionID string) error
}

// DeleteSession 删除单个会话（服务端清上下文+落库+解绑）。旧版本本地 Agent
// 无此能力，返回 nil 视为成功（本地会话本就无持久化）。
func (a *App) DeleteSession(sessionID string) error {
	d, ok := a.chat.(sessionDeleter)
	if !ok {
		return nil
	}
	return d.DeleteSession(sessionID)
}

// ChatMsg 历史消息的序列化形态（前端渲染用）
type ChatMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// historyFetcher 由 internal/remote.Client 实现
type historyFetcher interface {
	SessionHistory(ctx context.Context, sessionID string) ([]remote.HistoryMsg, error)
}

// SessionHistory 拉取指定会话的可见历史（切换会话时恢复对话内容）。
// 失败返回空列表，UI 静默降级为欢迎语。
func (a *App) SessionHistory(sessionID string) []ChatMsg {
	h, ok := a.chat.(historyFetcher)
	if !ok {
		return []ChatMsg{}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	msgs, err := h.SessionHistory(ctx, sessionID)
	if err != nil {
		log.Printf("[app] 会话历史获取失败: %v", err)
		return []ChatMsg{}
	}
	out := make([]ChatMsg, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, ChatMsg{Role: m.Role, Content: m.Content})
	}
	return out
}

// feedbacker 由 internal/remote.Client 实现（接口在使用处声明，按需断言）
type feedbacker interface {
	SubmitFeedback(sessionID string, rating int, question, answer string) error
}

// SubmitFeedback 前端回答气泡 👍👎 按钮调用（rating: 1=赞 / -1=踩）。
// 点赞问答自动进服务端 few-shot 案例库（自学习闭环），点踩供失败模式分析。
func (a *App) SubmitFeedback(sessionID string, rating int, question, answer string) error {
	f, ok := a.chat.(feedbacker)
	if !ok {
		return errors.New("当前对话服务不支持反馈")
	}
	return f.SubmitFeedback(sessionID, rating, question, answer)
}

// SetAlwaysOnTop 切换窗口置顶状态（游戏悬浮场景）
func (a *App) SetAlwaysOnTop(top bool) {
	wailsruntime.WindowSetAlwaysOnTop(a.ctx, top)
}
