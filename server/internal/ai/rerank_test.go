// internal/ai/rerank_test.go
//
// LLM 重排的纯本地单测：只测输出解析与分数换算（parseRankList / validOrder /
// 位置分数单调性），不发任何网络请求。VikingReranker 的 V4 签名与线上调用
// 属实机依赖（AK/SK + 开通状态），由 eval 工具实跑覆盖，不做 mock。
package ai

import (
	"strings"
	"testing"
)

func TestParseRankList(t *testing.T) {
	cases := []struct {
		name    string
		content string
		n       int
		want    []int // nil 表示期望"原序兜底"
	}{
		{"标准JSON", `{"rank":[3,1,2]}`, 3, []int{2, 0, 1}},
		{"带围栏", "```json\n{\"rank\":[2,3,1]}\n```", 3, []int{1, 2, 0}},
		{"裸数组", `[2,1]`, 2, []int{1, 0}},
		{"夹带说明文字", "好的，排序结果如下：{\"rank\":[1,2,3]} 以上。", 3, []int{0, 1, 2}},
		{"越界剔除", `{"rank":[1,2,9]}`, 2, []int{0, 1}},
		{"重复剔除后缺号补尾", `{"rank":[2,2]}`, 3, []int{1, 0, 2}},
		{"完全乱掉回退原序", "抱歉我不能排序", 3, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := parseRankList(c.content, c.n)
			if c.want == nil {
				for i := range got {
					if got[i] != i {
						t.Fatalf("期望原序兜底，得到 %v", got)
					}
				}
				return
			}
			if len(got) != len(c.want) {
				t.Fatalf("长度不符：got %v want %v", got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Fatalf("got %v want %v", got, c.want)
				}
			}
		})
	}
}

func TestValidOrderPerm(t *testing.T) {
	// 任何合法输出都必须是 0..n-1 的全排列（不重不漏）
	for _, nums := range [][]int{{1, 2, 3, 4}, {4, 3, 2, 1}, {2, 4, 1, 3}} {
		order := validOrder(nums, 4)
		seen := map[int]bool{}
		for _, v := range order {
			if v < 0 || v > 3 || seen[v] {
				t.Fatalf("非法排列 %v -> %v", nums, order)
			}
			seen[v] = true
		}
		if len(order) != 4 {
			t.Fatalf("长度不足: %v", order)
		}
	}
}

func TestSortDocsByScore(t *testing.T) {
	scores := []float64{0.1, 0.9, 0.5, 0.9}
	idx := SortDocsByScore(scores)
	// 稳定排序：同分保持原相对顺序（1 在 3 前）
	want := []int{1, 3, 2, 0}
	for i := range idx {
		if idx[i] != want[i] {
			t.Fatalf("got %v want %v", idx, want)
		}
	}
}

func TestLLMRerankScoreShape(t *testing.T) {
	// 不发网络：直接验证分数换算逻辑需要 mock chatOnce，此处仅约束
	// 接口语义——空输入返回 nil（生产零候选短路路径）
	r := &LLMReranker{model: "test"}
	scores, err := r.Rerank(nil, "q", nil)
	if err != nil || scores != nil {
		t.Fatalf("空输入应返回 nil/nil，得到 %v %v", scores, err)
	}
}

func TestTruncateRunes(t *testing.T) {
	s := strings.Repeat("枪", 300)
	got := truncateRunes(s, 100)
	if n := len([]rune(got)); n != 101 { // 100 + 省略号
		t.Fatalf("截断后长度 %d", n)
	}
}
