// band_test.go 带状窗几何与命中换算的单测（纯函数，无系统依赖）
package win

import "testing"

func TestBandRect(t *testing.T) {
	// 1920x1080 全屏工作区（假设无任务栏）
	work := Rect{Left: 0, Top: 0, Right: 1920, Bottom: 1080}
	cfg := BandConfig{Height: 260, BottomGap: 3}

	got := BandRect(work, cfg)
	want := Rect{Left: 0, Top: 1080 - 3 - 260, Right: 1920, Bottom: 1080 - 3}
	if got != want {
		t.Fatalf("BandRect = %+v, want %+v", got, want)
	}
	// 地面=窗口底边=工作区底边-3（Coopanion 离底边 3 DIP 规范）
	if got.Bottom != 1077 {
		t.Fatalf("底边应为 1077，得到 %d", got.Bottom)
	}
}

func TestBandRectNegativeGap(t *testing.T) {
	// 覆盖任务栏语义：gap 为负时窗口底边超出工作区
	work := Rect{Left: 0, Top: 0, Right: 1920, Bottom: 1040}
	got := BandRect(work, BandConfig{Height: 300, BottomGap: -48})
	if got.Bottom != 1088 {
		t.Fatalf("负间隙时底边应为 1088，得到 %d", got.Bottom)
	}
}

func TestBandRectFallback(t *testing.T) {
	// 非法参数回退默认（高度 260 间隙 3）
	work := Rect{Left: 100, Top: 200, Right: 2100, Bottom: 1000}
	got := BandRect(work, BandConfig{Height: -5, BottomGap: 3})
	if got.Height() != 260 || got.Bottom != 997 {
		t.Fatalf("回退失败：got %+v", got)
	}
	// 多显示器工作区偏移保留（Left 跟随）
	if got.Left != 100 {
		t.Fatalf("工作区 X 偏移丢失：%d", got.Left)
	}
}

func TestScaleHitBox(t *testing.T) {
	// 1.0x DPI：CSS=物理，坐标原样
	phys := Rect{Left: 0, Top: 800, Right: 1920, Bottom: 1060}
	x, y, w, h := ScaleHitBox(100, 20, 200, 180, 1920, 260, phys)
	if x != 100 || y != 20 || w != 200 || h != 180 {
		t.Fatalf("1.0x 换算错误：%d,%d,%d,%d", x, y, w, h)
	}
	// 1.5x DPI：CSS 1280 → 物理 1920，坐标按 1.5 放大
	phys = Rect{Left: 0, Top: 800, Right: 1920, Bottom: 1190} // 高 390 物理
	x, y, w, h = ScaleHitBox(100, 20, 200, 173.3333, 1280, 260, phys)
	if x != 150 || y != 30 || w != 300 {
		t.Fatalf("1.5x 换算错误：%d,%d,%d,%d", x, y, w, h)
	}
	if h < 259 || h > 260 { // 173.3333*390/260=260（浮点容差）
		t.Fatalf("1.5x 高度换算错误：%d", h)
	}
}

func TestScaleHitBoxInvalidCSS(t *testing.T) {
	// 前端尚未上报尺寸（0）：返回全 0，由 HitTest 判未命中
	x, y, w, h := ScaleHitBox(10, 10, 10, 10, 0, 0, Rect{})
	if x != 0 || y != 0 || w != 0 || h != 0 {
		t.Fatalf("非法入参应返回 0：%d,%d,%d,%d", x, y, w, h)
	}
}

func TestHitTest(t *testing.T) {
	origin := Point{X: 100, Y: 800} // 窗口左上物理坐标
	cases := []struct {
		name              string
		cursor            Point
		hit               [4]int32
		want              bool
	}{
		{"命中盒内", Point{X: 150, Y: 850}, [4]int32{40, 40, 120, 160}, true},
		{"命中盒左上角", Point{X: 140, Y: 840}, [4]int32{40, 40, 120, 160}, true},
		{"命中盒右下角(开区间)", Point{X: 260, Y: 1040}, [4]int32{40, 40, 120, 160}, false},
		{"命中盒外", Point{X: 500, Y: 850}, [4]int32{40, 40, 120, 160}, false},
		{"未上报命中盒", Point{X: 150, Y: 850}, [4]int32{}, false},
		{"宽为0", Point{X: 150, Y: 850}, [4]int32{40, 40, 0, 160}, false},
	}
	for _, c := range cases {
		if got := HitTest(c.cursor, origin, c.hit[0], c.hit[1], c.hit[2], c.hit[3]); got != c.want {
			t.Errorf("%s：HitTest=%v want %v", c.name, got, c.want)
		}
	}
}
