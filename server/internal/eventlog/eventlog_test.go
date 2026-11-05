package eventlog

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEmitNilSafe(t *testing.T) {
	// Default 未装配时，包级 Emit 必须 no-op 而不是 panic
	var r *Recorder
	r.Emit(LevelInfo, "x", nil)
	r.EmitWith(LevelInfo, "x", "t1", "s1", nil)
	if got := r.Recent(10); got != nil {
		t.Fatalf("nil Recorder 的 Recent 应返回 nil")
	}
	Emit(LevelInfo, "x", nil)       // Default == nil
	EmitWith(LevelInfo, "x", "", "", nil)
}

func TestEmitAndRecent(t *testing.T) {
	dir := t.TempDir()
	r := NewRecorder(dir, 30)
	if r == nil {
		t.Fatalf("NewRecorder 失败")
	}
	t.Cleanup(r.Close)

	r.EmitWith(LevelInfo, EvtServiceStart, "", "", map[string]any{"pid": 1234, "addr": ":8080"})
	r.EmitWith(LevelError, EvtAgentTurnCrashed, "t_abc", "sess_1", map[string]any{"panic": "nil pointer"})
	r.Emit(LevelWarn, EvtUpstreamError, map[string]any{"error": "timeout"})

	got := r.Recent(10)
	if len(got) != 3 {
		t.Fatalf("期望读回 3 条，实际 %d", len(got))
	}
	// Recent 倒序：最新（warn 那条）在前
	if got[0].Event != EvtUpstreamError || got[0].Level != LevelWarn {
		t.Fatalf("最新一条应为 upstream.error warn: %+v", got[0])
	}
	if got[1].Event != EvtAgentTurnCrashed || got[1].TraceID != "t_abc" || got[1].SessionID != "sess_1" {
		t.Fatalf("第二条 trace/session 关联异常: %+v", got[1])
	}
	if got[1].Fields["panic"] != "nil pointer" {
		t.Fatalf("fields 透传异常: %+v", got[1].Fields)
	}
	if got[2].Fields["pid"] != float64(1234) {
		t.Fatalf("数值字段经 JSON 往返应为 float64: %+v", got[2].Fields)
	}

	// limit 截断
	if g := r.Recent(1); len(g) != 1 || g[0].Event != EvtUpstreamError {
		t.Fatalf("Recent(1) 应只返回最新 1 条")
	}
	// 非法 JSON 行被跳过
	path := filepath.Join(dir, fileName(time.Now().Format("20060102")))
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("追加打开失败: %v", err)
	}
	_, _ = f.WriteString("{broken json\n")
	_ = f.Close()
	if g := r.Recent(10); len(g) != 3 {
		t.Fatalf("非法行应被跳过，期望 3 条，实际 %d", len(g))
	}
}

func TestRecentEmptyAndInvalid(t *testing.T) {
	r := NewRecorder(t.TempDir(), 30)
	t.Cleanup(r.Close)
	if g := r.Recent(10); len(g) != 0 {
		t.Fatalf("空目录应返回空切片，实际 %d", len(g))
	}
	if g := r.Recent(0); g != nil {
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

func TestAlertRestartFlapping(t *testing.T) {
	dir := t.TempDir()
	r := NewRecorder(dir, 30)
	t.Cleanup(r.Close)
	r.alerter = newAlerter(r)
	r.alerter.restartWindow = 5 * time.Minute
	r.alerter.restartThreshold = 3

	// 连续 3 次异常重启 → 应触发 1 次告警（冷却内不重复）
	for i := 0; i < 3; i++ {
		r.Emit(LevelError, EvtServiceRestartDetect, map[string]any{"i": i})
	}
	time.Sleep(50 * time.Millisecond) // 等 webhook 无关路径；落盘是同步的
	got := r.Recent(50)
	alertCount := 0
	for _, e := range got {
		if e.Event == EvtAlertRestartFlapping {
			alertCount++
		}
	}
	if alertCount != 1 {
		t.Fatalf("应触发 1 次重启抖动告警，实际 %d 次（共 %d 条事件）", alertCount, len(got))
	}

	// 再来 2 次仍在冷却期 → 不应新增告警
	for i := 0; i < 2; i++ {
		r.Emit(LevelError, EvtServiceRestartDetect, nil)
	}
	alertCount2 := 0
	for _, e := range r.Recent(50) {
		if e.Event == EvtAlertRestartFlapping {
			alertCount2++
		}
	}
	if alertCount2 != 1 {
		t.Fatalf("冷却期内不应重复告警，期望 1 次，实际 %d 次", alertCount2)
	}
}

func TestAlertUpstreamFailingResetsOnSuccess(t *testing.T) {
	dir := t.TempDir()
	r := NewRecorder(dir, 30)
	t.Cleanup(r.Close)
	r.alerter = newAlerter(r)
	r.alerter.upstreamThreshold = 3

	for i := 0; i < 3; i++ {
		r.Emit(LevelError, EvtUpstreamError, nil)
	}
	alertCount := 0
	for _, e := range r.Recent(50) {
		if e.Event == EvtAlertUpstreamFailing {
			alertCount++
		}
	}
	if alertCount != 1 {
		t.Fatalf("连续 3 次上游失败应触发告警，实际 %d 次", alertCount)
	}
	// 一轮成功结束 → 计数清零
	r.Emit(LevelInfo, EvtAgentTurnEnd, map[string]any{"ok": true})
	// 再 2 次失败（未达阈值）→ 不应告警
	for i := 0; i < 2; i++ {
		r.Emit(LevelError, EvtUpstreamError, nil)
	}
	total := 0
	for _, e := range r.Recent(50) {
		if e.Event == EvtAlertUpstreamFailing {
			total++
		}
	}
	if total != 1 {
		t.Fatalf("成功后清零，再 2 次失败不应告警，期望仍为 1 次，实际 %d 次", total)
	}
}
