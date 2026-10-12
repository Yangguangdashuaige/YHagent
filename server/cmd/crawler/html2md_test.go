package main

import (
	"fmt"
	"strings"
	"testing"
)

func TestHTMLToMarkdownSample(t *testing.T) {
	src := `<p><span style="color: #fff;"><strong><span>【新内容】</span></strong></span></p><p>&nbsp;</p>` +
		`<p><strong>1、限时单排体验</strong></p>` +
		`<p>活动时间：7月5日0点~7月8日0点</p>` +
		`<p><img src="https://x/y.jpg" style="display:block"/></p>` +
		`<ul><li>第一项：<strong>加粗</strong>内容</li><li>第二项</li></ul>` +
		`<table><tr><th>模式</th><th>时间</th></tr><tr><td>单排</td><td>3天</td></tr></table>` +
		`<p>含<a href="https://df.qq.com/x">链接文字</a>与&nbsp;&nbsp;多个空格</p>`
	out := htmlToMarkdown(src)
	fmt.Println("----MARKDOWN----")
	fmt.Println(out)
	fmt.Println("----------------")
	for _, want := range []string{"## 新内容", "**1、限时单排体验**", "- 第一项：**加粗**内容",
		"- 第二项", "| 模式 | 时间 |", "| --- | --- |", "| 单排 | 3天 |", "链接文字"} {
		if !strings.Contains(out, want) {
			t.Errorf("输出缺少 %q", want)
		}
	}
	if strings.Contains(out, "<") || strings.Contains(out, "y.jpg") {
		t.Errorf("输出残留标签或图片链接:\n%s", out)
	}
	if strings.Contains(out, "- 第一项\n\n- 第二项") {
		t.Errorf("列表项之间不应有空行")
	}
}
