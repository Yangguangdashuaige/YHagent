package openaicompat

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/volcengine/ark-runtime-go/arkruntime/model/chat"
	"yhagent-server/internal/ai"
)

const diyPlan = `{"name":"自定义","jz":188102,"price":149800,"data":[{"type":"枪械1","name":"Vector冲锋枪","price":104800},{"type":"枪械1-枪托","name":"Vector共振一体式枪托","price":30000},{"type":"护甲","name":"尼龙防弹衣 (破损)","price":15000}]}`

func TestReadyReplyDeliversEquipmentBeforeAnotherModelRound(t *testing.T) {
	market := `[{"name":"市场","zb":600000,"list":[{"jz":600200,"price":498000,"data":[{"type":"枪1","name":"SR9射手步枪"},{"type":"枪1-瞄具","name":"八倍瞄准镜"},{"type":"枪2","name":"SR9射手步枪"},{"type":"枪2-枪口","name":"轻语消音器"},{"type":"护甲","name":"尼龙防弹衣"}],"unused":"` + strings.Repeat("a", maxToolResultBytes) + `"},{"jz":599999,"price":10,"data":[{"type":"胸挂","name":"未达门槛"}]}]},{"name":"部门兑换","zb":600000,"list":[{"jz":601000,"price":10,"data":[{"type":"护甲","name":"部门装备","exchange":"兑换"}]}]}]`
	for _, tc := range []struct {
		name, frame, tool, args, payload string
		mixed, details, failedReceipt    bool
		want                             []string
	}{
		{"60w voice", "[voice] 玩家:我想卡1个60万的战备，该怎么赔养？", "delta_ready_build_v4", `{"lv":3}`, market, false, true, false,
			[]string{"一号枪用SR9射手步枪", "一号枪的瞄具用八倍瞄准镜", "二号枪用SR9射手步枪", "二号枪的枪口用轻语消音器", "尼龙防弹衣", "600200", "498000"}},
		{"187500 follow-up", "[voice] 玩家:那我如果想卡1个187500的战备呢？", "delta_diy_ready_build", `{"zb":187500}`, diyPlan, true, true, false,
			[]string{"Vector冲锋枪", "Vector共振一体式枪托", "尼龙防弹衣 (破损)", "188102", "149800"}},
		{"legacy tool", "[弹幕|玩家] 帮我凑18.75万战备", "delta_diy_ready_build", `{"zb":187500}`, diyPlan, false, false, true,
			[]string{"Vector冲锋枪", "Vector共振一体式枪托", "尼龙防弹衣 (破损)", "188102", "149800"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := []chat.ChatCompletionMessageToolCall{testReadyCall("lookup", tc.tool, tc.args)}
			if tc.mixed {
				calls = append(calls, testReadyCall("summary", "pet_say", `{"script":"查到了，已经给你配好。"}`))
			}
			f, llm := testReadyFacade(calls, tc.payload)
			defer close(f.relays.closed)
			tool := petSayTool()
			if tc.details {
				tool.Function.Parameters = json.RawMessage(`{"properties":{"script":{"type":"string"},"details":{"type":"string"}}}`)
			}
			frame, _ := json.Marshal("[1 new event]\n" + tc.frame)
			req := ChatRequest{Tools: []WireTool{tool}, Messages: []WireMessage{
				{Role: "assistant", ToolCalls: []WireToolCall{{ID: "evf_1", Type: "function", Function: &WireToolCallFn{Name: "external_event_frame", Arguments: "{}"}}}},
				{Role: "tool", ToolCallID: "evf_1", Content: frame},
				{Role: "user", Content: json.RawMessage(`"[system] 1 new event"`)},
			}}
			res, err := f.RunTurn(context.Background(), req, TurnEvents{})
			if err != nil || llm.callCount != 1 || res.FinishReason != FinishToolCalls || len(res.ClientCalls) != 1 {
				t.Fatalf("expected direct delivery, got %+v, %v (%d model calls)", res, err, llm.callCount)
			}
			call := res.ClientCalls[0]
			var reply petReply
			if err := json.Unmarshal([]byte(call.Function.Arguments), &reply); err != nil {
				t.Fatal(err)
			}
			for _, value := range tc.want {
				if !strings.Contains(reply.Script, value) {
					t.Fatalf("spoken plan missing %q: %s", value, reply.Script)
				}
			}
			if strings.Contains(reply.Script, "查到了") || (reply.Details != "") != tc.details ||
				(tc.details && !strings.Contains(reply.Details, "实际挂单价格可能变动")) {
				t.Fatalf("unexpected reply: %+v", reply)
			}
			receipt := json.RawMessage(`"已开始显示,这段约 12 秒。"`)
			if tc.failedReceipt {
				receipt = json.RawMessage(`"[pet_say 没执行] 窗口已关闭"`)
			}
			req.Messages = append(req.Messages, WireMessage{Role: "assistant", ToolCalls: []WireToolCall{call}},
				WireMessage{Role: "tool", ToolCallID: call.ID, Content: receipt})
			ack, err := f.RunTurn(context.Background(), req, TurnEvents{})
			if err != nil || ack.FinishReason != FinishStop {
				t.Fatalf("receipt %+v, %v", ack, err)
			}
			if tc.failedReceipt {
				if llm.callCount != 2 || ack.Text == "" || !strings.Contains(messageContentText(llm.lastMsgs[len(llm.lastMsgs)-1]), "窗口已关闭") {
					t.Fatalf("failed delivery should continue in the model: %+v, calls %d", ack, llm.callCount)
				}
			} else if llm.callCount != 1 || ack.Text != "" {
				t.Fatalf("successful delivery should end without repeating: %+v, calls %d", ack, llm.callCount)
			}
		})
	}
}

func TestReadyReplyLeavesUnmetTargetsAndComparisonsToModel(t *testing.T) {
	for _, tc := range []struct{ name, question, args, payload string }{
		{"below minimum", "卡187500战备", `{"zb":187500}`, strings.Replace(diyPlan, "188102", "187499", 1)},
		{"rounded target", "卡187500战备", `{"zb":180000}`, diyPlan},
		{"extra constraint", "卡187500战备，不带护甲行吗？", `{"zb":187500}`, diyPlan},
		{"comparison", "对比两套187500战备", `{"zb":187500}`, diyPlan},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, llm := testReadyFacade([]chat.ChatCompletionMessageToolCall{testReadyCall("lookup", "delta_diy_ready_build", tc.args)}, tc.payload)
			defer close(f.relays.closed)
			question, _ := json.Marshal(tc.question)
			res, err := f.RunTurn(context.Background(), ChatRequest{Tools: []WireTool{petSayTool()}, Messages: []WireMessage{{Role: "user", Content: question}}}, TurnEvents{})
			if err != nil || llm.callCount != 2 || res.FinishReason != FinishStop {
				t.Fatalf("fallback got %+v, %v (%d model calls)", res, err, llm.callCount)
			}
		})
	}
}

func testReadyCall(id, name, args string) chat.ChatCompletionMessageToolCall {
	return chat.ChatCompletionMessageToolCall{ID: id, Type: chat.ToolTypeFunction, Function: chat.ChatCompletionMessageToolCallFunction{Name: name, Arguments: args}}
}

func testReadyFacade(calls []chat.ChatCompletionMessageToolCall, payload string) (*Facade, *fakeLLM) {
	reg := ai.NewRegistry()
	reg.Register(ai.NewFunctionTool(calls[0].Function.Name, "fixture", nil, nil, func(context.Context, json.RawMessage) (any, error) {
		return json.RawMessage(payload), nil
	}))
	llm := &fakeLLM{script: []fakeTurn{{toolCalls: calls}, {text: "按工具结果补充答复。"}}}
	return NewFacade(llm, reg, Config{}), llm
}
