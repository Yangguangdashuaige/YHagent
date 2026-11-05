// internal/eventlog/eventlog.go
//
// 结构化事件日志：把服务生命周期、对话轮次、上游异常、崩溃恢复等关键事件
// 以 JSONL 追加写落盘（data/events/events-YYYYMMDD.jsonl，按天轮转，保留 N 天）。
//
// 与 internal/trace 互补：trace 记录单次请求的性能明细，eventlog 记录跨请求的
// 业务/运维事件（服务启停、崩溃恢复、告警），供 GET /v1/events 查看与运维告警。
//
// 设计约束（与 trace 包同范式，避免两套可观测性机制漂移）：
//   - 零三方依赖、nil 安全（Recorder 未启用时 Emit 全链路 no-op）；
//   - 追加写 + 按天轮转 + 过期清理，内存不积压，契合低内存目标；
//   - Emit 失败只影响可观测性，绝不阻断业务链路；
//   - 事件字段用 map[string]any 透传，新增事件类型无需改结构。
package eventlog

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	LevelInfo  = "info"
	LevelWarn  = "warn"
	LevelError = "error"

	// defaultKeepDays 默认保留天数（EVENTLOG_KEEP_DAYS 可覆盖）
	defaultKeepDays = 30
	// recentLimit 单次读取字节上限，防止异常大文件占满内存
	recentLimit = 8 << 20
)

// Event 一条结构化事件（JSONL 一行）
type Event struct {
	TS        string         `json:"ts"`                   // RFC3339
	Level     string         `json:"level"`                // info / warn / error
	Event     string         `json:"event"`                // 事件类型，如 service.start / agent.turn_begin
	TraceID   string         `json:"trace_id,omitempty"`   // 关联的请求 trace（无则空）
	SessionID string         `json:"session_id,omitempty"` // 关联的会话（无则空）
	Fields    map[string]any `json:"fields,omitempty"`     // 事件附带字段（任意结构）
}

// Default 全局 Recorder；main 启动时装配，nil = 事件日志关闭（全链路 no-op）
var Default *Recorder

// Recorder 事件日志记录器，可并发使用（与 trace.Recorder 同构）
type Recorder struct {
	mu       sync.Mutex
	dir      string
	keepDays int
	day      string // 当前写入文件对应的日期（20060102）
	file     *os.File
	alerter  *alerter // 可选：异常模式检测 → 产出 alert.* 事件（nil = 关闭告警）
}

// NewRecorder 创建记录器并初始化当日文件；失败返回 nil（调用方拿到"关闭"状态）。
// dir 为空用 data/events；keepDays<=0 用默认 30。
func NewRecorder(dir string, keepDays int) *Recorder {
	if dir == "" {
		dir = "data/events"
	}
	if keepDays <= 0 {
		keepDays = defaultKeepDays
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil
	}
	r := &Recorder{dir: dir, keepDays: keepDays}
	if err := r.openLocked(time.Now()); err != nil {
		return nil
	}
	go r.cleanupLoop()
	return r
}

// fileName 单日事件日志文件名
func fileName(day string) string { return "events-" + day + ".jsonl" }

// openLocked 打开（或切换到）指定日期的文件；跨天时顺带清理过期文件。调用方持锁。
func (r *Recorder) openLocked(now time.Time) error {
	day := now.Format("20060102")
	if r.file != nil && day == r.day {
		return nil
	}
	if r.file != nil {
		_ = r.file.Close()
	}
	f, err := os.OpenFile(filepath.Join(r.dir, fileName(day)), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	r.file, r.day = f, day
	r.cleanupExpiredLocked(now)
	return nil
}

// writeLocked 追加一条事件（调用方持锁；不触发告警，供告警器回写避免递归）
func (r *Recorder) writeLocked(e *Event) {
	if e == nil {
		return
	}
	if e.TS == "" {
		e.TS = time.Now().Format(time.RFC3339)
	}
	line, err := json.Marshal(e)
	if err != nil {
		return
	}
	if err := r.openLocked(time.Now()); err != nil {
		return // 落盘失败只影响可观测性
	}
	_, _ = r.file.Write(append(line, '\n'))
}

// Emit 记录一条事件。r 为 nil 时直接返回（全链路 no-op）。
// level/event 为空时分别兜底为 info / "event"，避免日志里出现空类型。
// fields 可为 nil；调用方负责把 trace_id/session_id 放进 fields 或单独传（见 EmitWith）。
func (r *Recorder) Emit(level, event string, fields map[string]any) {
	r.EmitWith(level, event, "", "", fields)
}

// EmitWith 记录一条事件并显式关联 trace / session（Agent 侧埋点用）。
func (r *Recorder) EmitWith(level, event, traceID, sessionID string, fields map[string]any) {
	if r == nil {
		return
	}
	if level == "" {
		level = LevelInfo
	}
	if event == "" {
		event = "event"
	}
	e := &Event{
		Level:     level,
		Event:     event,
		TraceID:   traceID,
		SessionID: sessionID,
		Fields:    fields,
	}
	r.mu.Lock()
	r.writeLocked(e)
	alerter := r.alerter
	r.mu.Unlock()
	if alerter != nil {
		alerter.observe(*e)
	}
}

// emitAlertLocked 告警器回写 alert.* 事件（持锁调用，不递归触发告警）。
func (r *Recorder) emitAlertLocked(event string, fields map[string]any) {
	r.writeLocked(&Event{Level: LevelError, Event: event, Fields: fields})
}

// writeAlert 告警器回写 alert.* 事件的解锁入口（获取锁后调用 emitAlertLocked）。
// 走 writeLocked 而非 Emit，避免触发 alerter.observe 造成递归。
func (r *Recorder) writeAlert(event string, fields map[string]any) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.emitAlertLocked(event, fields)
}

// Recent 从最新文件开始倒序读取最近 n 条事件（/v1/events 数据源）。
// r 为 nil 或目录无文件时返回空切片。
func (r *Recorder) Recent(n int) []Event {
	if r == nil || n <= 0 {
		return nil
	}
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "events-") && strings.HasSuffix(e.Name(), ".jsonl") {
			names = append(names, e.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names))) // 文件名含日期，字典序倒序 = 时间倒序

	var out []Event
	for _, name := range names {
		if len(out) >= n {
			break
		}
		out = append(out, readTail(filepath.Join(r.dir, name), n-len(out))...)
	}
	return out
}

// readTail 读取单个 JSONL 文件的最后 max 条（缓冲读全文，上限 recentLimit 字节）
func readTail(path string, max int) []Event {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var lines []string
	sc := bufio.NewScanner(io.LimitReader(f, recentLimit))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	out := make([]Event, 0, max)
	for i := len(lines) - 1; i >= 0 && len(out) < max; i-- {
		var e Event
		if json.Unmarshal([]byte(lines[i]), &e) == nil {
			out = append(out, e)
		}
	}
	return out
}

// cleanupExpiredLocked 删除保留窗口之外的事件文件。调用方持锁。
func (r *Recorder) cleanupExpiredLocked(now time.Time) {
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		return
	}
	cutoff := now.AddDate(0, 0, -r.keepDays).Format("20060102")
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, "events-") || !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		day := strings.TrimSuffix(strings.TrimPrefix(name, "events-"), ".jsonl")
		if len(day) == 8 && day < cutoff {
			_ = os.Remove(filepath.Join(r.dir, name))
		}
	}
}

// Close 关闭当前打开的文件（优雅停机 / 测试清理用）
func (r *Recorder) Close() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.file != nil {
		_ = r.file.Close()
		r.file = nil
	}
}

// cleanupLoop 定期清理（除跨天轮转外，长时间不重启的进程也能清掉旧文件）
func (r *Recorder) cleanupLoop() {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for range ticker.C {
		r.mu.Lock()
		r.cleanupExpiredLocked(time.Now())
		r.mu.Unlock()
	}
}

// ---- 包级便捷函数：全局 Default 未装配时全部 no-op ----

// Emit 向全局 Recorder 写一条事件（Default 为 nil 时静默跳过）
func Emit(level, event string, fields map[string]any) {
	if Default != nil {
		Default.Emit(level, event, fields)
	}
}

// EmitWith 向全局 Recorder 写一条带 trace/session 关联的事件
func EmitWith(level, event, traceID, sessionID string, fields map[string]any) {
	if Default != nil {
		Default.EmitWith(level, event, traceID, sessionID, fields)
	}
}
