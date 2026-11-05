package ai

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/volcengine/ark-runtime-go/arkruntime/model/chat"
)

// buildAskUserTurn 向会话追加一轮"模型调 ask_user"的完整消息序列
// （assistant tool_calls + tool 结果，与 Agent 主循环落盘格式一致）
func buildAskUserTurn(s *Session) {
	calls := []chat.ChatCompletionMessageToolCall{{
		ID: "call_ask_1",
		Function: chat.ChatCompletionMessageToolCallFunction{
			Name:      askUserToolName,
			Arguments: `{"question":"你更看重哪个方向？","options":[{"label":"压枪优先"},{"label":"机动优先"}]}`,
		},
	}}
	s.append(NewAssistantToolCallMessage(calls))
	s.append(NewToolResultMessage("call_ask_1", askUserToolName, `{"status":"shown"}`))
}

// TestAskUserToolRegistered NewAgent 默认注册 ask_user（无需显式装配）
func TestAskUserToolRegistered(t *testing.T) {
	a := NewAgent(nil)
	if _, ok := a.tools.Get(askUserToolName); !ok {
		t.Fatalf("ask_user 工具应默认注册")
	}
}

// TestAskUserSnapshotPairing 长对话（超 20 条窗口）中含 ask_user 调用对：
// snapshot 窗口滚动后 tool_calls 与 tool 结果必须成对出现，
// 且窗口起点不能落在 tool 消息上（拆开发给模型直接 API 400）
func TestAskUserSnapshotPairing(t *testing.T) {
	s := newSession("sess_snapshot", "system")
	// 填充到超过 maxKeep+1：多轮问答 + 中途一轮 ask_user + 玩家选择
	for i := 0; i < 8; i++ {
		s.append(NewUserMessage("第" + string(rune('A'+i)) + "个问题，查一下价格"))
		s.append(NewAssistantTextMessage("第" + string(rune('A'+i)) + "个问题的回答"))
	}
	buildAskUserTurn(s)
	s.append(NewUserMessage("压枪优先")) // 玩家点选的选项作为下一轮消息
	s.append(NewAssistantTextMessage("好的，按压枪优先给你方案……"))
	for i := 0; i < 4; i++ {
		s.append(NewUserMessage("追问" + string(rune('0'+i))))
		s.append(NewAssistantTextMessage("追问回答" + string(rune('0'+i))))
	}

	snap := s.snapshot()
	if len(snap) >= len(s.messages) {
		t.Fatalf("会话已超窗口，snapshot 应发生截断（%d vs %d）", len(snap), len(s.messages))
	}
	// 窗口第一条是 system，其后不能以 tool 消息开头
	if len(snap) > 1 {
		if r, _, _ := messageText(snap[1]); r == "tool" {
			t.Fatalf("窗口起点落在 tool 消息上：配对撕裂会 API 400")
		}
	}
	// 全窗口内 assistant 的 tool_calls 与 tool 结果必须逐个配对
	pending := map[string]bool{}
	for _, m := range snap {
		role, _, _ := messageText(m)
		if role == "assistant" {
			sum := m.OneOf
			if sum.IsChatCompletionRequestAssistantMessage() {
				for _, c := range sum.ChatCompletionRequestAssistantMessage.ToolCalls {
					pending[c.ID] = true
				}
			}
		} else if role == "tool" {
			sum := m.OneOf
			id := sum.ChatCompletionRequestToolMessage.ToolCallID
			if !pending[id] {
				t.Fatalf("出现无发起者的 tool 消息（id=%s）", id)
			}
			delete(pending, id)
		}
	}
	if len(pending) > 0 {
		t.Fatalf("tool_calls 未配对 tool 结果：%v", pending)
	}
}

// TestAskUserCompactCutWithClarifyTurn 压缩切点不落在 ask_user 的
// tool 消息上（切点必须推进到 user 消息边界）
func TestAskUserCompactCutWithClarifyTurn(t *testing.T) {
	s := newSession("sess_compact", "system")
	for i := 0; i < 10; i++ {
		s.append(NewUserMessage(strings.Repeat("长问题内容", 60))) // 每条 ~240 token
		s.append(NewAssistantTextMessage(strings.Repeat("长回答内容", 60)))
	}
	buildAskUserTurn(s) // ask_user 消息对落在中段
	s.append(NewUserMessage("玩家选择"))
	s.append(NewAssistantTextMessage("按选择作答"))

	msgs := s.historyAfterSystem()
	k := compactCut(msgs, 2000)
	if k < 0 {
		t.Fatalf("预算 2000 下应产生切点")
	}
	if k < len(msgs) && !isUserMessage(msgs[k]) {
		t.Fatalf("切点 %d 落在非 user 消息上（tool 配对撕裂风险）", k)
	}
}

// TestAskUserHandlerValidation 工具参数校验：<2 个有效选项返回 invalid（模型自纠）
func TestAskUserHandlerValidation(t *testing.T) {
	a := NewAgent(nil)
	tool, ok := a.tools.Get(askUserToolName)
	if !ok {
		t.Fatalf("ask_user 工具应默认注册")
	}
	// 经 Execute 路径调用（ask_user 无 IO 副作用，仅 emitProgress）
	got := []string{}
	ctx := WithProgressFunc(context.Background(), func(ev ProgressEvent) {
		if ev.Kind == ProgressClarify {
			got = append(got, ev.Text)
		}
	})
	// 有效调用：两个选项
	res, err := tool.Execute(ctx, json.RawMessage(
		`{"question":"选哪个方向","options":[{"label":"A方案"},{"label":"B方案"}]}`))
	if err != nil {
		t.Fatalf("执行失败: %v", err)
	}
	if m, ok := res.(map[string]any); !ok || m["status"] != "shown" {
		t.Fatalf("有效调用应返回 shown: %v", res)
	}
	if len(got) != 1 {
		t.Fatalf("应推送 1 次 clarify 事件，实际 %d", len(got))
	}
	// 无效调用：只有 1 个选项
	res2, err := tool.Execute(ctx, json.RawMessage(
		`{"question":"选哪个","options":[{"label":"A方案"}]}`))
	if err != nil {
		t.Fatalf("执行失败: %v", err)
	}
	if m, ok := res2.(map[string]any); !ok || m["status"] != "invalid" {
		t.Fatalf("单选项应返回 invalid 让模型自纠: %v", res2)
	}
	if len(got) != 1 {
		t.Fatalf("invalid 调用不应推送 clarify，实际推送 %d 次", len(got)-1)
	}
}
