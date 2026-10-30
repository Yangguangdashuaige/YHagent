// internal/ai/session_store.go
//
// 会话持久化：把内存会话历史落 SQLite，服务重启 / 空闲淘汰后按 sessionID 无感恢复。
// 分层记忆第二步（第一步账号系统提供 UID 归属，第三步长期记忆在其上提取用户画像）。
//
// 设计要点：
//   - 序列化直接用 SDK 的 Marshal/UnmarshalJSON（OpenAI 风格 JSON），不手工映射五类消息；
//   - 持久化时把历史消息里的图片 data URL 替换为占位文本——单张图 base64 可达 5MB+，
//     5000 会话全量落盘会撑爆磁盘；模型已消费过历史图片，恢复后的上下文带占位即可用；
//   - 恢复时丢弃落库的 system 消息、重写为当前 systemPrompt：提示词跨版本升级即时生效；
//   - 写入时机 = 整轮结束（turn 锁内 defer），轮内多次 append/压缩由最后一次快照覆盖；
//   - 存储失败只记日志不阻断对话：可用性优先于持久性，最多丢一轮；
//   - 行保留 7 天（按 updated_at），空闲会话自动过期，防无限增长。
package ai

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/volcengine/ark-runtime-go/arkruntime/model/chat"
	_ "modernc.org/sqlite"
)

// chatMessage SDK 消息类型的短别名（本文件及 Session 持久化相关代码用）
type chatMessage = chat.ChatCompletionRequestMessage

// sessionRetention 落库会话的保留时长：超过 7 天未更新即清理
const sessionRetention = 7 * 24 * time.Hour

// SessionStore 会话历史的持久化后端（测试可替换为内存实现）
type SessionStore interface {
	// Load 读取历史消息；ok=false 表示无记录（新会话），err 非 nil 时调用方按新会话兜底
	Load(sessionID string) (messages []json.RawMessage, ok bool, err error)
	// Save 全量覆盖该会话的历史（messages 为逐条序列化后的 JSON）
	Save(sessionID string, messages []json.RawMessage) error
	// Delete 删除该会话的持久化记录（重置语义）
	Delete(sessionID string) error
	Close() error
}

// SQLiteSessionStore SQLite 实现单文件落盘，串行写（账号系统同款策略：单连接规避 BUSY）
type SQLiteSessionStore struct {
	db *sql.DB
}

// NewSQLiteSessionStore 打开（或初始化）会话库；父目录不存在时自动创建
func NewSQLiteSessionStore(dbPath string) (*SQLiteSessionStore, error) {
	if dir := filepath.Dir(dbPath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("创建会话库目录: %w", err)
		}
	}
	dsn := "file:" + filepath.ToSlash(dbPath) + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("打开会话库: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS chat_sessions (
		session_id TEXT PRIMARY KEY,
		messages   BLOB NOT NULL,
		updated_at INTEGER NOT NULL
	)`); err != nil {
		db.Close()
		return nil, fmt.Errorf("初始化会话表: %w", err)
	}
	s := &SQLiteSessionStore{db: db}
	go s.cleanupLoop()
	return s, nil
}

// rows 会话表行结构（messages 为 JSON 数组整体落一个 BLOB）
type rows struct {
	Messages  []byte
	UpdatedAt int64
}

func (s *SQLiteSessionStore) Load(sessionID string) ([]json.RawMessage, bool, error) {
	var r rows
	err := s.db.QueryRow(`SELECT messages, updated_at FROM chat_sessions WHERE session_id = ?`, sessionID).
		Scan(&r.Messages, &r.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("读取会话 %s: %w", sessionID, err)
	}
	var msgs []json.RawMessage
	if err := json.Unmarshal(r.Messages, &msgs); err != nil {
		// 单行损坏按无记录处理，不让一条脏数据杀死整个会话层
		log.Printf("[session-store] 会话 %s 落库数据损坏，按新会话处理: %v", sessionID, err)
		return nil, false, nil
	}
	return msgs, true, nil
}

func (s *SQLiteSessionStore) Save(sessionID string, messages []json.RawMessage) error {
	blob, err := json.Marshal(messages)
	if err != nil {
		return fmt.Errorf("序列化会话 %s: %w", sessionID, err)
	}
	_, err = s.db.Exec(`INSERT INTO chat_sessions (session_id, messages, updated_at) VALUES (?,?,?)
		ON CONFLICT(session_id) DO UPDATE SET messages = excluded.messages, updated_at = excluded.updated_at`,
		sessionID, blob, time.Now().Unix())
	if err != nil {
		return fmt.Errorf("落盘会话 %s: %w", sessionID, err)
	}
	return nil
}

func (s *SQLiteSessionStore) Delete(sessionID string) error {
	if _, err := s.db.Exec(`DELETE FROM chat_sessions WHERE session_id = ?`, sessionID); err != nil {
		return fmt.Errorf("删除会话 %s: %w", sessionID, err)
	}
	return nil
}

func (s *SQLiteSessionStore) Close() error {
	return s.db.Close()
}

// cleanupLoop 每小时清理超过保留期的会话行（内存侧另有 2 小时空闲清理，两者独立）
func (s *SQLiteSessionStore) cleanupLoop() {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for range t.C {
		if _, err := s.db.Exec(`DELETE FROM chat_sessions WHERE updated_at < ?`,
			time.Now().Add(-sessionRetention).Unix()); err != nil {
			continue // 下个周期再试
		}
	}
}

// ---- 序列化与瘦身 ----

// persistableMessages 把 SDK 消息序列化为逐条 JSON，同时把历史图片替换为占位文本。
// 图片处理走 JSON 层手术：不触碰 SDK 类型，user 消息的 parts 数组里
// type=="image_url" 的分片替换为 {"type":"text","text":"（图片内容未保存）"}。
func persistableMessages(msgs []chatMessage) []json.RawMessage {
	out := make([]json.RawMessage, 0, len(msgs))
	for _, m := range msgs {
		b, err := m.MarshalJSON()
		if err != nil {
			continue // 单条序列化失败跳过该条，不让一条脏消息废掉整轮快照
		}
		out = append(out, stripImageParts(b))
	}
	return out
}

// stripImageParts 在 JSON 层把消息内的图片分片替换为占位文本（非 user / 非数组内容原样返回）
func stripImageParts(raw json.RawMessage) json.RawMessage {
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return raw
	}
	parts, ok := obj["content"].([]any)
	if !ok || len(parts) == 0 {
		return raw
	}
	changed := false
	for i, p := range parts {
		pm, ok := p.(map[string]any)
		if !ok || pm["type"] != "image_url" {
			continue
		}
		parts[i] = map[string]any{"type": "text", "text": "（图片内容未保存）"}
		changed = true
	}
	if !changed {
		return raw
	}
	obj["content"] = parts
	if b, err := json.Marshal(obj); err == nil {
		return b
	}
	return raw
}

// restoreMessages 把落库 JSON 还原为 SDK 消息；跳过 system 消息（恢复时重写当前提示词）
func restoreMessages(raws []json.RawMessage) []chatMessage {
	out := make([]chatMessage, 0, len(raws))
	for _, r := range raws {
		var m chatMessage
		if err := m.UnmarshalJSON(r); err != nil {
			continue // 单条损坏跳过，保住其余历史
		}
		if m.OneOf.IsChatCompletionRequestSystemMessage() {
			continue
		}
		out = append(out, m)
	}
	return out
}
