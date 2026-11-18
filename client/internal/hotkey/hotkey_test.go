// hotkey_test.go 边沿检测纯函数单测
package hotkey

import (
	"testing"
	"time"
)

func TestDetectEdge(t *testing.T) {
	cases := []struct {
		prev, cur bool
		want      Edge
	}{
		{false, true, EdgeDown},  // 按下沿
		{true, false, EdgeUp},    // 松开沿
		{false, false, EdgeNone}, // 一直松开
		{true, true, EdgeNone},  // 一直按住
	}
	for _, c := range cases {
		if got := DetectEdge(c.prev, c.cur); got != c.want {
			t.Errorf("DetectEdge(%v,%v)=%v want %v", c.prev, c.cur, got, c.want)
		}
	}
}

func TestPollerDefaults(t *testing.T) {
	// 非法参数回退默认键（左 Alt）与默认间隔
	p := NewPoller(-1, -1)
	if p.vk != VK_LMENU {
		t.Fatalf("默认键应为左 Alt(0x%X)，得到 0x%X", VK_LMENU, p.vk)
	}
	if p.interval != 30*time.Millisecond {
		t.Fatalf("默认间隔应为 30ms，得到 %v", p.interval)
	}
}

func TestPollerStartStop(t *testing.T) {
	p := NewPoller(VK_LMENU, 5*time.Millisecond)
	if !p.Start() {
		t.Fatal("首次 Start 应成功")
	}
	if p.Start() {
		t.Fatal("重复 Start 应返回 false")
	}
	p.Stop()
	p.Stop() // 幂等
}
