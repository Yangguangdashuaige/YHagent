package ai

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTurnWALAppendAndReplay(t *testing.T) {
	store := NewTurnStore(t.TempDir(), 1)
	if store == nil {
		t.Fatalf("NewTurnStore 失败")
	}
	t.Cleanup(store.Close)

	w := store.Open("t_001", "sess_1", "M14 改枪方案")
	if w == nil {
		t.Fatalf("Open 失败")
	}
	w.AppendChunk("第一个")
	w.AppendThought("推理片段")
	w.AppendChunk("第二个")
	w.MarkDone()

	all := store.ReplayAll("t_001")
	if len(all) != 5 { // begin + 2 chunk + thought + done
		t.Fatalf("期望 5 条，实际 %d: %+v", len(all), all)
	}
	// seq 单调递增
	for i := 1; i < len(all); i++ {
		if all[i].Seq <= all[i-1].Seq {
			t.Fatalf("seq 应单调递增: %+v", all)
		}
	}
	if all[0].Kind != walKindBegin || all[0].SessionID != "sess_1" || all[0].Query != "M14 改枪方案" {
		t.Fatalf("begin 行异常: %+v", all[0])
	}
	exists, complete := store.State("t_001")
	if !exists || !complete {
		t.Fatalf("done 后应 complete")
	}

	// afterSeq 回放：只补发序号 > 1 的
	got := store.Replay("t_001", 1)
	if len(got) != 3 { // thought(seq2) + chunk(seq3) + done(seq4)
		t.Fatalf("Replay(after=1) 期望 3 条，实际 %d: %+v", len(got), got)
	}
	// 拼接回答文本
	var sb string
	for _, e := range store.ReplayAll("t_001") {
		if e.Kind == walKindChunk {
			sb += e.Text
		}
	}
	if sb != "第一个第二个" {
		t.Fatalf("回答拼接异常: %q", sb)
	}
}

func TestTurnWALNilSafe(t *testing.T) {
	var s *TurnStore
	if s.Replay("x", 0) != nil {
		t.Fatalf("nil store Replay 应返回 nil")
	}
	var w *TurnWriter
	w.AppendChunk("x") // 不应 panic
	w.MarkDone()
	w.MarkInterrupted("r")
	if w.TurnID() != "" {
		t.Fatalf("nil writer TurnID 应为空")
	}
	DefaultTurnStore = nil
	if DefaultTurnStore.Open("x", "s", "q") != nil {
		t.Fatalf("nil DefaultTurnStore.Open 应返回 nil")
	}
}

func TestTurnWALRecoverIncomplete(t *testing.T) {
	dir := t.TempDir()
	store := NewTurnStore(dir, 1)
	t.Cleanup(store.Close)

	// 一个中断回合（无 done 标记）：模拟崩溃后残留
	w := store.Open("t_crash", "sess_x", "怎么改枪")
	w.AppendChunk("已生成一半")
	w.AppendChunk("的回答")
	// 不调用 MarkDone / closeFile，模拟进程被杀

	// 一个正常回合（不应被恢复）
	w2 := store.Open("t_ok", "sess_y", "另一个问题")
	w2.AppendChunk("完整回答")
	w2.MarkDone()

	// 一个太老的中断回合（应被 maxAge 过滤）
	old := filepath.Join(dir, "t_old.jsonl")
	_ = os.WriteFile(old, []byte(`{"seq":0,"kind":"begin","session_id":"sess_z","query":"老问题"}`+"\n"+
		`{"seq":1,"kind":"chunk","text":"老内容"}`+"\n"), 0o644)
	oldTime := time.Now().Add(-48 * time.Hour)
	_ = os.Chtimes(old, oldTime, oldTime)

	rec := store.RecoverIncomplete(24 * time.Hour)
	if len(rec) != 1 {
		t.Fatalf("期望恢复 1 条中断回合，实际 %d: %+v", len(rec), rec)
	}
	if rec[0].TurnID != "t_crash" || rec[0].SessionID != "sess_x" {
		t.Fatalf("恢复条目异常: %+v", rec[0])
	}
	if rec[0].Answer != "已生成一半的回答" {
		t.Fatalf("恢复回答拼接异常: %q", rec[0].Answer)
	}
	// 恢复后该回合应被标记 interrupted（不再重复恢复）
	exists, complete := store.State("t_crash")
	if !exists || !complete {
		t.Fatalf("恢复后应标记 complete")
	}
	rec2 := store.RecoverIncomplete(24 * time.Hour)
	if len(rec2) != 0 {
		t.Fatalf("已恢复的回合不应重复恢复，实际 %d", len(rec2))
	}
	// 释放模拟崩溃残留的 writer 句柄（Windows 下 TempDir 才能清理）
	w.closeFile()
}

func TestTurnWALCleanupOld(t *testing.T) {
	dir := t.TempDir()
	store := NewTurnStore(dir, 1) // keep 1h
	t.Cleanup(store.Close)

	old := filepath.Join(dir, "t_old.jsonl")
	_ = os.WriteFile(old, []byte("{}\n"), 0o644)
	oldTime := time.Now().Add(-2 * time.Hour)
	_ = os.Chtimes(old, oldTime, oldTime)

	fresh := filepath.Join(dir, "t_fresh.jsonl")
	_ = os.WriteFile(fresh, []byte("{}\n"), 0o644)

	store.sweepOld()
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("过期 WAL 未被清理")
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Fatalf("新鲜 WAL 不应被清理: %v", err)
	}
}
