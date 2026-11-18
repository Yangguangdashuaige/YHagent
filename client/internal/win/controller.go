// controller.go 桌宠窗口控制器：把 Wails 创建的普通窗口改造成桌宠窗口。
//
// 生命周期（main 的 OnStartup 之后调用 Start）：
//  1. ApplyBand：按主屏工作区计算带状窗几何并 SetWindowPos 贴底；
//  2. ToolWindow：加 WS_EX_TOOLWINDOW 跳过任务栏；
//  3. 穿透轮询（dsh-pet 机制）：60ms 读取全局光标 vs 前端上报的宠物
//     命中盒（物理像素），命中→可交互，未命中→WS_EX_LAYERED|
//     WS_EX_TRANSPARENT 全窗穿透；
//  4. 几何巡检：每 5s 复核工作区（分辨率/任务栏变化），变化即重放窗口。
package win

import (
	"sync"
	"time"
)

// Controller 桌宠窗口控制器（goroutine 安全）
type Controller struct {
	title  string
	cfg    BandConfig

	mu       sync.Mutex
	hwnd     uintptr
	hitBox   [4]int32 // 相对窗口左上的物理像素命中盒 x,y,w,h
	passThru bool     // 当前是否处于穿透态（去重 syscall）

	stopCh chan struct{}
	// logf 可观测性注入（测试桩可替换；nil 安全）
	logf func(format string, args ...any)
}

// NewController 创建控制器；title 为 Wails 窗口标题（FindWindow 定位用）
func NewController(title string, cfg BandConfig) *Controller {
	if cfg.Height <= 0 || cfg.BottomGap < 0 {
		cfg = DefaultBandConfig()
	}
	return &Controller{title: title, cfg: cfg, stopCh: make(chan struct{})}
}

// Start 定位窗口并启动后台轮询（窗口尚未创建时幂等重试，Wails 启动有时序竞争）
func (c *Controller) Start(retry time.Duration) {
	go func() {
		// 窗口查找重试：OnStartup 后 WebView 窗口可能仍未完成创建
		for i := 0; i < 50; i++ {
			if h := findWindow(c.title); h != 0 {
				c.mu.Lock()
				c.hwnd = h
				c.mu.Unlock()
				break
			}
			select {
			case <-c.stopCh:
				return
			case <-time.After(retry):
			}
		}
		c.mu.Lock()
		h := c.hwnd
		c.mu.Unlock()
		if h == 0 {
			c.printf("[win] 窗口定位失败：%s（穿透/带状几何将不生效）", c.title)
			return
		}

		c.applyBand()
		c.setToolWindow()
		go c.pollLoop()
		go c.geometryLoop()
	}()
}

// Stop 结束后台轮询（应用退出时调用；幂等）
func (c *Controller) Stop() {
	select {
	case <-c.stopCh:
	default:
		close(c.stopCh)
	}
}

// SetHitBox 前端上报宠物命中盒（CSS 像素，相对视口左上）+ 视口 CSS 尺寸，
// 由 app.go 绑定层在宠物移动/动画切换时节流调用。
func (c *Controller) SetHitBox(relX, relY, w, h, cssWinW, cssWinH float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.hwnd == 0 {
		return
	}
	phys, ok := getWindowRect(c.hwnd)
	if !ok {
		return
	}
	x, y, pw, ph := ScaleHitBox(relX, relY, w, h, cssWinW, cssWinH, phys)
	c.hitBox = [4]int32{x, y, pw, ph}
}

// ForceInteractive 拖拽期间前端强制可交互（与命中盒命中同效，
// 防止轮询间隙穿透导致拖拽丢事件）；传 false 恢复轮询判定。
func (c *Controller) ForceInteractive(on bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.hwnd == 0 {
		return
	}
	if on {
		c.applyStyleLocked(c.hwnd, false)
	}
	// 关闭强制态无需立即动作：下一轮轮询自然接管
}

// applyBand 工作区几何 → 窗口贴底（Coopanion 带状窗规范）
func (c *Controller) applyBand() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.hwnd == 0 {
		return
	}
	work, ok := monitorWorkArea(c.hwnd)
	if !ok {
		c.printf("[win] 工作区查询失败，保持当前窗口几何")
		return
	}
	r := BandRect(work, c.cfg)
	setWindowPos(c.hwnd, r.Left, r.Top, r.Width(), r.Height())
}

// setToolWindow 跳过任务栏（WS_EX_TOOLWINDOW），并常驻挂 WS_EX_LAYERED。
// LAYERED 是鼠标穿透（WS_EX_TRANSPARENT）对系统命中测试生效的前置条件；
// 需配 SetLayeredWindowAttributes(alpha=1)：alpha=255 时窗口表面整体不透明，
// 会把桌面盖成一条黑框——alpha=1 近全透明，桌面正常透出。
func (c *Controller) setToolWindow() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.hwnd == 0 {
		return
	}
	ex := getExStyle(c.hwnd)
	setExStyle(c.hwnd, ex|wsExToolWin|wsExLayered)
	setLayeredAlpha(c.hwnd, 1)
}

// pollLoop dsh-pet 穿透轮询：60ms 光标 vs 命中盒
func (c *Controller) pollLoop() {
	t := time.NewTicker(60 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-c.stopCh:
			return
		case <-t.C:
		}
		c.mu.Lock()
		if c.hwnd == 0 {
			c.mu.Unlock()
			continue
		}
		hwnd := c.hwnd
		cur, ok := getCursorPos()
		if !ok {
			c.mu.Unlock()
			continue
		}
		phys, ok := getWindowRect(hwnd)
		if !ok {
			c.mu.Unlock()
			continue
		}
		hb := c.hitBox
		hit := HitTest(cur, Point{phys.Left, phys.Top}, hb[0], hb[1], hb[2], hb[3])
		want := !hit
		if want != c.passThru {
			c.applyStyleLocked(hwnd, want)
		}
		c.mu.Unlock()
	}
}

// geometryLoop 工作区巡检：每 5s 复核（分辨率/任务栏高度变化时重放带状窗）
func (c *Controller) geometryLoop() {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-c.stopCh:
			return
		case <-t.C:
			c.applyBand()
		}
	}
}

// applyStyleLocked 实际写扩展样式（调用方持锁）。
// WS_EX_LAYERED 全程常驻（启动时挂一次 + alpha=1，见 ensureLayeredLocked），
// 轮询只切换 WS_EX_TRANSPARENT 位——避免 60ms 轮询反复增删 LAYERED 引起
// 视觉闪变；alpha=255 会让窗口表面整体不透明（黑框盖住桌面），alpha=1 的
// 窗口表面近全透明，WebView2 子窗口不参与父表面的 alpha 合成、照常渲染。
func (c *Controller) applyStyleLocked(hwnd uintptr, passThru bool) {
	ex := getExStyle(hwnd)
	if passThru {
		setExStyle(hwnd, ex|wsExTransp)
	} else {
		setExStyle(hwnd, ex &^ wsExTransp)
	}
	c.passThru = passThru
}

// printf 日志（logf 未注入时静默，业务不因可观测性失败而阻断）
func (c *Controller) printf(format string, args ...any) {
	if c.logf != nil {
		c.logf(format, args...)
	}
}

// SetLogger 注入日志函数（main 层接 log.Printf）
func (c *Controller) SetLogger(fn func(format string, args ...any)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.logf = fn
}
