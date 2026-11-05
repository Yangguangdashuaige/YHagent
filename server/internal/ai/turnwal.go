// internal/ai/turnwal.go
//
// 流式回合预写日志（WAL）：每轮对话把已产出的回答/推理分片实时追加到
// data/turns/<turn_id>.jsonl，进程崩溃 / 客户端断连后可回放已生成内容，
// 避免重新消耗上游 token，也让"半截回答"不丢。
//
// turn_id 复用 trace_id（trace 启用时 trace.Start 生成的 "t_<hex>"），
// 保证事件日志 / trace / WAL 三者可按同一 ID 关联；trace 未启用时本地生成同形态 ID。
//
// 崩溃安全：
//   - 每个分片一次 O_APPEND 单行写（小写入在多数文件系统上原子），进程被
//     kill -9 也最多丢最后一个未刷盘的分片；读取端跳过非法 JSON 行， torn 行无害；
//   - 正常结束写 done 标记；凡无 done / interrupted 终止标记的文件 = 中断回合，
//     启动时 RecoverIncomplete 扫描并回放拼回会话历史。
//
// 与持久化的分工：session store 负责"完整回合"的历史（轮末落库），WAL 负责
// "进行中/被打断回合"的分片兜底。两者配合实现"随时重启可恢复"。
package ai

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// WAL 条目 kind 常量
const (
	walKindBegin       = "begin"       // 回合开始（首行，含 session/query）
	walKindChunk       = "chunk"       // 一段回答文本分片
	walKindThought     = "thought"     // 一段推理流分片（思考模型）
	walKindDone        = "done"        // 回合正常结束
	walKindInterrupted = "interrupted" // 回合被中断（崩溃/断连/取消）
)

// defaultKeepHours WAL 文件默认保留时长（TURN_WAL_KEEP_HOURS 可覆盖）
const defaultKeepHours = 24

// walReadLimit 单个 WAL 读取字节上限，防异常大文件占满内存
const walReadLimit = 16 << 20

// WALEntry WAL 中的一行
type WALEntry struct {
	Seq       int    `json:"seq"`                  // 回合内单调递增序号（0 起）
	TS        string `json:"ts"`                   // RFC3339
	Kind      string `json:"kind"`                 // begin / chunk / thought / done / interrupted
	Text      string `json:"text,omitempty"`       // chunk/thought 文本
	SessionID string `json:"session_id,omitempty"` // begin 行携带
	Query     string `json:"query,omitempty"`      // begin 行携带（提问预览，便于排查）
	Reason    string `json:"reason,omitempty"`     // interrupted 行携带
}

// RecoveredTurn 启动恢复扫描得到的一条中断回合（供 main 拼回会话历史）
type RecoveredTurn struct {
	TurnID    string
	SessionID string
	Query     string
	Answer    string // 已生成的回答分片拼接结果
	Reason    string
}

// DefaultTurnStore 全局 WAL 存储；main 装配，nil = 关闭 WAL（全链路 no-op）
var DefaultTurnStore *TurnStore

// TurnStore WAL 文件管理器（按 turn_id 分文件，可并发读写不同文件）
type TurnStore struct {
	dir       string
	keepHours int
	stop      chan struct{}
}

// NewTurnStore 创建 WAL 存储并启动过期清理循环；失败返回 nil。
func NewTurnStore(dir string, keepHours int) *TurnStore {
	if dir == "" {
		dir = "data/turns"
	}
	if keepHours <= 0 {
		keepHours = defaultKeepHours
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil
	}
	s := &TurnStore{dir: dir, keepHours: keepHours, stop: make(chan struct{})}
	go s.cleanupLoop()
	return s
}

// Open 为一回合打开（或新建）WAL 文件并写入 begin 行。
// store 为 nil 时返回 nil（后续所有方法 nil 安全 no-op）。
func (s *TurnStore) Open(turnID, sessionID, query string) *TurnWriter {
	if s == nil || turnID == "" {
		return nil
	}
	f, err := os.OpenFile(s.path(turnID), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return nil // WAL 失败不阻断对话，仅丧失回放能力
	}
	w := &TurnWriter{store: s, turnID: turnID, file: f}
	w.write(&WALEntry{Kind: walKindBegin, SessionID: sessionID, Query: queryPreview(query)})
	return w
}

// path 回合 WAL 文件路径
func (s *TurnStore) path(turnID string) string {
	return filepath.Join(s.dir, turnID+".jsonl")
}

// Replay 读取回合 WAL，返回序号 > afterSeq 的条目（按 seq 升序）。
// 用于断线续传：客户端上报已收到的 seq，服务端只补发之后的分片。
func (s *TurnStore) Replay(turnID string, afterSeq int) []WALEntry {
	if s == nil || turnID == "" {
		return nil
	}
	all := s.readAll(turnID)
	if len(all) == 0 {
		return nil
	}
	out := make([]WALEntry, 0, len(all))
	for _, e := range all {
		if e.Seq > afterSeq {
			out = append(out, e)
		}
	}
	return out
}

// ReplayAll 读取回合全部条目（按 seq 升序），供恢复扫描使用
func (s *TurnStore) ReplayAll(turnID string) []WALEntry {
	return s.readAll(turnID)
}

// State 返回回合是否存在、是否已结束（含 done 或 interrupted 终止标记）
func (s *TurnStore) State(turnID string) (exists, complete bool) {
	if s == nil || turnID == "" {
		return false, false
	}
	all := s.readAll(turnID)
	if len(all) == 0 {
		return false, false
	}
	last := all[len(all)-1]
	return true, last.Kind == walKindDone || last.Kind == walKindInterrupted
}

// readAll 读取整个 WAL 文件（缓冲读，上限 walReadLimit），非法行跳过，按 seq 升序返回
func (s *TurnStore) readAll(turnID string) []WALEntry {
	f, err := os.Open(s.path(turnID))
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []WALEntry
	sc := bufio.NewScanner(io.LimitReader(f, walReadLimit))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		var e WALEntry
		if json.Unmarshal(sc.Bytes(), &e) == nil {
			out = append(out, e)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Seq < out[j].Seq })
	return out
}

// RecoverIncomplete 扫描所有无终止标记的 WAL，回放并标记为 interrupted。
// 返回值供调用方把半截回答拼回对应会话的持久化历史。maxAge 限定只处理
// 最近一段时间内的中断（太老的按过期清理掉，不再打扰用户）。
func (s *TurnStore) RecoverIncomplete(maxAge time.Duration) []RecoveredTurn {
	if s == nil {
		return nil
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil
	}
	cutoff := time.Now().Add(-maxAge)
	var recovered []RecoveredTurn
	for _, ent := range entries {
		name := ent.Name()
		if ent.IsDir() || !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		turnID := strings.TrimSuffix(name, ".jsonl")
		if info, ierr := ent.Info(); ierr == nil && info.ModTime().Before(cutoff) {
			continue // 太老的跳过（cleanupLoop 会删掉）
		}
		all := s.readAll(turnID)
		if len(all) == 0 {
			continue
		}
		last := all[len(all)-1]
		if last.Kind == walKindDone || last.Kind == walKindInterrupted {
			continue // 已有终止标记，无需恢复
		}
		// 拼回回答文本（仅 chunk，不含 thought）
		var sessionID, query string
		var sb strings.Builder
		for _, e := range all {
			switch e.Kind {
			case walKindBegin:
				sessionID, query = e.SessionID, e.Query
			case walKindChunk:
				sb.WriteString(e.Text)
			}
		}
		reason := "服务重启，上一轮回答未完成"
		// 追加 interrupted 标记，避免重复恢复；写失败也不阻断（最多下次再扫一次）
		s.appendMarker(turnID, &WALEntry{Kind: walKindInterrupted, Reason: reason})
		recovered = append(recovered, RecoveredTurn{
			TurnID:    turnID,
			SessionID: sessionID,
			Query:     query,
			Answer:    sb.String(),
			Reason:    reason,
		})
	}
	return recovered
}

// appendMarker 向已存在的 WAL 追加一条终止标记（恢复扫描用）
func (s *TurnStore) appendMarker(turnID string, e *WALEntry) {
	f, err := os.OpenFile(s.path(turnID), os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	e.TS = time.Now().Format(time.RFC3339)
	// 序号取文件已有最大 seq + 1，保证 Replay 升序正确
	if all := s.readAll(turnID); len(all) > 0 {
		e.Seq = all[len(all)-1].Seq + 1
	}
	line, err := json.Marshal(e)
	if err != nil {
		return
	}
	_, _ = f.Write(append(line, '\n'))
}

// Cleanup 删除指定回合的 WAL 文件（回合彻底结束且无需回放时调用，可选）
func (s *TurnStore) Cleanup(turnID string) {
	if s == nil || turnID == "" {
		return
	}
	_ = os.Remove(s.path(turnID))
}

// Close 停止清理循环
func (s *TurnStore) Close() {
	if s == nil {
		return
	}
	select {
	case <-s.stop:
	default:
		close(s.stop)
	}
}

// cleanupLoop 定期删除超过保留期的 WAL 文件
func (s *TurnStore) cleanupLoop() {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			s.sweepOld()
		}
	}
}

// sweepOld 删除修改时间超过保留期的 WAL 文件
func (s *TurnStore) sweepOld() {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-time.Duration(s.keepHours) * time.Hour)
	for _, ent := range entries {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".jsonl") {
			continue
		}
		info, err := ent.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(s.dir, ent.Name()))
		}
	}
}

// ---- TurnWriter：单回合写入器 ----

// TurnWriter 单个回合的 WAL 写入器（一回合一个，单 goroutine 追加）
type TurnWriter struct {
	store  *TurnStore
	turnID string
	mu     sync.Mutex
	file   *os.File
	seq    int
	closed bool
}

// write 追加一条 WAL 条目（自动分配 seq 与时间戳）
func (w *TurnWriter) write(e *WALEntry) {
	if w == nil || w.file == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return
	}
	e.Seq = w.seq
	w.seq++
	e.TS = time.Now().Format(time.RFC3339)
	line, err := json.Marshal(e)
	if err != nil {
		return
	}
	if _, err := w.file.Write(append(line, '\n')); err != nil {
		// 写失败关闭文件，后续写入降级为 no-op（不阻断对话）
		_ = w.file.Close()
		w.file = nil
		w.closed = true
	}
}

// AppendChunk 追加一段回答文本分片
func (w *TurnWriter) AppendChunk(text string) {
	w.write(&WALEntry{Kind: walKindChunk, Text: text})
}

// AppendThought 追加一段推理流分片
func (w *TurnWriter) AppendThought(text string) {
	w.write(&WALEntry{Kind: walKindThought, Text: text})
}

// MarkDone 标记回合正常结束（之后文件只读，供回放）
func (w *TurnWriter) MarkDone() {
	w.write(&WALEntry{Kind: walKindDone})
	w.closeFile()
}

// MarkInterrupted 标记回合被中断（reason 说明原因）
func (w *TurnWriter) MarkInterrupted(reason string) {
	w.write(&WALEntry{Kind: walKindInterrupted, Reason: reason})
	w.closeFile()
}

// TurnID 返回回合标识
func (w *TurnWriter) TurnID() string {
	if w == nil {
		return ""
	}
	return w.turnID
}

// closeFile 关闭底层文件句柄（标记完成后调用，Windows 下及时释放句柄）
func (w *TurnWriter) closeFile() {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file != nil {
		_ = w.file.Close()
		w.file = nil
	}
	w.closed = true
}

// ---- 辅助 ----

// newTurnID 生成回合标识（trace 未启用时使用，形态与 trace.Start 一致）
func newTurnID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return "t_" + hex.EncodeToString(b[:])
}

// queryPreview 提问预览（截断到 200 字符，避免 WAL 膨胀）
func queryPreview(q string) string {
	r := []rune(strings.TrimSpace(q))
	if len(r) > 200 {
		return string(r[:200]) + "…"
	}
	return string(r)
}
