package ai

import (
	"strings"
	"testing"

	"github.com/volcengine/ark-runtime-go/arkruntime/model/chat"
)

func TestEstString(t *testing.T) {
	// 纯中文 ≈ 1 token/字
	if got := estString("三角洲行动"); got != 5 {
		t.Errorf("estString(中文5字) = %d, want 5", got)
	}
	// 纯 ASCII ≈ 1/4 token/字符
	if got := estString("abcdefgh"); got != 2 {
		t.Errorf("estString(ascii8) = %d, want 2", got)
	}
	if got := estString(""); got != 0 {
		t.Errorf("estString(空) = %d, want 0", got)
	}
}

func TestEstimateMessageTokens(t *testing.T) {
	u := NewUserMessage("三角洲行动") // 5 CJK + 8 固定开销
	if got := estimateMessageTokens(u); got != 13 {
		t.Errorf("user 文本消息 = %d, want 13", got)
	}
	// 工具结果消息：内容 + 固定开销
	tr := NewToolResultMessage("call_1", "search_knowledge", `{"资料":[]}`)
	if got := estimateMessageTokens(tr); got < 10 {
		t.Errorf("tool 结果消息 token 过小: %d", got)
	}
}

func TestCompactCutBoundary(t *testing.T) {
	// 构造：用户 / (assistant tool_calls + tool 结果) 配对交替的典型历史
	msgs := []chat.ChatCompletionRequestMessage{
		NewUserMessage(strings.Repeat("问", 200)), // 208
		NewAssistantToolCallMessage([]chat.ChatCompletionMessageToolCall{{
			ID:   "c1",
			Type: chat.ToolTypeFunction,
			Function: chat.ChatCompletionMessageToolCallFunction{
				Name:      "search_knowledge",
				Arguments: `{"query":"测试"}`,
			},
		}}),
		NewToolResultMessage("c1", "search_knowledge", strings.Repeat("果", 400)), // 408
		NewUserMessage(strings.Repeat("问", 200)),
		NewAssistantTextMessage(strings.Repeat("答", 300)), // 308
		NewUserMessage(strings.Repeat("问", 50)),           // 最新一条用户消息
	}
	total := 0
	for _, m := range msgs {
		total += estimateMessageTokens(m)
	}

	// 预算充足 → 不压缩
	if k := compactCut(msgs, total*2); k != -1 {
		t.Errorf("预算充足时应返回 -1, got %d", k)
	}

	// 预算吃紧（保留配额只能覆盖尾部几条）：切点必须落在用户消息上，
	// 且保留段不得以 tool 消息开头（拆散配对会导致 API 400）
	k := compactCut(msgs, total/2)
	if k < 0 {
		t.Fatalf("超预算应触发压缩, got -1 (total=%d)", total)
	}
	if !isUserMessage(msgs[k]) {
		t.Errorf("切点 %d 必须是用户消息, 实际角色=%v", k, msgs[k].OneOf.Type)
	}
	// 保留段至少含最后一条用户消息
	if k >= len(msgs) {
		t.Errorf("切点 %d 超出消息范围", k)
	}
}

func TestCompactCutPrefixTooSmall(t *testing.T) {
	// 前缀不足 minPrefixTokens：即使超预算也不值得压缩
	msgs := []chat.ChatCompletionRequestMessage{
		NewUserMessage("问"),
		NewUserMessage(strings.Repeat("答", 3000)), // 单条大消息撑爆预算
	}
	if k := compactCut(msgs, 500); k != -1 {
		t.Errorf("前缀过小应返回 -1, got %d", k)
	}
}

func TestSessionReplacePrefix(t *testing.T) {
	s := newSession("s-test", "你是系统")
	s.append(NewUserMessage("第一问"))
	s.append(NewAssistantTextMessage("第一答"))
	s.append(NewUserMessage("第二问"))

	msgs := s.historyAfterSystem()
	if len(msgs) != 3 {
		t.Fatalf("historyAfterSystem 长度 = %d, want 3", len(msgs))
	}

	s.replacePrefix(2, NewUserMessage(summaryMarker+"\n玩家问了两个问题"))
	after := s.historyAfterSystem()
	if len(after) != 2 {
		t.Fatalf("replacePrefix 后长度 = %d, want 2 (摘要+第二问)", len(after))
	}
	if !isSummaryMessage(after[0]) {
		t.Errorf("第一条应为摘要消息")
	}
	if !isUserMessage(after[1]) || !strings.Contains(msgTextOf(after[1]), "第二问") {
		t.Errorf("第二条应保留原文用户消息")
	}
	// system 消息仍在首位
	if _, text, _ := messageText(s.messages[0]); text != "你是系统" {
		t.Errorf("system 消息被破坏: %q", text)
	}
	// 非法 k 被兜底忽略
	s.replacePrefix(99, NewUserMessage("坏"))
	if len(s.historyAfterSystem()) != 2 {
		t.Errorf("非法 k 应被忽略")
	}
}

func msgTextOf(m chat.ChatCompletionRequestMessage) string {
	_, text, _ := messageText(m)
	return text
}
