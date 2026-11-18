// band.go 带状窗几何（Coopanion 行为规范）：宠物窗横跨工作区整宽、
// 贴底边固定间隙，宠物"地面"= 窗口底边 = 工作区底边。
// 全部纯函数，无 syscall 依赖，可跨平台单测。
package win

// BandConfig 带状窗几何参数（物理像素；main 层从 env 注入）
type BandConfig struct {
	// Height 窗口高度：宠物活动区 + 气泡空间（默认 260）
	Height int32
	// BottomGap 窗口底边离工作区底边的间隙（Coopanion AUTOHIDE_TASKBAR_GAP=3）
	BottomGap int32
}

// DefaultBandConfig 默认几何：高 260、离底边 3（与 Coopanion 参数一致）
func DefaultBandConfig() BandConfig {
	return BandConfig{Height: 260, BottomGap: 3}
}

// BandRect 计算带状窗矩形：工作区整宽 × Height 高，底边离工作区底 BottomGap。
// gap 为负时窗口底边超出工作区（覆盖任务栏，保留 Coopanion overTaskbar 语义）。
func BandRect(work Rect, cfg BandConfig) Rect {
	if cfg.Height <= 0 {
		cfg.Height = DefaultBandConfig().Height
	}
	return Rect{
		Left:   work.Left,
		Top:    work.Bottom - cfg.BottomGap - cfg.Height,
		Right:  work.Right,
		Bottom: work.Bottom - cfg.BottomGap,
	}
}

// ScaleHitBox 把前端上报的 CSS 像素命中盒换算为窗口物理像素坐标。
// 前端只知道自己视口的 CSS 尺寸（winW/winH）与宠物元素在视口内的 CSS 矩形；
// Go 侧拿到的窗口物理矩形由 GetWindowRect 给出，两者比值即 DPI 缩放。
// 返回相对窗口左上角的物理像素矩形（x,y,w,h 全为非负数）。
func ScaleHitBox(relX, relY, w, h, cssWinW, cssWinH float64, phys Rect) (x, y, pw, ph int32) {
	if cssWinW <= 0 || cssWinH <= 0 {
		return 0, 0, 0, 0
	}
	sx := float64(phys.Width()) / cssWinW
	sy := float64(phys.Height()) / cssWinH
	if sx < 0 {
		sx = 0
	}
	if sy < 0 {
		sy = 0
	}
	return int32(relX * sx), int32(relY * sy), int32(w * sx), int32(h * sy)
}

// HitTest 判断屏幕坐标点是否落在命中盒内（全部物理像素）。
// hitW/hitH 为 0 表示前端尚未上报（如启动初期），视为未命中。
func HitTest(cursor Point, winOrigin Point, hitX, hitY, hitW, hitH int32) bool {
	if hitW <= 0 || hitH <= 0 {
		return false
	}
	x, y := cursor.X-winOrigin.X, cursor.Y-winOrigin.Y
	return x >= hitX && x < hitX+hitW && y >= hitY && y < hitY+hitH
}
