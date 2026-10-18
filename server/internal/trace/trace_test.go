package trace

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestSpanNilSafe(t *testing.T) {
	// Recorder 未启用时 Start 返回 nil，所有方法必须是 no-op 而不是 panic
	span := Start("s1", "你好", 0)
	if span != nil {
		t.Fatalf("Default 未装配时 Start 应返回 nil")
	}
	span.LLMRound(100, 10, 20, 30)
	span.FirstToken()
	span.ToolCall(1, "delta_price", "{}", 5, 100, true)
	span.NoteCacheHit()
	span.NoteCacheMiss()
	span.AnswerChars(42)
	span.End("")

	// FromContext 对空 context / 未注入 context 均返回 nil
	if got := FromContext(context.Background()); got != nil {
		t.Fatalf("未注入时 FromContext 应返回 nil")
	}
	if got := FromContext(nil); got != nil {
		t.Fatalf("nil context 应返回 nil")
	}
}

func TestRecorderWriteAndRecent(t *testing.T) {
	dir := t.TempDir()
	r := NewRecorder(dir, 7)
	if r == nil {
		t.Fatalf("NewRecorder 失败")
	}
	t.Cleanup(r.Close) // Windows 下必须显式关句柄，TempDir 才能清理

	span := &Span{
		rec: r, traceID: "t_test", sessionID: "s1",
		query: "K437 压枪方案", images: 1,
		start: time.Now(),
	}
	span.LLMRound(2100, 1523, 35, 1558)
	span.ToolCall(1, "search_knowledge", `{"query":"K437"}`, 450, 3210, true)
	hit := true
	span.tools[0].CacheHit = &hit
	span.NoteCacheHit()
	span.NoteCacheMiss()
	span.AnswerChars(520)
	span.FirstToken()
	span.End("")

	// 写一条失败的 trace（error 路径）
	s2 := &Span{rec: r, traceID: "t_err", sessionID: "s2", query: "测试", start: time.Now()}
	s2.LLMRound(50, 0, 0, 0)
	s2.End("豆包AI响应超时，请检查网络后稍后再试")

	recs := r.Recent(10)
	if len(recs) != 2 {
		t.Fatalf("期望读回 2 条，实际 %d", len(recs))
	}
	// Recent 按时间倒序：最新（失败那条）在前
	if recs[0].TraceID != "t_err" || recs[0].Error == "" {
		t.Fatalf("最新一条应为失败的 t_err: %+v", recs[0])
	}
	if recs[0].OK {
		t.Fatalf("失败 trace 的 OK 应为 false")
	}
	got := recs[1]
	if got.TraceID != "t_test" || !got.OK {
		t.Fatalf("成功 trace 字段异常: %+v", got)
	}
	if got.LLMRounds != 1 || len(got.LLM) != 1 || got.LLM[0].PromptTokens != 1523 {
		t.Fatalf("LLM 轮次记录异常: %+v", got.LLM)
	}
	if len(got.Tools) != 1 || got.Tools[0].ResultBytes != 3210 || got.Tools[0].CacheHit == nil || !*got.Tools[0].CacheHit {
		t.Fatalf("工具调用记录异常: %+v", got.Tools)
	}
	if got.CacheHits != 1 || got.CacheMisses != 1 {
		t.Fatalf("缓存计数异常: hits=%d misses=%d", got.CacheHits, got.CacheMisses)
	}
	if got.AnswerChars != 520 || got.FirstTokMS < 0 {
		t.Fatalf("回答/TTFT 记录异常: chars=%d ttft=%d", got.AnswerChars, got.FirstTokMS)
	}

	// limit 截断
	if got := r.Recent(1); len(got) != 1 || got[0].TraceID != "t_err" {
		t.Fatalf("Recent(1) 应只返回最新 1 条")
	}
	// 非法行被跳过
	path := filepath.Join(dir, fileName(time.Now().Format("20060102")))
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("追加打开失败: %v", err)
	}
	_, _ = f.WriteString("{broken json\n")
	_ = f.Close()
	if got := r.Recent(10); len(got) != 2 {
		t.Fatalf("非法 JSON 行应被跳过，期望 2 条，实际 %d", len(got))
	}
}

func TestRecorderRecentNilAndEmpty(t *testing.T) {
	var r *Recorder
	if got := r.Recent(10); got != nil {
		t.Fatalf("nil Recorder 的 Recent 应返回 nil")
	}
	r = NewRecorder(t.TempDir(), 7)
	t.Cleanup(r.Close)
	if got := r.Recent(10); len(got) != 0 {
		t.Fatalf("空目录 Recent 应返回空切片")
	}
	if got := r.Recent(0); got != nil {
		t.Fatalf("n<=0 应返回 nil")
	}
}

func TestCleanupExpired(t *testing.T) {
	dir := t.TempDir()
	r := NewRecorder(dir, 7)
	if r == nil {
		t.Fatalf("NewRecorder 失败")
	}
	t.Cleanup(r.Close)
	// 造一个 30 天前的旧文件与一个 3 天前的文件（仍在保留窗口内）
	old := filepath.Join(dir, fileName(time.Now().AddDate(0, 0, -30).Format("20060102")))
	recent := filepath.Join(dir, fileName(time.Now().AddDate(0, 0, -3).Format("20060102")))
	for _, p := range []string{old, recent} {
		if err := os.WriteFile(p, []byte("{}\n"), 0o644); err != nil {
			t.Fatalf("造文件失败: %v", err)
		}
	}
	r.mu.Lock()
	r.cleanupExpiredLocked(time.Now())
	r.mu.Unlock()
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("过期文件未被清理")
	}
	if _, err := os.Stat(recent); err != nil {
		t.Fatalf("保留窗口内的文件不应被清理: %v", err)
	}
}

func TestPreviewUTF8Boundary(t *testing.T) {
	// 截断点落在多字节汉字中间时不应产生非法 UTF-8
	s := strings.Repeat("汉", 300) // 900 字节
	got := preview(s, 100)
	if !utf8.ValidString(got) {
		t.Fatalf("preview 产生了非法 UTF-8: %q", got)
	}
	if len(got) > 100+len("…(截断)") {
		t.Fatalf("preview 超长: %d", len(got))
	}
	// 短字符串原样返回（空白压缩与 agent 日志 previewArgs 同口径：去除全部空白）
	if got := preview("  hello  world  ", 100); got != "helloworld" {
		t.Fatalf("空白压缩异常: %q", got)
	}
}
