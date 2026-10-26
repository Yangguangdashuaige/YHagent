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
)

// 推给前端的事件名约定（前端 runtime.EventsOn 监听同名事件）
const (
	evChatChunk = "chat:chunk" // 流式文本片段，payload: string（打字机效果逐片到达）
	evChatDone  = "chat:done"  // 本轮回答正常结束，无 payload
	evChatError = "chat:error" // 本轮回答失败，payload: 中文错误信息
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

// SetAlwaysOnTop 切换窗口置顶状态（游戏悬浮场景）
func (a *App) SetAlwaysOnTop(top bool) {
	wailsruntime.WindowSetAlwaysOnTop(a.ctx, top)
}
