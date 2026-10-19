// internal/trace/trace.go
//
// 最小可观测性层：把每次 /v1/chat 请求的完整执行过程记录成一条结构化 trace，
// 以 JSONL 追加写落盘（data/trace/trace-YYYYMMDD.jsonl，按天轮转，保留 N 天）。
//
// 记录内容：
//   - LLM 调用轮数、每轮耗时与 token 用量（豆包流式响应 final chunk 的 usage）
//   - 每个工具调用的耗时 / 入参摘要 / 结果字节数 / 成败
//   - 三角洲 API 进程内缓存命中情况（经 context 传播，仅 delta_* 工具有值）
//   - 总延迟、首 token 延迟（TTFT）、回答字数、是否成功
//
// 设计约束：
//   - 零三方依赖、nil 安全（Recorder 未启用时全链路 no-op，Agent 侧无需判空）；
//   - 追加写 + 按天轮转，内存不积压，契合低内存目标；
//   - 查看端 GET /v1/traces 读 JSONL 尾部 N 条，无需额外存储引擎。
package trace

import (
	"bufio"
	"context"
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
	"unicode/utf8"
)

// queryPreviewMax trace 里用户提问的预览长度（字节），超长截断，避免日志膨胀
const queryPreviewMax = 256

// argsPreviewMax 工具入参摘要长度（字节），与 agent 日志的 previewArgs 口径一致
const argsPreviewMax = 300

// defaultKeepDays trace 文件保留天数（TRACE_KEEP_DAYS 可覆盖）
const defaultKeepDays = 7

// LLMRound 一轮豆包流式调用的观测数据
type LLMRound struct {
	Round            int   `json:"round"`                       // 第几轮（1 起）
	MS               int64 `json:"ms"`                          // 该轮总耗时
	PromptTokens     int32 `json:"prompt_tokens,omitempty"`     // 输入 token（上游未返回 usage 时为 0）
	CompletionTokens int32 `json:"completion_tokens,omitempty"` // 输出 token
	TotalTokens      int32 `json:"total_tokens,omitempty"`
}

// ToolCall 一次工具调用的观测数据
type ToolCall struct {
	Round       int    `json:"round"`               // 发生在第几轮 LLM 调用
	Name        string `json:"name"`                // 工具名
	Args        string `json:"args,omitempty"`      // 入参摘要（截断）
	MS          int64  `json:"ms"`                  // 执行耗时
	ResultBytes int    `json:"result_bytes"`        // 结果 JSON 字节数
	OK          bool   `json:"ok"`                  // 是否成功（失败/超时/截断降级均为 false）
	CacheHit    *bool  `json:"cache_hit,omitempty"` // 三角洲缓存命中（非 delta 工具无此字段）
}

// CompactInfo 一次上下文压缩的观测数据（会话超 token 预算时触发）
type CompactInfo struct {
	Messages int   `json:"messages"` // 被压缩的消息条数
	Before   int   `json:"before"`   // 压缩前历史估算 token
	After    int   `json:"after"`    // 压缩后历史估算 token（摘要+保留段）
	MS       int64 `json:"ms"`       // 压缩耗时（含一次 LLM 摘要调用）
}

// Record 单次请求的完整 trace（JSONL 一行）
type Record struct {
	TraceID     string       `json:"trace_id"`
	Time        string       `json:"ts"` // RFC3339
	SessionID   string       `json:"session_id"`
	Query       string       `json:"query"`
	Mode        string       `json:"mode,omitempty"` // 执行范式：reactive（反应式）/ plan（规划式，调用了 plan_and_execute）
	Images      int          `json:"images"`         // 携带图片数
	OK          bool         `json:"ok"`
	Error       string       `json:"error,omitempty"`
	TotalMS     int64        `json:"total_ms"`     // 请求总耗时
	FirstTokMS  int64        `json:"first_tok_ms"` // 首 token 延迟（TTFT），0 = 未产生回答
	LLMRounds   int          `json:"llm_rounds"`   // LLM 调用轮数
	LLM         []LLMRound   `json:"llm,omitempty"`
	Tools       []ToolCall   `json:"tools,omitempty"`
	Compact     *CompactInfo `json:"compact,omitempty"` // 本次请求触发的上下文压缩
	CacheHits   int          `json:"cache_hits"`        // delta 缓存命中次数
	CacheMisses int          `json:"cache_misses"`      // delta 缓存未命中次数
	AnswerChars int          `json:"answer_chars"`      // 最终回答字符数
}

// Span 请求级 trace 累积器。Agent 在请求入口 Start，结束时 End 落盘；
// 中间各埋点通过 context 拿到 Span（可能为 nil，全部方法 nil 安全）。
// 同轮多个工具并行执行时会并发调用各记录方法，内部用互斥锁保证安全。
type Span struct {
	mu         sync.Mutex
	rec        *Recorder
	traceID    string
	sessionID  string
	query      string
	images     int
	start      time.Time
	firstChunk time.Time
	llm        []LLMRound
	tools      []ToolCall
	compact    *CompactInfo
	mode       string
	cacheHits  int
	cacheMiss  int
	answer     int // 回答字符数
}

type spanKey struct{}

// Start 为一次请求开启 trace。全局 Recorder 未启用时返回 nil（后续所有方法 no-op）。
func Start(sessionID, query string, images int) *Span {
	r := Default
	if r == nil {
		return nil
	}
	var b [8]byte
	_, _ = rand.Read(b[:])
	return &Span{
		rec:       r,
		traceID:   "t_" + hex.EncodeToString(b[:]),
		sessionID: sessionID,
		query:     preview(query, queryPreviewMax),
		images:    images,
		start:     time.Now(),
	}
}

// WithSpan 把 Span 注入 context（span 为 nil 时原样返回）
func WithSpan(ctx context.Context, span *Span) context.Context {
	if span == nil {
		return ctx
	}
	return context.WithValue(ctx, spanKey{}, span)
}

// FromContext 取出当前请求的 Span；未注入时返回 nil
func FromContext(ctx context.Context) *Span {
	if ctx == nil {
		return nil
	}
	span, _ := ctx.Value(spanKey{}).(*Span)
	return span
}

// LLMRound 记录一轮 LLM 调用（耗时 + usage）
func (s *Span) LLMRound(ms int64, prompt, completion, total int32) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.llm = append(s.llm, LLMRound{
		Round:            len(s.llm) + 1,
		MS:               ms,
		PromptTokens:     prompt,
		CompletionTokens: completion,
		TotalTokens:      total,
	})
}

// FirstToken 记录首 token 时刻（只记第一次）
func (s *Span) FirstToken() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.firstChunk.IsZero() {
		s.firstChunk = time.Now()
	}
}

// ToolCall 记录一次工具调用
func (s *Span) ToolCall(round int, name, args string, ms int64, resultBytes int, ok bool) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tools = append(s.tools, ToolCall{
		Round:       round,
		Name:        name,
		Args:        preview(args, argsPreviewMax),
		MS:          ms,
		ResultBytes: resultBytes,
		OK:          ok,
	})
}

// NoteCacheHit 记录一次三角洲缓存命中
func (s *Span) NoteCacheHit() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cacheHits++
}

// NoteCacheMiss 记录一次三角洲缓存未命中（含 singleflight 合并等待的请求）
func (s *Span) NoteCacheMiss() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cacheMiss++
}

// NoteMode 标记执行范式（reactive / plan），观测页按此分组对比两种模式
func (s *Span) NoteMode(mode string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mode = mode
}

// NoteCompact 记录一次上下文压缩（token 预算超限触发）
func (s *Span) NoteCompact(messages, before, after int, ms int64) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.compact = &CompactInfo{Messages: messages, Before: before, After: after, MS: ms}
}

// AnswerChars 记录最终回答的字符数
func (s *Span) AnswerChars(n int) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.answer = n
}

// End 结束请求并落盘。errMsg 非空表示请求失败。
func (s *Span) End(errMsg string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	rec := &Record{
		TraceID:     s.traceID,
		Time:        s.start.Format(time.RFC3339),
		SessionID:   s.sessionID,
		Query:       s.query,
		Mode:        s.mode,
		Images:      s.images,
		OK:          errMsg == "",
		Error:       errMsg,
		TotalMS:     time.Since(s.start).Milliseconds(),
		LLMRounds:   len(s.llm),
		LLM:         s.llm,
		Tools:       s.tools,
		Compact:     s.compact,
		CacheHits:   s.cacheHits,
		CacheMisses: s.cacheMiss,
		AnswerChars: s.answer,
	}
	if !s.firstChunk.IsZero() {
		rec.FirstTokMS = s.firstChunk.Sub(s.start).Milliseconds()
	}
	s.mu.Unlock()
	s.rec.Write(rec)
}

// preview 压缩空白并按字节截断（UTF-8 边界对齐到合法字符）
func preview(s string, max int) string {
	s = strings.Join(strings.Fields(s), "")
	if len(s) <= max {
		return s
	}
	cut := s[:max]
	// 尾部最多只有一个被截断的多字节字符，回退最多 3 字节即可恢复合法边界；
	// 若入参本身含非法 UTF-8（极端用户输入），回退 3 次后交由 JSON 编码兜底替换
	for i := 0; i < 3 && len(cut) > 0 && !utf8.ValidString(cut); i++ {
		cut = cut[:len(cut)-1]
	}
	return cut + "…(截断)"
}

// ============================================================================
// Recorder：JSONL 追加写 + 按天轮转 + 过期清理 + 尾部读取
// ============================================================================

// Default 全局 Recorder；main 启动时装配，nil = trace 关闭（全链路 no-op）
var Default *Recorder

// Recorder trace 文件记录器，可并发使用
type Recorder struct {
	mu       sync.Mutex
	dir      string
	keepDays int
	day      string // 当前写入文件对应的日期（20060102）
	file     *os.File
}

// NewRecorder 创建记录器并初始化当日文件；失败返回 nil（调用方拿到的就是"关闭"状态）。
func NewRecorder(dir string, keepDays int) *Recorder {
	if dir == "" {
		dir = "data/trace"
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

// fileName 单日 trace 文件名
func fileName(day string) string { return "trace-" + day + ".jsonl" }

// openLocked 打开（或切换到）指定日期的 trace 文件；跨天时顺带清理过期文件。调用方持锁。
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

// Write 追加一条 trace（内部自动按天轮转）
func (r *Recorder) Write(rec *Record) {
	if r == nil || rec == nil {
		return
	}
	line, err := json.Marshal(rec)
	if err != nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.openLocked(time.Now()); err != nil {
		return // 落盘失败只影响可观测性，不影响业务
	}
	_, _ = r.file.Write(append(line, '\n'))
}

// RecentLimit 单次读取的字节上限（防止异常大文件占用内存）
const RecentLimit = 8 << 20

// Recent 从最新文件开始倒序读取最近 n 条 trace（查看页数据源）。
// r 为 nil 或目录无文件时返回空切片。
func (r *Recorder) Recent(n int) []Record {
	if r == nil || n <= 0 {
		return nil
	}
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "trace-") && strings.HasSuffix(e.Name(), ".jsonl") {
			names = append(names, e.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names))) // 文件名含日期，字典序倒序 = 时间倒序

	var out []Record
	for _, name := range names {
		if len(out) >= n {
			break
		}
		recs := readTail(filepath.Join(r.dir, name), n-len(out))
		out = append(out, recs...)
	}
	return out
}

// readTail 读取单个 JSONL 文件的最后 max 条（缓冲读全文，上限 RecentLimit 字节）
func readTail(path string, max int) []Record {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var lines []string
	sc := bufio.NewScanner(io.LimitReader(f, RecentLimit))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20) // 单条 trace 可达数百 KB（多轮工具调用）
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	out := make([]Record, 0, max)
	for i := len(lines) - 1; i >= 0 && len(out) < max; i-- {
		var rec Record
		if json.Unmarshal([]byte(lines[i]), &rec) == nil {
			out = append(out, rec)
		}
	}
	return out
}

// cleanupExpiredLocked 删除保留窗口之外的 trace 文件。调用方持锁。
func (r *Recorder) cleanupExpiredLocked(now time.Time) {
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		return
	}
	cutoff := now.AddDate(0, 0, -r.keepDays).Format("20060102")
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, "trace-") || !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		day := strings.TrimSuffix(strings.TrimPrefix(name, "trace-"), ".jsonl")
		if len(day) == 8 && day < cutoff {
			_ = os.Remove(filepath.Join(r.dir, name))
		}
	}
}

// Close 关闭当前打开的 trace 文件（优雅停机 / 测试清理用）
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
