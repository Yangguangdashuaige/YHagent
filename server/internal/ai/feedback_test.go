package ai

import (
	"path/filepath"
	"strings"
	"testing"
)

func newTestFeedbackService(t *testing.T) *FeedbackService {
	t.Helper()
	st, err := NewSQLiteFeedbackStore(filepath.Join(t.TempDir(), "feedback.db"))
	if err != nil {
		t.Fatalf("打开反馈库失败: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return NewFeedbackService(st)
}

// TestFeedbackSubmit 反馈落库 + 点赞自动进案例库；点踩只落反馈不动案例库
func TestFeedbackSubmit(t *testing.T) {
	svc := newTestFeedbackService(t)

	err := svc.Submit(Feedback{
		TraceID: "t_abc", SessionID: "sess-1", UID: 7, Rating: 1,
		Question: "M14 怎么改出据枪+8？", Answer: "## 推荐方案\n改枪码：`xxx`",
	})
	if err != nil {
		t.Fatalf("点赞提交失败: %v", err)
	}
	err = svc.Submit(Feedback{
		TraceID: "t_def", SessionID: "sess-1", UID: 7, Rating: -1,
		Question: "完全无关的差评问题", Answer: "不太行",
	})
	if err != nil {
		t.Fatalf("点踩提交失败: %v", err)
	}

	// 直接读 store 验证两表内容
	st := svc.store.(*SQLiteFeedbackStore)
	var n int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM feedback`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("feedback 表应有 2 条，实际 %d（err=%v）", n, err)
	}
	cases, err := st.LoadCases(10)
	if err != nil {
		t.Fatalf("读取案例库失败: %v", err)
	}
	if len(cases) != 1 {
		t.Fatalf("点踩不应进案例库，期望 1 条案例，实际 %d", len(cases))
	}
	if cases[0].Question != "M14 怎么改出据枪+8？" {
		t.Fatalf("案例问题异常: %q", cases[0].Question)
	}
	// Markdown 换行被清洗为空格（防伪造 system 结构）
	if strings.ContainsAny(cases[0].Answer, "\n\r") {
		t.Fatalf("案例回答应剥掉换行: %q", cases[0].Answer)
	}
}

// TestFeedbackSubmitInvalidRating 非法 rating 拒收
func TestFeedbackSubmitInvalidRating(t *testing.T) {
	svc := newTestFeedbackService(t)
	if err := svc.Submit(Feedback{Rating: 5}); err == nil {
		t.Fatalf("rating=5 应被拒绝")
	}
	if err := svc.Submit(Feedback{Rating: 0}); err == nil {
		t.Fatalf("rating=0 应被拒绝")
	}
}

// TestCaseBlock 案例检索注入：相似问题命中、不相关问题不注入、
// 注入块含防照抄声明与清洗后的案例文本
func TestCaseBlock(t *testing.T) {
	svc := newTestFeedbackService(t)
	mustSubmit := func(q, a string) {
		if err := svc.Submit(Feedback{Rating: 1, Question: q, Answer: a}); err != nil {
			t.Fatalf("提交失败: %v", err)
		}
	}
	mustSubmit("M14 怎么改出据枪+8", "M14 据枪方案：枪管选XX……")
	mustSubmit("M7 改枪方案推荐", "M7 方案……")
	mustSubmit("密码门今天多少", "今天密码……")

	// 相似提问命中 M14 案例
	block := svc.CaseBlock("m14怎么改出据枪加8")
	if block == "" {
		t.Fatalf("相似问题应命中案例")
	}
	if !strings.Contains(block, "参考成功案例") || !strings.Contains(block, "不要照抄") {
		t.Fatalf("注入块缺少防照抄声明: %q", block)
	}
	if !strings.Contains(block, "M14 怎么改出据枪+8") {
		t.Fatalf("注入块应含 M14 案例: %q", block)
	}

	// 无关问题不注入
	if b := svc.CaseBlock("帮我查一下曼德尔砖价格"); b != "" {
		t.Fatalf("无关问题不应注入，实际: %q", b)
	}
	// 空查询安全返回
	if b := svc.CaseBlock("  "); b != "" {
		t.Fatalf("空查询应返回空串")
	}
}

// TestCaseBlockTopN 多条命中时只注入相似度最高的前 3 条
func TestCaseBlockTopN(t *testing.T) {
	svc := newTestFeedbackService(t)
	for i := 0; i < 5; i++ {
		q := strings.Repeat("M14 改枪 配装 方案 ", i+1)
		if err := svc.Submit(Feedback{Rating: 1, Question: q, Answer: "方案" + string(rune('A'+i))}); err != nil {
			t.Fatalf("提交失败: %v", err)
		}
	}
	block := svc.CaseBlock("M14 改枪 配装 方案")
	got := strings.Count(block, "【案例")
	if got > maxCaseInject {
		t.Fatalf("注入案例数 %d 超上限 %d", got, maxCaseInject)
	}
	if got == 0 {
		t.Fatalf("应有案例命中")
	}
}

// TestFeedbackClean 清洗：控制字符剥离、按字符限长（中文按字符不按字节截断）
func TestFeedbackClean(t *testing.T) {
	in := "第一行\n第二行\t缩进\x00控制符"
	if got := feedbackClean(in, 100); got != "第一行 第二行 缩进控制符" {
		t.Fatalf("清洗结果异常: %q", got)
	}
	if got := feedbackClean("一二三四五", 3); got != "一二三" {
		t.Fatalf("限长截断异常: %q", got)
	}
}
