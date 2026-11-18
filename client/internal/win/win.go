// Package win 桌宠窗口的原生控制层（纯 Win32 syscall，零 CGO）。
//
// 职责对应两份参考实现的机制（均为行为复刻而非代码搬运）：
//   - dsh-pet：透明置顶小窗 + 60ms 光标轮询，除宠物命中区外全窗鼠标穿透；
//   - Coopanion：带状窗几何（离工作区底边固定间隙、地面=工作区底边）。
package win

import (
	"syscall"
	"unsafe"
)

// 常量：窗口扩展样式与定位标记（WinUser.h）
const (
	wsExTransp    = 0x20    // WS_EX_TRANSPARENT：鼠标命中穿透
	wsExLayered   = 0x80000 // WS_EX_LAYERED：分层窗口（鼠标穿透的前置条件）
	wsExToolWin   = 0x80    // WS_EX_TOOLWINDOW：跳过任务栏
	swpNoZOrder   = 0x0004
	swpNoActivate = 0x0010

	monitorDefaultToNearest = 2

	lwaAlpha = 2 // 按 alpha 通道设置分层属性
)

// gwlExStyle GetWindowLongPtr 的扩展样式槽位。负值无法用常量转 uintptr
// （编译期溢出），声明为变量走运行时按位回绕，即 Win32 期望的位模式。
var gwlExStyle = -20

var (
	user32 = syscall.NewLazyDLL("user32.dll")

	procFindWindowW            = user32.NewProc("FindWindowW")
	procGetCursorPos           = user32.NewProc("GetCursorPos")
	procGetWindowRect          = user32.NewProc("GetWindowRect")
	procGetWindowLongPtrW      = user32.NewProc("GetWindowLongPtrW")
	procSetWindowLongPtrW      = user32.NewProc("SetWindowLongPtrW")
	procSetWindowPos           = user32.NewProc("SetWindowPos")
	procSetLayeredWindowAttrs  = user32.NewProc("SetLayeredWindowAttributes")
	procMonitorFromWindow      = user32.NewProc("MonitorFromWindow")
	procGetMonitorInfoW         = user32.NewProc("GetMonitorInfoW")
)

// Point 屏幕物理像素坐标（与 Win32 POINT 一致）
type Point struct{ X, Y int32 }

// Rect 屏幕物理像素矩形（与 Win32 RECT 一致；物理像素，无 DPI 缩放）
type Rect struct{ Left, Top, Right, Bottom int32 }

// Width 矩形宽度（物理像素）
func (r Rect) Width() int32 { return r.Right - r.Left }

// Height 矩形高度（物理像素）
func (r Rect) Height() int32 { return r.Bottom - r.Top }

// monitorInfo 与 Win32 MONITORINFO 对齐（cbSize 必须为结构体总大小）
type monitorInfo struct {
	cbSize    uint32
	rcMonitor Rect
	rcWork    Rect
	dwFlags   uint32
}

// findWindow 按窗口标题查找顶层窗口句柄（Wails 未暴露 HWND，标题定位是
// 社区通行做法；标题在 main.go 中保证唯一）
func findWindow(title string) uintptr {
	t, _ := syscall.UTF16PtrFromString(title)
	h, _, _ := procFindWindowW.Call(0, uintptr(unsafe.Pointer(t)))
	return h
}

// getCursorPos 读取全局光标物理坐标；失败返回 false（可观测性失败不阻断轮询）
func getCursorPos() (Point, bool) {
	var p Point
	r, _, _ := procGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	return p, r != 0
}

// getWindowRect 读取窗口物理矩形
func getWindowRect(hwnd uintptr) (Rect, bool) {
	var r Rect
	ok, _, _ := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	return r, ok != 0
}

// getExStyle 读取窗口扩展样式位
func getExStyle(hwnd uintptr) uintptr {
	v, _, _ := procGetWindowLongPtrW.Call(hwnd, uintptr(gwlExStyle))
	return v
}

// setExStyle 写入窗口扩展样式位
func setExStyle(hwnd uintptr, exstyle uintptr) {
	procSetWindowLongPtrW.Call(hwnd, uintptr(gwlExStyle), exstyle)
}

// setWindowPos 调整窗口位置/尺寸（物理像素）
func setWindowPos(hwnd uintptr, x, y, w, h int32) {
	procSetWindowPos.Call(hwnd, 0, uintptr(x), uintptr(y), uintptr(w), uintptr(h),
		uintptr(swpNoZOrder|swpNoActivate))
}

// monitorWorkArea 取窗口所在显示器的工作区（不含任务栏）矩形
func monitorWorkArea(hwnd uintptr) (Rect, bool) {
	hm, _, _ := procMonitorFromWindow.Call(hwnd, uintptr(monitorDefaultToNearest))
	if hm == 0 {
		return Rect{}, false
	}
	var mi monitorInfo
	mi.cbSize = uint32(unsafe.Sizeof(mi))
	ok, _, _ := procGetMonitorInfoW.Call(hm, uintptr(unsafe.Pointer(&mi)))
	return mi.rcWork, ok != 0
}

// setLayeredAlpha 给分层窗口设置整体不透明度（不调用的话 WS_EX_LAYERED
// 窗口不会绘制）。alpha=255 时窗口表面整体不透明——桌宠带状窗会盖住桌面
// 呈现"黑框"；alpha=1 近全透明，桌面正常透出，WebView2 子窗口不受影响。
func setLayeredAlpha(hwnd uintptr, alpha byte) {
	procSetLayeredWindowAttrs.Call(hwnd, 0, uintptr(alpha), uintptr(lwaAlpha))
}
