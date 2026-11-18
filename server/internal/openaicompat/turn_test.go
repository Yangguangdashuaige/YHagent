// internal/openaicompat/turn_test.go
//
// 门面循环 + 影子回合状态机单测：fake LLM 脚本化模型行为，验证三种分叉
// （纯文本 / 纯服务端工具 / 含客户端工具中继）与续轮唤醒的 id 映射。
package openaicompat

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/volcengine/ark-runtime-go/arkruntime/model/chat"

	"yhagent-server/internal/ai"
)

// fakeTurn 一次模型调用应产出的内容（文本 + 工具调用）。
type fakeTurn struct {
	text      string
	toolCalls []chat.ChatCompletionMessageToolCall
}

// fakeLLM 脚本化模型：每次 ChatStream 消费 script 一项，并捕获
// 最后一次收到的消息与工具集（供测试断言门面组装的上下文）。
type fakeLLM struct {
	script      []fakeTurn
	callCount   int
	lastMsgs    []chat.ChatCompletionRequestMessage
	lastTools   []chat.ChatCompletionTool
	lastToolset map[string]bool
}

func (f *fakeLLM) ChatStream(
	_ context.Context,
	history []chat.ChatCompletionRequestMessage,
	tools []chat.ChatCompletionTool,
	onChunk func(string),
	onToolCall func([]chat.ChatCompletionMessageToolCall) error,
	_ func(chat.CompletionUsage),
	_ func(string),
) error {
	f.lastMsgs = history
	f.lastTools = tools
	f.lastToolset = make(map[string]bool, len(tools))
	for _, t := range tools {
		f.lastToolset[t.Function.Name] = true
	}
	i := f.callCount
	if i >= len(f.script) {
		i = len(f.script) - 1
	}
	f.callCount++
	turn := f.script[i]
	if turn.text != "" && onChunk != nil {
		onChunk(turn.text)
	}
	if len(turn.toolCalls) > 0 && onToolCall != nil {
		_ = onToolCall(turn.toolCalls)
	}
	return nil
}

// petSayTool 构造一个典型客户端工具 schema（pet_say）。
func petSayTool() WireTool {
	return WireTool{
		Type: "function",
		Function: &WireToolFnDef{
			Name:        "pet_say",
			Description: "冒气泡说话",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"script":{"type":"string"}},"required":["script"]}`),
		},
	}
}

// newTestFacade 构造测试门面：注册 delta_test 服务端工具 + ask_user（验证剔除）。
func newTestFacade(script []fakeTurn) (*Facade, *fakeLLM) {
	llm := &fakeLLM{script: script}
	reg := ai.NewRegistry()
	reg.Register(ai.NewFunctionTool("delta_test", "测试服务端工具",
		map[string]any{"q": map[string]any{"type": "string"}},
		[]string{"q"},
		func(ctx context.Context, rawArgs json.RawMessage) (any, error) {
			return map[string]string{"result": "delta-data"}, nil
		},
	))
	reg.Register(ai.NewFunctionTool("ask_user", "澄清（应被剔除）",
		map[string]any{"question": map[string]any{"type": "string"}},
		[]string{"question"},
		func(ctx context.Context, rawArgs json.RawMessage) (any, error) { return nil, nil },
	))
	return NewFacade(llm, reg, Config{PendingTTL: 0}), llm
}

// simpleReq 构造最小请求：system + 一条 user 消息。
func simpleReq(extra string) ChatRequest {
	raw := `[{"role":"system","content":"桌宠人格"},{"role":"user","content":"帮我查密码"}]`
	var msgs []WireMessage
	_ = json.Unmarshal([]byte(raw), &msgs)
	req := ChatRequest{Model: "m", Messages: msgs}
	if extra != "" {
		_ = json.Unmarshal([]byte(extra), &req.Tools)
	}
	return req
}

func TestRunTurnPlainAnswer(t *testing.T) {
	f, llm := newTestFacade([]fakeTurn{{text: "零号大坝密码是 123456"}})
	res, err := f.RunTurn(context.Background(), simpleReq(""), TurnEvents{})
	if err != nil {
		t.Fatalf("回合失败: %v", err)
	}
	if res.FinishReason != FinishStop || res.Text != "零号大坝密码是 123456" {
		t.Fatalf("终态: %+v", res)
	}
	// 系统提示词融合：业务基底在前、桌宠人格在后
	first := llm.lastMsgs[0]
	b, _ := first.MarshalJSON()
	var obj struct {
		Content string `json:"content"`
	}
	_ = json.Unmarshal(b, &obj)
	if !strings.Contains(obj.Content, "三角洲行动") || !strings.Contains(obj.Content, "桌宠人格") {
		t.Fatalf("融合提示词缺少组成段: %q", obj.Content)
	}
	// ask_user 必须被剔除
	if llm.lastToolset["ask_user"] {
		t.Fatal("ask_user 未被剔除")
	}
}

func TestRunTurnServerToolOnly(t *testing.T) {
	f, llm := newTestFacade([]fakeTurn{
		{toolCalls: []chat.ChatCompletionMessageToolCall{{
			ID:   "call_sv1",
			Type: chat.ToolTypeFunction,
			Function: chat.ChatCompletionMessageToolCallFunction{
				Name: "delta_test", Arguments: `{"q":"密码"}`,
			},
		}}},
		{text: "查询完成"},
	})
	res, err := f.RunTurn(context.Background(), simpleReq(""), TurnEvents{})
	if err != nil {
		t.Fatalf("回合失败: %v", err)
	}
	if res.FinishReason != FinishStop {
		t.Fatalf("终态: %+v", res)
	}
	// 第二轮发给模型的消息应含：assistant.tool_calls + tool 结果
	var sawCall, sawResult bool
	for _, m := range llm.lastMsgs {
		b, _ := m.MarshalJSON()
		s := string(b)
		if strings.Contains(s, "delta_test") && strings.Contains(s, "call_sv1") {
			sawCall = true
		}
		if strings.Contains(s, "delta-data") {
			sawResult = true
		}
	}
	if !sawCall || !sawResult {
		t.Fatalf("服务端工具痕迹缺失: call=%v result=%v", sawCall, sawResult)
	}
}

func TestRunTurnClientRelayAndResume(t *testing.T) {
	f, llm := newTestFacade([]fakeTurn{
		// 第一回合：混合调用（服务端 delta_test + 客户端 pet_say）
		{toolCalls: []chat.ChatCompletionMessageToolCall{
			{ID: "call_c1", Type: chat.ToolTypeFunction,
				Function: chat.ChatCompletionMessageToolCallFunction{Name: "pet_say", Arguments: `{"script":"hi"}`}},
			{ID: "call_s1", Type: chat.ToolTypeFunction,
				Function: chat.ChatCompletionMessageToolCallFunction{Name: "delta_test", Arguments: `{"q":"x"}`}},
		}},
		// 续轮（回执到达后）：最终回答
		{text: "done"},
	})
	req := simpleReq(`[{"type":"function","function":{"name":"pet_say","description":"d"}}]`)
	res, err := f.RunTurn(context.Background(), req, TurnEvents{})
	if err != nil {
		t.Fatalf("回合失败: %v", err)
	}
	if res.FinishReason != FinishToolCalls || len(res.ClientCalls) != 1 {
		t.Fatalf("中继终态: %+v", res)
	}
	wireID := res.ClientCalls[0].ID
	if !strings.HasPrefix(wireID, relayCallIDPrefix) {
		t.Fatalf("客户端工具 id 未重写: %s", wireID)
	}
	// 客户端工具 schema 必须下发给模型
	if !llm.lastToolset["pet_say"] || !llm.lastToolset["delta_test"] {
		t.Fatalf("工具集异常: %v", llm.lastToolset)
	}

	// 续轮：客户端带 pet_say 回执（wire id）再请求
	resumeRaw := `[
		{"role":"system","content":"桌宠人格"},
		{"role":"user","content":"帮我查密码"},
		{"role":"assistant","content":"","tool_calls":[
			{"id":"` + wireID + `","type":"function","function":{"name":"pet_say","arguments":"{\"script\":\"hi\"}"}}
		]},
		{"role":"tool","tool_call_id":"` + wireID + `","content":"{\"shown\":true}"}
	]`
	var resumeMsgs []WireMessage
	_ = json.Unmarshal([]byte(resumeRaw), &resumeMsgs)
	resumeReq := ChatRequest{Model: "m", Messages: resumeMsgs,
		Tools: []WireTool{petSayTool()}}

	res2, err := f.RunTurn(context.Background(), resumeReq, TurnEvents{})
	if err != nil {
		t.Fatalf("续轮失败: %v", err)
	}
	if res2.FinishReason != FinishStop || res2.Text != "done" {
		t.Fatalf("续轮终态: %+v", res2)
	}
	// 唤醒的影子上下文：末尾 tool 消息必须是原生 id（call_c1），
	// 且含服务端工具 delta_test 的结果（客户端看不到但模型必须看到）
	var sawOrigReceipt, sawServerResult bool
	for _, m := range llm.lastMsgs {
		b, _ := m.MarshalJSON()
		s := string(b)
		if strings.Contains(s, "call_c1") && strings.Contains(s, "shown") {
			sawOrigReceipt = true
		}
		if strings.Contains(s, "delta-data") {
			sawServerResult = true
		}
	}
	if !sawOrigReceipt {
		t.Fatal("客户端回执未映射回原生 id（配对断裂）")
	}
	if !sawServerResult {
		t.Fatal("影子上下文丢失服务端工具结果")
	}
}

func TestRunTurnResumePendingLost(t *testing.T) {
	// 不先跑中继回合（无 pending），直接发一条带 yhx_ 回执的历史 → 降级重建
	f, llm := newTestFacade([]fakeTurn{{text: "降级回答"}})
	raw := `[
		{"role":"user","content":"q"},
		{"role":"assistant","tool_calls":[
			{"id":"yhx_deadbeef_0","type":"function","function":{"name":"pet_say","arguments":"{}"}}
		]},
		{"role":"tool","tool_call_id":"yhx_deadbeef_0","content":"ok"}
	]`
	var msgs []WireMessage
	_ = json.Unmarshal([]byte(raw), &msgs)
	res, err := f.RunTurn(context.Background(), ChatRequest{Model: "m", Messages: msgs}, TurnEvents{})
	if err != nil {
		t.Fatalf("降级路径失败: %v", err)
	}
	if res.FinishReason != FinishStop || res.Text != "降级回答" {
		t.Fatalf("降级终态: %+v", res)
	}
	// 降级上下文 = 融合 system + 客户端历史原样（wire id 回执天然配对完整）
	if len(llm.lastMsgs) != 4 {
		t.Fatalf("降级上下文消息数 = %d", len(llm.lastMsgs))
	}
}

// relayReceiptTurnID 的 turnID 提取必须容忍含下划线的 trace ID（t_<hex>）：
// 按最后一个下划线剥离序号，不能把 "t" 当作回合标识（曾致影子回合唤不醒）。
func TestRelayReceiptTurnIDWithUnderscoredID(t *testing.T) {
	raw := `[
		{"role":"assistant","tool_calls":[
			{"id":"yhx_t_dafdc608255b4e5d_0","type":"function","function":{"name":"pet_say","arguments":"{}"}}
		]},
		{"role":"tool","tool_call_id":"yhx_t_dafdc608255b4e5d_0","content":"ok"}
	]`
	h, err := ParseHistory(mustParse(t, raw))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if got := relayReceiptTurnID(h.Messages); got != "t_dafdc608255b4e5d" {
		t.Fatalf("turnID 提取 = %q", got)
	}
}

func TestRunTurnMixedReceiptFallback(t *testing.T) {
	// 末尾回执混有非 yhx_ 前缀 → 不认续轮，整体按客户端历史重建
	f, _ := newTestFacade([]fakeTurn{{text: "ok"}})
	raw := `[
		{"role":"user","content":"q"},
		{"role":"tool","tool_call_id":"yhx_aa_0","content":"a"},
		{"role":"tool","tool_call_id":"call_other","content":"b"}
	]`
	var msgs []WireMessage
	_ = json.Unmarshal([]byte(raw), &msgs)
	res, err := f.RunTurn(context.Background(), ChatRequest{Model: "m", Messages: msgs}, TurnEvents{})
	if err != nil {
		t.Fatalf("混合回执路径失败: %v", err)
	}
	if res.FinishReason != FinishStop {
		t.Fatalf("终态: %+v", res)
	}
}

func TestRunTurnUpstreamError(t *testing.T) {
	// 直接以恒错模型构造门面（fakeLLM 空 script 会越界，不走那条路径）
	reg := ai.NewRegistry()
	f := NewFacade(&errLLM{}, reg, Config{})
	if _, err := f.RunTurn(context.Background(), simpleReq(""), TurnEvents{}); err == nil {
		t.Fatal("上游错误未返回")
	}
}

// errLLM 恒定返回错误的模型桩。
type errLLM struct{}

func (e *errLLM) ChatStream(
	context.Context, []chat.ChatCompletionRequestMessage, []chat.ChatCompletionTool,
	func(string), func([]chat.ChatCompletionMessageToolCall) error,
	func(chat.CompletionUsage), func(string),
) error {
	return context.DeadlineExceeded
}

func TestFuseSystemPrompt(t *testing.T) {
	cfg := Config{BaseSystemPrompt: "业务规则", PassthroughSystem: false}
	got := fuseSystemPrompt(cfg, "人格")
	if !strings.Contains(got, "业务规则") || !strings.Contains(got, "人格") || !strings.Contains(got, "pet_*") {
		t.Fatalf("融合结果: %q", got)
	}
	cfg.PassthroughSystem = true
	if got := fuseSystemPrompt(cfg, "人格"); got != "人格" {
		t.Fatalf("透传模式: %q", got)
	}
	cfg.PassthroughSystem = false
	if got := fuseSystemPrompt(cfg, ""); got != "业务规则" {
		t.Fatalf("空人格: %q", got)
	}
}
