// app.go
// Wails 前端绑定层：前端 JS 能调用的 Go 方法全部声明在这里。
// 这一层只负责“接收请求 + 通过事件把流式回答/听写结果推给前端”，不含业务逻辑。
// 对话能力由 chatService 接口抽象（internal/remote 的 SSE 客户端实现）；
// 桌宠窗口控制（internal/win）与语音听写（internal/asr）经此处桥接到前端事件。
package main

import (
	"context"
	"errors"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"yhagent-client/internal/asr"
	"yhagent-client/internal/config"
	"yhagent-client/internal/remote"
	"yhagent-client/internal/win"
)

// 推给前端的事件名约定（前端 runtime.EventsOn 监听同名事件）
const (
	evChatChunk       = "chat:chunk"       // 流式文本片段，payload: string（打字机效果逐片到达）
	evChatDone        = "chat:done"        // 本轮回答正常结束，无 payload
	evChatError       = "chat:error"       // 本轮回答失败，payload: 中文错误信息
	evChatInterrupted = "chat:interrupted" // 本轮回答被中断，payload: {reason,turn_id}（已生成内容保留，前端渲染「重新生成」）

	evMicOn      = "mic:on"      // 开始听写（按住 PTT 键或点击麦克风），无 payload
	evMicOff     = "mic:off"     // 结束听写，无 payload
	evAsrPartial = "asr:partial" // 听写实时假设，payload: {text}（持续覆盖）
	evAsrFinal   = "asr:final"   // 一句定稿，payload: {text,confidence}
	evAsrUtter   = "asr:utterance" // PTT 松开后的聚合文本，payload: {text,autosend}
	evAsrError   = "asr:error"   // 听写链路失败，payload: {message}
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
	chat chatService // nil 安全：配置缺失时对话不可用但不崩溃
	cfg  *config.Config
	// configErr 启动期配置错误（前端启动时查询并气泡提示；空串=正常）
	configErr string
	// busy 保证同一窗口同一时间只有一轮回答在跑，避免上下文被并发写花
	busy atomic.Bool

	// 桌宠窗口控制器（nil 安全：单元测试/降级场景可不注入）
	winCtl *win.Controller
	// SAPI 听写管理器（nil 安全：PTT_ENABLED=0 时不注入）
	asrMgr *asr.Manager
	// held PTT 按住态（hotkey 轮询与手动麦克风共用一套听写编排）
	held atomic.Bool
	// utterMU 听写聚合文本的锁（多句 final 累积，松开时一次发前端）
	utterMu sync.Mutex
	utter   strings.Builder
}

// NewApp 创建绑定层实例，chat 由 main 注入（远端 SSE 客户端；配置缺失时可为 nil）
func NewApp(chat chatService, cfg *config.Config, configErr string) *App {
	return &App{chat: chat, cfg: cfg, configErr: configErr}
}

// ConfigError 启动期配置错误（前端启动时查询；空串=配置正常）
func (a *App) ConfigError() string { return a.configErr }

// AttachWinCtl 注入桌宠窗口控制器（main 的 OnStartup 前调用）
func (a *App) AttachWinCtl(c *win.Controller) { a.winCtl = c }

// AttachASR 注入听写管理器并接通事件转发（PTT_ENABLED=1 时由 main 调用）
func (a *App) AttachASR(m *asr.Manager) {
	a.asrMgr = m
	m.OnEvent(func(e asr.Event) {
		switch e.Type {
		case "partial":
			a.emit(evAsrPartial, map[string]any{"text": e.Text})
		case "final":
			a.utterMu.Lock()
			if a.utter.Len() > 0 {
				a.utter.WriteRune('，')
			}
			a.utter.WriteString(e.Text)
			a.utterMu.Unlock()
			a.emit(evAsrFinal, map[string]any{"text": e.Text, "confidence": e.Confidence})
		case "done":
			// 一句说完仍按住 → 自动续听下一句（Coopanion 多句 PTT 语义）
			if a.held.Load() && (e.Reason == "completed" || e.Reason == "timeout" || e.Reason == "rejected") {
				time.AfterFunc(300*time.Millisecond, func() {
					if a.held.Load() {
						a.startASR()
					}
				})
			}
		}
	})
}

// emit 事件推送（ctx 未就绪时静默，nil 安全）
func (a *App) emit(name string, data ...any) {
	if a.ctx == nil {
		return
	}
	wailsruntime.EventsEmit(a.ctx, name, data...)
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

// shutdown Wails 生命周期回调：释放后台资源
func (a *App) shutdown(ctx context.Context) {
	if a.winCtl != nil {
		a.winCtl.Stop()
	}
	if a.asrMgr != nil {
		a.asrMgr.Stop()
	}
}

// ---------- 桌宠窗口与听写绑定 ----------

// SetPetHitBox 前端上报宠物命中盒（CSS 像素，相对视口左上）+ 视口 CSS 尺寸。
// 宠物移动/动画切换时节流调用；Go 侧换算物理像素驱动穿透轮询。
func (a *App) SetPetHitBox(relX, relY, w, h, cssWinW, cssWinH float64) {
	if a.winCtl != nil {
		a.winCtl.SetHitBox(relX, relY, w, h, cssWinW, cssWinH)
	}
}

// SetDragging 拖拽宠物期间强制窗口可交互，防轮询间隙穿透丢事件
func (a *App) SetDragging(on bool) {
	if a.winCtl != nil {
		a.winCtl.ForceInteractive(on)
	}
}

// startASR 拉起一次听写进程（失败经事件上报，不返回错误打断调用方）
func (a *App) startASR() {
	if a.asrMgr == nil {
		return
	}
	if err := a.asrMgr.Start(); err != nil {
		log.Printf("[app] 听写启动失败: %v", err)
		a.emit(evAsrError, map[string]any{"message": err.Error()})
	}
}

// BeginListening 开始听写（hotkey 按下沿调用；也作为前端手动麦克风入口的内部实现）
func (a *App) beginListening() {
	if a.asrMgr == nil || !a.held.CompareAndSwap(false, true) {
		return
	}
	a.utterMu.Lock()
	a.utter.Reset()
	a.utterMu.Unlock()
	a.emit(evMicOn)
	a.startASR()
}

// endListening 结束听写并聚合文本（hotkey 松开沿调用）
func (a *App) endListening() {
	if a.asrMgr == nil || !a.held.CompareAndSwap(true, false) {
		return
	}
	a.asrMgr.Stop()
	a.utterMu.Lock()
	text := a.utter.String()
	a.utter.Reset()
	a.utterMu.Unlock()
	a.emit(evMicOff)
	a.emit(evAsrUtter, map[string]any{"text": text, "autosend": a.cfg != nil && a.cfg.ASRAutoSend})
}

// StartListening 前端手动麦克风（按钮按下）：与 PTT 共用编排
func (a *App) StartListening() { a.beginListening() }

// StopListening 前端手动麦克风（按钮松开）
func (a *App) StopListening() { a.endListening() }

// OnPTTDown / OnPTTUp 供 main 的 hotkey 轮询回调（按住说话/松开发送）
func (a *App) OnPTTDown() { a.beginListening() }
func (a *App) OnPTTUp()   { a.endListening() }

// Quit 退出应用（桌宠无标题栏，右键菜单的唯一退出通道）
func (a *App) Quit() {
	if a.ctx != nil {
		wailsruntime.Quit(a.ctx)
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
	if a.chat == nil {
		return errors.New("远端服务未配置（REMOTE_BASE_URL/REMOTE_TOKEN），请在 exe 同目录 .env 填写后重启")
	}
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

// ResetSession 清空指定会话的历史上下文（前端右键菜单调用）
func (a *App) ResetSession(sessionID string) {
	a.chat.Reset(sessionID)
}

// SetAlwaysOnTop 切换窗口置顶状态（右键菜单）
func (a *App) SetAlwaysOnTop(top bool) {
	wailsruntime.WindowSetAlwaysOnTop(a.ctx, top)
}
