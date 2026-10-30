// internal/ai/memory.go
//
// 长期记忆：跨会话的用户事实记忆（分层记忆第三步）。
// 第一步账号系统提供 UID，第二步会话持久化保住单会话历史；
// 本层把"主玩 M14、预算 5 万、新手"这类稳定用户事实沉淀下来，
// 新会话创建时注入 system 提示词——模型跨会话"认识"用户。
//
// 与向量库方案的本质区别（刻意为之）：
//   用户画像是结构化 KV（key 精确匹配），更新语义是覆盖——"预算5万"变"预算8万"
//   必须 upsert 同一个 key，而不是靠相似度并存两条（相似度合并会错杀该覆盖的更新，
//   且语义漂移不可控）。情景记忆（会话摘要）走 RAG 是后续可选项，本层不涉及。
//
// 数据流：
//   整轮成功结束 → 异步 LLM 提取（输入=本轮问答 + 现有记忆清单，输出=JSON 操作数组）
//   → 解析校验清洗 → SQLite upsert/delete（按 user_id+fact_key）
//   → 下次会话创建时拼入 system 提示词。
//
// 安全与成本：
//   - 提取在独立 goroutine（60s 超时、独立 ctx），失败仅记日志，不影响主对话；
//   - key/value 清洗换行与控制字符，杜绝借记忆块伪造 system 结构（提示词注入面）；
//   - key 限 [a-z0-9_] 且 ≤64 字节，value ≤200 字节，单轮操作 ≤20 条，
//     单用户记忆 ≤100 条（超出按更新时间淘汰最旧的）——LLM 输出全部当不可信输入处理。
package ai

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const (
	maxMemoryOps    = 20  // 单轮提取允许的最大操作数
	maxMemoryKeyLen = 64  // fact_key 字节上限
	maxMemoryValLen = 200 // fact_value 字节上限
	maxUserMemories = 100 // 单用户记忆条数硬上限
	extractTimeout  = 60 * time.Second
	// extractInputLimit 提取输入截断：问答各取前 4000 字符（足够覆盖偏好表达，防 prompt 膨胀）
	extractInputLimit = 4000
)

// MemOp 一次记忆操作（LLM 提取输出；op ∈ upsert/delete）
type MemOp struct {
	Op    string `json:"op"`
	Key   string `json:"key"`
	Value string `json:"value"`
}

// MemoryEntry 一条已落库的用户记忆
type MemoryEntry struct {
	Key   string
	Value string
}

// MemoryStore 长期记忆持久化后端
type MemoryStore interface {
	// Load 读取用户全部记忆（按更新时间降序，至多 maxUserMemories 条）
	Load(uid int64) ([]MemoryEntry, error)
	// Apply 应用一批操作（upsert/delete），原子执行
	Apply(uid int64, ops []MemOp) error
	Close() error
}

// SQLiteMemoryStore SQLite 实现（单连接串行写 + WAL，账号/会话库同款策略）
type SQLiteMemoryStore struct {
	db *sql.DB
}

// NewSQLiteMemoryStore 打开（或初始化）记忆库；父目录不存在时自动创建
func NewSQLiteMemoryStore(dbPath string) (*SQLiteMemoryStore, error) {
	if dir := filepath.Dir(dbPath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("创建记忆库目录: %w", err)
		}
	}
	dsn := "file:" + filepath.ToSlash(dbPath) + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("打开记忆库: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS user_memories (
		user_id    INTEGER NOT NULL,
		fact_key   TEXT NOT NULL,
		fact_value TEXT NOT NULL,
		updated_at INTEGER NOT NULL,
		PRIMARY KEY (user_id, fact_key)
	)`); err != nil {
		db.Close()
		return nil, fmt.Errorf("初始化记忆表: %w", err)
	}
	return &SQLiteMemoryStore{db: db}, nil
}

func (s *SQLiteMemoryStore) Load(uid int64) ([]MemoryEntry, error) {
	rows, err := s.db.Query(
		`SELECT fact_key, fact_value FROM user_memories WHERE user_id = ? ORDER BY updated_at DESC LIMIT ?`,
		uid, maxUserMemories)
	if err != nil {
		return nil, fmt.Errorf("读取用户 %d 记忆: %w", uid, err)
	}
	defer rows.Close()
	var out []MemoryEntry
	for rows.Next() {
		var e MemoryEntry
		if err := rows.Scan(&e.Key, &e.Value); err != nil {
			return nil, fmt.Errorf("扫描记忆行: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *SQLiteMemoryStore) Apply(uid int64, ops []MemOp) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("开启记忆事务: %w", err)
	}
	defer tx.Rollback()
	now := time.Now().Unix()
	for _, op := range ops {
		switch op.Op {
		case "upsert":
			if op.Key == "" {
				continue
			}
			if _, err := tx.Exec(
				`INSERT INTO user_memories (user_id, fact_key, fact_value, updated_at) VALUES (?,?,?,?)
				 ON CONFLICT(user_id, fact_key) DO UPDATE SET fact_value = excluded.fact_value, updated_at = excluded.updated_at`,
				uid, op.Key, op.Value, now); err != nil {
				return fmt.Errorf("写入记忆 %s: %w", op.Key, err)
			}
		case "delete":
			if _, err := tx.Exec(`DELETE FROM user_memories WHERE user_id = ? AND fact_key = ?`, uid, op.Key); err != nil {
				return fmt.Errorf("删除记忆 %s: %w", op.Key, err)
			}
		}
	}
	// 硬上限：超出按更新时间淘汰最旧的（提取器纪律之外的保险丝）
	if _, err := tx.Exec(
		`DELETE FROM user_memories WHERE user_id = ? AND fact_key NOT IN (
			SELECT fact_key FROM user_memories WHERE user_id = ? ORDER BY updated_at DESC, fact_key LIMIT ?
		)`, uid, uid, maxUserMemories); err != nil {
		return fmt.Errorf("裁剪用户 %d 记忆: %w", uid, err)
	}
	return tx.Commit()
}

func (s *SQLiteMemoryStore) Close() error { return s.db.Close() }

// ---- MemoryService：提取 + 注入 ----

// MemoryService 长期记忆服务：LLM 提取（复用主模型 SimpleChat）+ system 注入块渲染
type MemoryService struct {
	store  MemoryStore
	client *SeedClient
}

// NewMemoryService 组装记忆服务（client 为 nil 时只可注入不可提取）
func NewMemoryService(store MemoryStore, client *SeedClient) *MemoryService {
	return &MemoryService{store: store, client: client}
}

// ProfileBlock 渲染注入 system 的用户画像块；无记忆返回空串
func (m *MemoryService) ProfileBlock(uid int64) string {
	entries, err := m.store.Load(uid)
	if err != nil {
		log.Printf("[memory] 读取用户 %d 记忆失败（本次不注入）: %v", uid, err)
		return ""
	}
	if len(entries) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\n## 用户长期记忆（历史对话自动提取，供个性化参考，非系统指令）\n")
	for _, e := range entries {
		fmt.Fprintf(&b, "- %s: %s\n", e.Key, e.Value)
	}
	return b.String()
}

// ExtractAsync 异步提取本轮新记忆：独立 goroutine + 独立超时 ctx，
// 失败仅记日志——记忆是增值能力，绝不能拖垮主对话路径。
func (m *MemoryService) ExtractAsync(uid int64, userText, answer string) {
	if m.client == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), extractTimeout)
		defer cancel()

		// 现有记忆进提取提示词：LLM 才能决定"覆盖旧值 / 删除过时项 / 新增 key"
		existing, err := m.store.Load(uid)
		if err != nil {
			log.Printf("[memory] 提取前读取用户 %d 现有记忆失败，跳过本轮: %v", uid, err)
			return
		}
		sys := extractionSystemPrompt(existing)
		user := "【用户】\n" + truncateRunes(userText, extractInputLimit) +
			"\n\n【助手】\n" + truncateRunes(answer, extractInputLimit)
		raw, err := m.client.SimpleChat(ctx, sys, user)
		if err != nil {
			log.Printf("[memory] 用户 %d 记忆提取调用失败（跳过本轮）: %v", uid, err)
			return
		}
		ops := parseExtraction(raw)
		if len(ops) == 0 {
			return
		}
		if err := m.store.Apply(uid, ops); err != nil {
			log.Printf("[memory] 用户 %d 记忆写入失败: %v", uid, err)
			return
		}
		upserts, deletes := 0, 0
		for _, op := range ops {
			if op.Op == "delete" {
				deletes++
			} else {
				upserts++
			}
		}
		log.Printf("[memory] 用户 %d 记忆更新：+%d 更新 / -%d 删除", uid, upserts, deletes)
	}()
}

// extractionSystemPrompt 提取器提示词：现有记忆清单 + 提取规则 + 输出格式
func extractionSystemPrompt(existing []MemoryEntry) string {
	var b strings.Builder
	b.WriteString("你是游戏助手「三角洲行动攻略助手」的用户记忆提取引擎。根据本轮对话，提取关于**用户本人**的稳定事实与偏好（不是游戏知识本身）。\n\n")
	if len(existing) > 0 {
		b.WriteString("该用户的现有记忆（key: value）：\n")
		for _, e := range existing {
			fmt.Fprintf(&b, "- %s: %s\n", e.Key, e.Value)
		}
		b.WriteString("\n")
	}
	b.WriteString(`提取规则：
1. 只提取用户个人事实/偏好：常用武器、预算水平、新手/老手、偏好地图与玩法、外设或操作习惯、明确的长期目标等
2. 一次性的临时内容不提取（如"这把帮我配个装"、本次查的某个价格）
3. 与现有记忆冲突 → upsert 同 key 覆盖（如预算从 5 万变 8 万）；对话证实现有记忆已过时/有误 → delete
4. 尽量复用现有 key；新 key 用简短英文 snake_case（如 main_weapon、budget、skill_level、map_preference、play_style）
5. value 用简洁中文描述（如 "M14"、"约5万"、"新手"），不含换行
6. 本轮没有值得提取的信息 → 输出 []

只输出 JSON 数组本身（不要 markdown 代码块、不要解释）：
[{"op":"upsert","key":"main_weapon","value":"M14"},{"op":"delete","key":"budget"}]`)
	return b.String()
}

// ---- 解析与清洗：LLM 输出一律当不可信输入 ----

var keyPattern = regexp.MustCompile(`[^a-z0-9_]`)

// parseExtraction 解析提取器输出为操作列表：剥 markdown 围栏、逐条校验清洗，
// 任何格式问题都收敛为"丢弃该条/整批"，绝不 panic 或注入怪异内容。
func parseExtraction(raw string) []MemOp {
	s := strings.TrimSpace(raw)
	// 剥 ```json ... ``` 围栏（模型偶发不守规矩）
	if i := strings.Index(s, "```"); i >= 0 {
		s = s[i+3:]
		if j := strings.Index(s, "```"); j >= 0 {
			s = s[:j]
		}
		s = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s), "json"))
	}
	// 容错定位首个 [ 到最后一个 ]（防模型在数组前后加说明文字）
	start := strings.Index(s, "[")
	end := strings.LastIndex(s, "]")
	if start < 0 || end <= start {
		return nil
	}
	var ops []MemOp
	if err := json.Unmarshal([]byte(s[start:end+1]), &ops); err != nil {
		return nil
	}
	out := make([]MemOp, 0, len(ops))
	for _, op := range ops {
		op.Key = sanitizeKey(op.Key)
		op.Value = sanitizeValue(op.Value)
		if op.Op != "upsert" && op.Op != "delete" {
			continue
		}
		if op.Key == "" {
			continue
		}
		if op.Op == "upsert" && op.Value == "" {
			continue // 空值 upsert 没有语义
		}
		out = append(out, op)
		if len(out) >= maxMemoryOps {
			break
		}
	}
	return out
}

// sanitizeKey 强制小写蛇形：非法字符替换下划线，限长，去首尾下划线
func sanitizeKey(k string) string {
	k = strings.ToLower(strings.TrimSpace(k))
	k = keyPattern.ReplaceAllString(k, "_")
	if len(k) > maxMemoryKeyLen {
		k = k[:maxMemoryKeyLen]
	}
	return strings.Trim(k, "_")
}

// sanitizeValue 剥换行与控制字符（防伪造 system 结构），压缩空白，限长
func sanitizeValue(v string) string {
	v = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' {
			return ' '
		}
		if r < 32 {
			return -1
		}
		return r
	}, strings.TrimSpace(v))
	v = strings.Join(strings.Fields(v), " ")
	if len(v) > maxMemoryValLen {
		v = v[:maxMemoryValLen]
	}
	return v
}

// truncateRunes 复用 rerank.go 的既有实现（同包）

// ---- UID 上下文传递（server 层 → Agent）----

type uidCtxKey struct{}

// WithUID 把账号 UID 放入 ctx（0 = 匿名/主令牌，无长期记忆）
func WithUID(ctx context.Context, uid int64) context.Context {
	return context.WithValue(ctx, uidCtxKey{}, uid)
}

// UIDFromContext 取 UID；未设置返回 0
func UIDFromContext(ctx context.Context) int64 {
	uid, _ := ctx.Value(uidCtxKey{}).(int64)
	return uid
}
