package ai

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/volcengine/ark-runtime-go/arkruntime/model/chat"
)

// newStoreTestManager 带真实 SQLite 存储的管理器（临时目录）
func newStoreTestManager(t *testing.T) (*Manager, *SQLiteSessionStore) {
	t.Helper()
	st, err := NewSQLiteSessionStore(filepath.Join(t.TempDir(), "sessions.db"))
	if err != nil {
		t.Fatalf("打开会话库: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return NewSessionManager().WithStore(st), st
}

// TestPersistableMessagesRoundTrip 五类消息序列化往返 + 图片占位替换
func TestPersistableMessagesRoundTrip(t *testing.T) {
	toolCall := chat.ChatCompletionMessageToolCall{
		ID:   "call_1",
		Type: chat.ToolTypeFunction,
		Function: chat.ChatCompletionMessageToolCallFunction{
			Name:      "delta_item_list",
			Arguments: `{"types":"gun"}`,
		},
	}
	msgs := []chatMessage{
		newSystemMessage("系统提示"),
		NewUserMessage("M14 怎么改"),
		NewAssistantToolCallMessage([]chat.ChatCompletionMessageToolCall{toolCall}),
		NewToolResultMessage("call_1", "delta_item_list", `[{"name":"M14"}]`),
		NewAssistantTextMessage("M14 改枪建议……"),
	}

	raws := persistableMessages(msgs)
	if len(raws) != len(msgs) {
		t.Fatalf("应逐条序列化 %d 条，得到 %d", len(msgs), len(raws))
	}
	// 图片占位：多模态消息里的 image_url 分片应替换为文本
	imgMsg := NewUserMessageWithImages("看这张图", []UserImage{{DataURL: "data:image/png;base64,AAAA"}})
	imgRaw := persistableMessages([]chatMessage{imgMsg})[0]
	if strings.Contains(string(imgRaw), "base64") {
		t.Errorf("图片 data URL 不应落盘: %s", imgRaw)
	}
	if !strings.Contains(string(imgRaw), "图片内容未保存") {
		t.Errorf("应有图片占位文本: %s", imgRaw)
	}

	// 五类消息完整往返：restore 后角色与关键字段不丢
	all := append(msgs, imgMsg)
	restored := restoreMessages(persistableMessages(all))
	if len(restored) != len(all)-1 { // system 一条被跳过
		t.Fatalf("恢复条数不符：期望 %d 得到 %d", len(all)-1, len(restored))
	}
	ru := restored[0].OneOf
	if !ru.IsChatCompletionRequestUserMessage() {
		t.Errorf("第 1 条应为 user")
	}
	ra := restored[1].OneOf
	if !ra.IsChatCompletionRequestAssistantMessage() {
		t.Fatalf("第 2 条应为 assistant(tool_calls)")
	}
	am, _ := ra.GetChatCompletionRequestAssistantMessage()
	if len(am.ToolCalls) != 1 || am.ToolCalls[0].ID != "call_1" {
		t.Errorf("tool_calls 往返丢失: %+v", am.ToolCalls)
	}
	rt := restored[2].OneOf
	if !rt.IsChatCompletionRequestToolMessage() {
		t.Fatalf("第 3 条应为 tool")
	}
	tm, _ := rt.GetChatCompletionRequestToolMessage()
	if tm.ToolCallID != "call_1" || !strings.Contains(tm.Content.String, "M14") {
		t.Errorf("tool 结果往返丢失: id=%s", tm.ToolCallID)
	}
}

// TestStoreSaveLoadDelete 存储层 CRUD
func TestStoreSaveLoadDelete(t *testing.T) {
	_, st := newStoreTestManager(t)

	if _, ok, _ := st.Load("sess_none"); ok {
		t.Fatal("不存在的会话应 ok=false")
	}
	msgs := []json.RawMessage{[]byte(`{"role":"user","content":"hi"}`)}
	if err := st.Save("sess_a", msgs); err != nil {
		t.Fatalf("保存失败: %v", err)
	}
	got, ok, err := st.Load("sess_a")
	if err != nil || !ok || len(got) != 1 || !strings.Contains(string(got[0]), "hi") {
		t.Fatalf("读取不符: ok=%v err=%v got=%s", ok, err, got)
	}
	// 覆盖保存
	if err := st.Save("sess_a", []json.RawMessage{[]byte(`{"role":"user","content":"第二版"}`)}); err != nil {
		t.Fatal(err)
	}
	got, _, _ = st.Load("sess_a")
	if !strings.Contains(string(got[0]), "第二版") {
		t.Errorf("覆盖保存未生效: %s", got[0])
	}
	if err := st.Delete("sess_a"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := st.Load("sess_a"); ok {
		t.Errorf("删除后不应再读到")
	}
	// 幂等删除
	if err := st.Delete("sess_a"); err != nil {
		t.Errorf("重复删除应成功: %v", err)
	}
}

// TestManagerRestoreAfterRestart 核心场景：重启（新 Manager 同一库）后按 sessionID 恢复历史
func TestManagerRestoreAfterRestart(t *testing.T) {
	m1, st := newStoreTestManager(t)

	s1 := m1.GetOrCreate("sess_restart", "旧提示词")
	s1.append(NewUserMessage("M14 据枪+8 怎么配"))
	s1.append(NewAssistantTextMessage("轻语短枪管 + 实用消焰器……"))
	s1.persist()

	// 模拟重启：全新 Manager 挂同一存储
	m2 := NewSessionManager().WithStore(st)
	s2 := m2.GetOrCreate("sess_restart", "新提示词")
	if len(s2.messages) != 3 { // system + 2 条历史
		t.Fatalf("恢复后应有 3 条消息，得到 %d", len(s2.messages))
	}
	// system 重写为当前提示词（跨版本升级生效）
	sys, _ := s2.messages[0].OneOf.GetChatCompletionRequestSystemMessage()
	if sys.Content.String != "新提示词" {
		t.Errorf("恢复时应重写当前提示词，得到 %q", sys.Content.String)
	}
	// 历史内容完整
	u, _ := s2.messages[1].OneOf.GetChatCompletionRequestUserMessage()
	if !strings.Contains(u.Content.String, "据枪") {
		t.Errorf("历史用户消息丢失: %s", u.Content.String)
	}

	// 恢复的会话可继续落盘（store 已注入）
	s2.append(NewUserMessage("继续问"))
	s2.persist()
	m3 := NewSessionManager().WithStore(st)
	s3 := m3.GetOrCreate("sess_restart", "新提示词")
	if len(s3.messages) != 4 {
		t.Errorf("二次恢复应有 4 条消息，得到 %d", len(s3.messages))
	}
}

// TestResetClearsPersisted 重置语义：内存与磁盘一起清
func TestResetClearsPersisted(t *testing.T) {
	m, _ := newStoreTestManager(t)
	s := m.GetOrCreate("sess_reset", "p")
	s.append(NewUserMessage("历史"))
	s.persist()

	// Agent.Reset 同款路径：turn 锁内 reset + clearPersisted
	if !s.tryBeginTurn() {
		t.Fatal("会话应空闲")
	}
	s.reset("p")
	s.clearPersisted()
	s.endTurn()

	m2 := NewSessionManager().WithStore(m.store)
	s2 := m2.GetOrCreate("sess_reset", "p")
	if len(s2.messages) != 1 {
		t.Errorf("重置后恢复应只剩 system，得到 %d 条", len(s2.messages))
	}
}

// TestEvictKeepsPersistedRow 内存淘汰/空闲清理不清磁盘：只删 map 条目后仍可恢复
func TestEvictKeepsPersistedRow(t *testing.T) {
	m, _ := newStoreTestManager(t)
	s := m.GetOrCreate("sess_evict", "p")
	s.append(NewUserMessage("淘汰前的历史"))
	s.persist()

	// 模拟 cleanupLoop / evictOneIdleLocked 的内存路径：只删 map，不动 store
	m.mu.Lock()
	delete(m.sessions, "sess_evict")
	m.mu.Unlock()

	m2 := NewSessionManager().WithStore(m.store)
	s2 := m2.GetOrCreate("sess_evict", "p")
	if len(s2.messages) != 2 {
		t.Errorf("内存淘汰后应能从磁盘恢复 2 条，得到 %d", len(s2.messages))
	}
}
