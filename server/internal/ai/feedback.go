// internal/ai/feedback.go
//
// 自学习闭环：B 反馈收集 + C few-shot 案例库。
//
// 数据流：
//   前端回答气泡 👍👎 → POST /v1/feedback → FeedbackService.Submit 落 SQLite
//   ├─ 全部反馈（含点踩）进 feedback 表：与 trace_id 关联，供失败模式汇总分析；
//   └─ 点赞问答对自动进 good_cases 案例库（按问题文本去重，后赞覆盖先赞）。
//
// 案例注入（few-shot 自学习）：
//   每轮提问按"字符二元组重合率"检索相似案例（案例量级小，全量载入内存打分），
//   命中的前 3 条拼成「参考成功案例」块，由 Agent 并入本轮 system 消息尾部——
//   仅本次请求生效，不写入会话历史（避免案例块随轮次累积撑爆上下文）。
//
// 安全与成本：
//   - 问题/回答全部经清洗（剥控制字符与换行）后限长落库——LLM/用户文本一律
//     当不可信输入处理，杜绝借案例块伪造 system 结构（提示词注入面）；
//   - 注入块里明确声明"案例只是答法参考"；检索失败/无命中静默降级，不影响对话；
//   - 单库案例上限 500 条（超出按点赞时间淘汰最旧），注入块整体限长。
package ai

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const (
	// maxFeedbackQuestionRunes 反馈问题长度上限（字符）
	maxFeedbackQuestionRunes = 300
	// maxFeedbackAnswerRunes 反馈回答长度上限（字符）：完整回答有留存价值，
	// 但案例注入时还会再截断到展示长度
	maxFeedbackAnswerRunes = 4000
	// maxCaseStore 案例库硬上限（超出按更新时间淘汰最旧）
	maxCaseStore = 500
	// maxCaseInject 单次注入的案例条数
	maxCaseInject = 3
	// caseSimilarityThreshold 相似度门槛（查询二元组重合率低于此值不注入）
	caseSimilarityThreshold = 0.3
	// maxCaseAnswerRunes 注入块里单条案例回答的展示长度（字符）
	maxCaseAnswerRunes = 500
	// maxCaseQuestionRunes 注入块里单条案例问题的展示长度（字符）
	maxCaseQuestionRunes = 200
)

// Feedback 一条用户反馈（Rating: 1=点赞 / -1=点踩）
type Feedback struct {
	TraceID   string
	SessionID string
	UID       int64
	Rating    int
	Question  string
	Answer    string
}

// CaseEntry 案例库中的一条高质量问答对
type CaseEntry struct {
	Question string
	Answer   string
}

// FeedbackStore 反馈闭环持久化后端
type FeedbackStore interface {
	// SaveFeedback 落一条反馈（含点踩，供失败模式分析）
	SaveFeedback(fb Feedback) error
	// UpsertCase 按问题文本去重写入案例（后赞覆盖先赞）
	UpsertCase(question, answer, traceID string) error
	// LoadCases 读取案例库（最新优先，至多 limit 条）
	LoadCases(limit int) ([]CaseEntry, error)
	Close() error
}

// SQLiteFeedbackStore SQLite 实现（单连接串行写 + WAL，账号/会话/记忆库同款策略）
type SQLiteFeedbackStore struct {
	db *sql.DB
}

// NewSQLiteFeedbackStore 打开（或初始化）反馈库；父目录不存在时自动创建
func NewSQLiteFeedbackStore(dbPath string) (*SQLiteFeedbackStore, error) {
	if dir := filepath.Dir(dbPath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("创建反馈库目录: %w", err)
		}
	}
	dsn := "file:" + filepath.ToSlash(dbPath) + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("打开反馈库: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS feedback (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		trace_id   TEXT NOT NULL DEFAULT '',
		session_id TEXT NOT NULL DEFAULT '',
		uid        INTEGER NOT NULL DEFAULT 0,
		rating     INTEGER NOT NULL,
		question   TEXT NOT NULL DEFAULT '',
		answer     TEXT NOT NULL DEFAULT '',
		created_at INTEGER NOT NULL
	); CREATE INDEX IF NOT EXISTS idx_feedback_created ON feedback(created_at);
	CREATE TABLE IF NOT EXISTS good_cases (
		id              INTEGER PRIMARY KEY AUTOINCREMENT,
		question        TEXT NOT NULL UNIQUE,
		answer          TEXT NOT NULL,
		source_trace_id TEXT NOT NULL DEFAULT '',
		updated_at      INTEGER NOT NULL
	)`); err != nil {
		db.Close()
		return nil, fmt.Errorf("初始化反馈表: %w", err)
	}
	return &SQLiteFeedbackStore{db: db}, nil
}

func (s *SQLiteFeedbackStore) SaveFeedback(fb Feedback) error {
	_, err := s.db.Exec(
		`INSERT INTO feedback (trace_id, session_id, uid, rating, question, answer, created_at)
		 VALUES (?,?,?,?,?,?,?)`,
		fb.TraceID, fb.SessionID, fb.UID, fb.Rating, fb.Question, fb.Answer, time.Now().Unix())
	if err != nil {
		return fmt.Errorf("写入反馈: %w", err)
	}
	return nil
}

func (s *SQLiteFeedbackStore) UpsertCase(question, answer, traceID string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("开启案例事务: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(
		`INSERT INTO good_cases (question, answer, source_trace_id, updated_at) VALUES (?,?,?,?)
		 ON CONFLICT(question) DO UPDATE SET answer = excluded.answer,
			source_trace_id = excluded.source_trace_id, updated_at = excluded.updated_at`,
		question, answer, traceID, time.Now().Unix()); err != nil {
		return fmt.Errorf("写入案例: %w", err)
	}
	// 硬上限：超出按更新时间淘汰最旧（案例库膨胀会让检索变慢、注入噪声变多）
	if _, err := tx.Exec(
		`DELETE FROM good_cases WHERE question NOT IN (
			SELECT question FROM good_cases ORDER BY updated_at DESC, question LIMIT ?
		)`, maxCaseStore); err != nil {
		return fmt.Errorf("裁剪案例库: %w", err)
	}
	return tx.Commit()
}

func (s *SQLiteFeedbackStore) LoadCases(limit int) ([]CaseEntry, error) {
	if limit <= 0 {
		limit = maxCaseStore
	}
	rows, err := s.db.Query(
		`SELECT question, answer FROM good_cases ORDER BY updated_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("读取案例库: %w", err)
	}
	defer rows.Close()
	var out []CaseEntry
	for rows.Next() {
		var e CaseEntry
		if err := rows.Scan(&e.Question, &e.Answer); err != nil {
			return nil, fmt.Errorf("扫描案例行: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *SQLiteFeedbackStore) Close() error { return s.db.Close() }

// ---- FeedbackService：提交 + 案例检索注入 ----

// FeedbackService 反馈闭环服务：收集 👍👎，点赞问答进案例库，按相似度渲染注入块
type FeedbackService struct {
	store FeedbackStore
}

// NewFeedbackService 组装反馈服务
func NewFeedbackService(store FeedbackStore) *FeedbackService {
	return &FeedbackService{store: store}
}

// Submit 清洗并落一条反馈；点赞同时把问答对写入案例库。
// 案例写入失败只记日志（反馈本体已落库，不因案例问题拒收反馈）。
func (f *FeedbackService) Submit(fb Feedback) error {
	if fb.Rating != 1 && fb.Rating != -1 {
		return errors.New("rating 只支持 1（赞）或 -1（踩）")
	}
	fb.Question = feedbackClean(fb.Question, maxFeedbackQuestionRunes)
	fb.Answer = feedbackClean(fb.Answer, maxFeedbackAnswerRunes)
	fb.TraceID = strings.TrimSpace(fb.TraceID)
	fb.SessionID = strings.TrimSpace(fb.SessionID)
	if err := f.store.SaveFeedback(fb); err != nil {
		return err
	}
	if fb.Rating == 1 && fb.Question != "" && fb.Answer != "" {
		if err := f.store.UpsertCase(fb.Question, fb.Answer, fb.TraceID); err != nil {
			log.Printf("[feedback] 案例写入失败（反馈已落库）: %v", err)
		}
	}
	if fb.Rating == 1 {
		log.Printf("[feedback] 用户 %d 点赞（案例库已更新）", fb.UID)
	} else {
		log.Printf("[feedback] 用户 %d 点踩 trace=%s", fb.UID, fb.TraceID)
	}
	return nil
}

// CaseBlock 按当前问题检索相似案例并渲染注入 system 尾部的参考块；
// 无命中/检索失败返回空串（对话路径零影响）。
func (f *FeedbackService) CaseBlock(query string) string {
	q := strings.TrimSpace(query)
	if q == "" {
		return ""
	}
	cases, err := f.store.LoadCases(maxCaseStore)
	if err != nil {
		log.Printf("[feedback] 案例库读取失败（本次不注入）: %v", err)
		return ""
	}
	if len(cases) == 0 {
		return ""
	}
	type scored struct {
		entry CaseEntry
		score float64
	}
	var hits []scored
	qg := bigrams(q)
	for _, c := range cases {
		if s := similarity(qg, c.Question); s >= caseSimilarityThreshold {
			hits = append(hits, scored{entry: c, score: s})
		}
	}
	if len(hits) == 0 {
		return ""
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].score > hits[j].score })
	if len(hits) > maxCaseInject {
		hits = hits[:maxCaseInject]
	}
	var b strings.Builder
	b.WriteString("\n\n## 参考成功案例（历史玩家点赞过的优质回答；只学习其结构与答法，数据须以工具最新返回为准，不要照抄其中可能过期的数字）\n")
	for i, h := range hits {
		fmt.Fprintf(&b, "【案例%d】问题：%s\n回答：%s\n",
			i+1,
			feedbackClean(h.entry.Question, maxCaseQuestionRunes),
			feedbackClean(h.entry.Answer, maxCaseAnswerRunes))
	}
	return b.String()
}

// bigrams 提取字符串的字符二元组集合（粗粒度中文相似度，零依赖够用）
func bigrams(s string) map[string]struct{} {
	rs := []rune(strings.Join(strings.Fields(s), ""))
	m := make(map[string]struct{}, len(rs))
	for i := 0; i+1 < len(rs); i++ {
		m[string(rs[i:i+2])] = struct{}{}
	}
	return m
}

// similarity 查询二元组在候选中的重合率（0~1，越高越相关；按查询侧归一化）
func similarity(queryGrams map[string]struct{}, candidate string) float64 {
	if len(queryGrams) == 0 {
		return 0
	}
	cg := bigrams(candidate)
	hit := 0
	for g := range queryGrams {
		if _, ok := cg[g]; ok {
			hit++
		}
	}
	return float64(hit) / float64(len(queryGrams))
}

// feedbackClean 清洗反馈/案例文本：剥换行与控制字符（防伪造 system 结构），
// 压缩空白后按字符数限长
func feedbackClean(s string, maxRunes int) string {
	s = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' {
			return ' '
		}
		if r < 32 {
			return -1
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	rs := []rune(s)
	if len(rs) > maxRunes {
		rs = rs[:maxRunes]
	}
	return string(rs)
}
