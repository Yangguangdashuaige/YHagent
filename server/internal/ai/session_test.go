package ai

import (
	"errors"
	"testing"
)

// TestSessionTurnLock 整轮轮转锁：同一会话第二轮尝试必须失败，释放后可重新获取
func TestSessionTurnLock(t *testing.T) {
	s := newSession("sess_turn_test", "x")
	if !s.tryBeginTurn() {
		t.Fatal("首轮获取轮转锁应成功")
	}
	if s.tryBeginTurn() {
		t.Fatal("轮转锁持有时再次获取应失败（并发请求需被拒绝）")
		s.endTurn()
	}
	s.endTurn()
	if !s.tryBeginTurn() {
		t.Fatal("释放后应能重新获取轮转锁")
	}
	s.endTurn()
}

// TestAgentResetBusy 问答进行中 Reset 必须返回 ErrSessionBusy，不能与对话并发改历史
func TestAgentResetBusy(t *testing.T) {
	a := &Agent{sessions: NewSessionManager(), systemPrompt: "x"}
	const id = "sess_reset_busy_test"

	// 模拟一轮对话持有轮转锁
	sess := a.sessions.GetOrCreate(id, a.systemPrompt)
	if !sess.tryBeginTurn() {
		t.Fatal("预置轮转锁失败")
	}
	if err := a.Reset(id); !errors.Is(err, ErrSessionBusy) {
		t.Fatalf("忙时 Reset 应返回 ErrSessionBusy，实际 %v", err)
	}
	sess.endTurn()

	if err := a.Reset(id); err != nil {
		t.Fatalf("空闲时 Reset 应成功，实际 %v", err)
	}
}

// TestManagerCapEviction 会话数达到上限后新建应淘汰空闲会话，且总数不再增长；
// 忙会话受轮转锁保护不被淘汰（全部忙时允许短暂超限）
func TestManagerCapEviction(t *testing.T) {
	m := NewSessionManager()
	for i := 0; i < maxSessions+5; i++ {
		m.GetOrCreate(sessionTestID(i), "x")
	}
	if got := len(m.sessions); got != maxSessions {
		t.Fatalf("达上限后会话总数应稳定在 %d，实际 %d", maxSessions, got)
	}

	// 占住一个忙会话：新一轮淘汰不应选中它（计数仍稳定即可，忙会话可能留下、
	// 同时淘汰另一个空闲会话）
	busy := m.GetOrCreate(sessionTestID(0), "x")
	if !busy.tryBeginTurn() {
		t.Fatal("忙会话预置失败")
	}
	defer busy.endTurn()
	for i := maxSessions + 5; i < maxSessions+10; i++ {
		m.GetOrCreate(sessionTestID(i), "x")
	}
	if got := len(m.sessions); got > maxSessions {
		t.Fatalf("存在可淘汰空闲会话时总数不应超限，实际 %d", got)
	}
}

func sessionTestID(i int) string {
	// 生成定长十六进制风格 ID（仅测试用，长度足够避免彼此前缀碰撞）
	return "cap_" + padID(i)
}

func padID(i int) string {
	const digits = "0123456789abcdef"
	b := make([]byte, 8)
	for j := 7; j >= 0; j-- {
		b[j] = digits[i%16]
		i /= 16
	}
	return string(b)
}
