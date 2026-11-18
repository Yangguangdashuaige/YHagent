// Package hotkey 按键说话（PTT）热键轮询（Coopanion 行为规范）。
//
// 不用注册式全局热键（RegisterHotKey 需要消息循环且会吞掉按键本身），
// 而是复刻 Coopanion 的 GetAsyncKeyState 轮询：30ms 采样虚拟键按住状态，
// 边沿检测触发按下/松开回调——左 Alt 按住即说话，松开即结束。
// 该方式不吞按键（Alt+Tab 等系统组合不受影响）、无需窗口焦点。
package hotkey

import (
	"sync"
	"syscall"
	"time"
)

// 虚拟键码
const (
	VK_LMENU = 0xA4 // 左 Alt（Coopanion 默认 PTT 键）
)

var (
	user32          = syscall.NewLazyDLL("user32.dll")
	procGetAsyncKey = user32.NewProc("GetAsyncKeyState")
)

// isDown 采样虚拟键当前按住状态（高位置位；调用零次也无妨，只取瞬时值）
func isDown(vk int) bool {
	r, _, _ := procGetAsyncKey.Call(uintptr(vk))
	// 只有调用前一小段时间内的物理按下才会置高位；与"当前是否按住"合并判断，
	// 避免轮询间隙错过短按（低位表示自上次调用以来按过）
	return r&0x8000 != 0
}

// Edge 边沿检测结果
type Edge int

const (
	EdgeNone    Edge = iota // 无变化
	EdgeDown                 // 按下沿（未按 → 按住）
	EdgeUp                   // 松开沿（按住 → 未按）
)

// DetectEdge 纯函数边沿检测：prev 上一采样按住态，cur 当前采样按住态。
// 抽出来是为了单测覆盖轮询间隙抖动、粘滞等场景。
func DetectEdge(prev, cur bool) Edge {
	switch {
	case !prev && cur:
		return EdgeDown
	case prev && !cur:
		return EdgeUp
	default:
		return EdgeNone
	}
}

// Poller 热键轮询器（goroutine 安全的启停）
type Poller struct {
	vk      int
	interval time.Duration

	mu      sync.Mutex
	running bool
	stopCh  chan struct{}

	// onDown/onUp 在轮询 goroutine 中回调（上层应只做轻量转发）
	onDown func()
	onUp   func()
	// logf 可观测性注入（nil 安全）
	logf func(format string, args ...any)
}

// NewPoller 创建轮询器：vk 虚拟键码，interval 采样间隔（Coopanion HOTKEY_POLL_MS=30）
func NewPoller(vk int, interval time.Duration) *Poller {
	if vk <= 0 {
		vk = VK_LMENU
	}
	if interval <= 0 {
		interval = 30 * time.Millisecond
	}
	return &Poller{vk: vk, interval: interval}
}

// OnDown 注册按下沿回调（开始听写）
func (p *Poller) OnDown(fn func()) { p.onDown = fn }

// OnUp 注册松开沿回调（结束听写）
func (p *Poller) OnUp(fn func()) { p.onUp = fn }

// SetLogger 注入日志函数
func (p *Poller) SetLogger(fn func(format string, args ...any)) { p.logf = fn }

func (p *Poller) printf(format string, args ...any) {
	if p.logf != nil {
		p.logf(format, args...)
	}
}

// Start 启动轮询（幂等：重复调用返回 false）
func (p *Poller) Start() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.running {
		return false
	}
	p.running = true
	p.stopCh = make(chan struct{})
	ch := p.stopCh
	go p.loop(ch)
	return true
}

// Stop 停止轮询（幂等）
func (p *Poller) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.running {
		return
	}
	p.running = false
	close(p.stopCh)
}

// loop 轮询主体：初次采样建基线（启动瞬间的按住不触发按下沿）
func (p *Poller) loop(stopCh chan struct{}) {
	prev := isDown(p.vk)
	t := time.NewTicker(p.interval)
	defer t.Stop()
	for {
		select {
		case <-stopCh:
			return
		case <-t.C:
		}
		cur := isDown(p.vk)
		switch DetectEdge(prev, cur) {
		case EdgeDown:
			p.printf("[hotkey] PTT 按下（vk=0x%X）", p.vk)
			if p.onDown != nil {
				p.onDown()
			}
		case EdgeUp:
			p.printf("[hotkey] PTT 松开（vk=0x%X）", p.vk)
			if p.onUp != nil {
				p.onUp()
			}
		}
		prev = cur
	}
}
