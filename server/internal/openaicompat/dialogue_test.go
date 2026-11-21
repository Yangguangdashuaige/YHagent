package openaicompat

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/volcengine/ark-runtime-go/arkruntime/model/chat"
	"yhagent-server/internal/ai"
	"yhagent-server/internal/data/deltaapi"
	"yhagent-server/internal/trace"
)

func TestTurnInputFindsDanmakuInsteadOfBatchNotice(t *testing.T) {
	data := []WireMessage{
		{Role: "user", Content: json.RawMessage(`"上一轮"`)},
		{Role: "assistant"},
		{Role: "tool", ToolCallID: "evf_15", Content: json.RawMessage(`"[1 new event]\n[弹幕|观众] 帮我查一下航天钥匙卡价格"`)},
		{Role: "user", Content: json.RawMessage(`"[system] 1 new event, 2026-11-22."`)},
	}
	want := "[弹幕|观众] 帮我查一下航天钥匙卡价格"
	if got := turnInput(data, false); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	data = append(data, WireMessage{Role: "assistant"}, WireMessage{Role: "user", Content: json.RawMessage(`"[system] 静默"`)})
	if got := turnInput(data, false); got != "" {
		t.Fatalf("unexpected old question %q", got)
	}
}

func TestDailyKeyPricesAreDeliveredWhenModelOnlySummarizes(t *testing.T) {
	reg := ai.NewRegistry()
	reg.Register(ai.NewFunctionTool("delta_keys_day_price", "fixture", nil, nil, func(context.Context, json.RawMessage) (any, error) {
		return deltaapi.DailyKeyPrices{Map: "航天基地", Cards: []deltaapi.DailyKeyPrice{
			{Name: "西区大门", Price: 800000, Hour: 5}, {Name: "金卡", Price: 3930000, Hour: 11},
		}}, nil
	}))
	lookup := chat.ChatCompletionMessageToolCall{ID: "price", Type: chat.ToolTypeFunction, Function: chat.ChatCompletionMessageToolCallFunction{Name: "delta_keys_day_price", Arguments: `{"mid":2}`}}
	for _, tc := range []struct {
		name   string
		answer fakeTurn
	}{
		{"text only", fakeTurn{text: "价格表已经整理完毕。"}},
		{"generic bubble", fakeTurn{toolCalls: []chat.ChatCompletionMessageToolCall{{ID: "say", Type: chat.ToolTypeFunction, Function: chat.ChatCompletionMessageToolCallFunction{Name: "pet_say", Arguments: `{"script":"表来了，最贵468万。"}`}}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			previous := trace.Default
			recorder := trace.NewRecorder(t.TempDir(), 1)
			trace.Default = recorder
			t.Cleanup(func() { trace.Default = previous })
			f := NewFacade(&fakeLLM{script: []fakeTurn{{toolCalls: []chat.ChatCompletionMessageToolCall{lookup}}, tc.answer}}, reg, Config{})
			defer close(f.relays.closed)
			tool := petSayTool()
			tool.Function.Parameters = json.RawMessage(`{"properties":{"script":{"type":"string"},"details":{"type":"string"}}}`)
			res, err := f.RunTurn(context.Background(), ChatRequest{
				Messages: []WireMessage{{Role: "user", Content: json.RawMessage(`"航天钥匙卡今日价格"`)}}, Tools: []WireTool{tool},
			}, TurnEvents{})
			if err != nil || res.FinishReason != FinishToolCalls || len(res.ClientCalls) != 1 || res.ClientCalls[0].Function.Name != "pet_say" {
				t.Fatalf("got %+v, err %v", res, err)
			}
			var reply petReply
			if err := json.Unmarshal([]byte(res.ClientCalls[0].Function.Arguments), &reply); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(reply.Script, "西区大门，800000 哈夫币，5 点") || !strings.Contains(reply.Details, "金卡　3930000　11:00") {
				t.Fatalf("incomplete answer %+v", reply)
			}
			records := recorder.Recent(1)
			if len(records) != 1 {
				t.Fatalf("expected one trace, got %+v", records)
			}
			var seenInput, seenPet, seenDetails bool
			for _, line := range records[0].Dialogue {
				switch line.Role {
				case "input":
					seenInput = line.Text == "航天钥匙卡今日价格"
				case "pet":
					seenPet = line.Text == reply.Script
				case "details":
					seenDetails = line.Text == reply.Details
				}
			}
			if !seenInput || !seenPet || !seenDetails {
				t.Fatalf("incomplete trace %+v", records[0].Dialogue)
			}
		})
	}
}
