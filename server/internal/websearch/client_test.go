package websearch

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// 用 2026-10-17 抓自方舟 Responses API 的真实响应验证兼容式解析：
// 正文取 output_text、搜索词取 action.query（分号分隔字符串）、
// 来源只取 annotations 里的 url_citation，且必须排除 logo_url 图片签名链接。
func TestParseRealResponses(t *testing.T) {
	raw, err := os.ReadFile("testdata/responses_harmonica.json")
	if err != nil {
		t.Fatalf("读取测试响应失败: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("测试响应不是合法 JSON: %v", err)
	}

	got := parseResponses(doc)

	if strings.TrimSpace(got.Answer) == "" {
		t.Fatal("Answer 为空，期望提取到 output_text 正文")
	}
	if !strings.Contains(got.Answer, "佐拉") {
		t.Fatalf("Answer 未包含关键事实「佐拉」，实际: %q", truncateForTest(got.Answer, 120))
	}

	if got.Query != "三角洲行动 守夜人口琴 获得方法" {
		t.Fatalf("Query 解析错误，期望取分号前第一个搜索词，实际: %q", got.Query)
	}

	if len(got.Sources) == 0 {
		t.Fatal("Sources 为空，期望从 annotations 提取到引用来源")
	}
	for _, s := range got.Sources {
		if s.URL == "" || !strings.HasPrefix(s.URL, "http") {
			t.Fatalf("来源 URL 非法: %+v", s)
		}
		if strings.Contains(s.URL, "logo") || strings.Contains(s.URL, "tplv-obj") {
			t.Fatalf("错误地把 logo/图片签名链接当成了来源: %s", s.URL)
		}
		if strings.TrimSpace(s.Title) == "" {
			t.Fatalf("来源标题为空: %+v", s)
		}
	}
	// 真实响应 3 条 annotation 中前两条 URL 相同，去重后应为 2 条
	if len(got.Sources) != 2 {
		t.Fatalf("来源数量错误，期望去重后 2 条，实际 %d 条: %+v", len(got.Sources), got.Sources)
	}
}

func truncateForTest(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}
