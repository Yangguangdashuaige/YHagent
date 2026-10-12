package main

import (
	"strings"
	"testing"
)

func TestExtractImageURLs(t *testing.T) {
	html := `<p><img src="//static.gametalk.qq.com/a.png" style="x"/></p>` +
		`<p><img src="https://static.gametalk.qq.com/b.jpg"></p>` +
		`<p><img src="//static.gametalk.qq.com/a.png"></p>` +
		`<img src="data:image/png;base64,xxx">`
	got := extractImageURLs(html)
	want := []string{
		"https://static.gametalk.qq.com/a.png",
		"https://static.gametalk.qq.com/b.jpg",
	}
	if len(got) != len(want) {
		t.Fatalf("得到 %v，期望 %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("第%d个 URL = %q，期望 %q", i, got[i], want[i])
		}
	}
}

func TestCleanOCRText(t *testing.T) {
	raw := `<div align="right"><i>群星</i></div>

**三角洲行动**

# 9月30日 更新公告
## 武器平衡性调整

---
[ PART 01 ]

### FS-12霰弹枪
- 四肢伤害倍率：0.6倍 → 0.4倍

<p align="right">项目组<br>2026年9月30日</p>`
	got := cleanOCRText(raw, "9月30日更新公告丨黑潮爆破平衡性调整与问题修复")

	for _, bad := range []string{"<div", "<p ", "PART", "---", "# 9月30日 更新公告"} {
		if strings.Contains(got, bad) {
			t.Errorf("清洗结果残留 %q:\n%s", bad, got)
		}
	}
	for _, good := range []string{"## 武器平衡性调整", "FS-12霰弹枪", "0.6倍 → 0.4倍"} {
		if !strings.Contains(got, good) {
			t.Errorf("清洗结果丢失 %q:\n%s", good, got)
		}
	}
	if t.Failed() {
		t.Logf("清洗结果:\n%s", got)
	}
}

func TestIsBanListTitle(t *testing.T) {
	if !isBanListTitle("违规行为处罚公告(9.24~10.7)") {
		t.Error("处罚公告应被识别为名单类")
	}
	if isBanListTitle("10月10日更新公告丨季中平衡性调整") {
		t.Error("正常更新公告不应被识别为名单类")
	}
}
