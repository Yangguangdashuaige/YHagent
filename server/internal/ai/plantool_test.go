package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"github.com/volcengine/ark-runtime-go/arkruntime/model/chat"
)

// fakeDispatch 返回固定成功结果，记录收到的调用
func fakeDispatch(calls *[]string) func(context.Context, chat.ChatCompletionMessageToolCall) string {
	return func(_ context.Context, call chat.ChatCompletionMessageToolCall) string {
		*calls = append(*calls, call.Function.Name+"|"+call.Function.Arguments)
		return `{"ok":true}`
	}
}

func newTestPlanRegistry() *Registry {
	r := NewRegistry()
	r.Register(NewFunctionTool("delta_gun_build", "查改枪方案", map[string]any{
		"gun": StrProp("枪名"),
	}, []string{"gun"}, func(ctx context.Context, args json.RawMessage) (any, error) {
		return map[string]any{"方案": "测试方案"}, nil
	}))
	return r
}

func planArgsJSON(t *testing.T, steps []planStepArgs) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(map[string]any{"steps": steps})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestPlanToolExecution(t *testing.T) {
	reg := newTestPlanRegistry()
	var mu sync.Mutex
	calls := []string{}
	tool := newPlanTool(reg, func(ctx context.Context, call chat.ChatCompletionMessageToolCall) string {
		mu.Lock()
		calls = append(calls, call.Function.Name)
		mu.Unlock()
		return `{"方案":"测试方案"}`
	})

	raw := planArgsJSON(t, []planStepArgs{
		{Tool: "delta_gun_build", Args: map[string]any{"gun": "K416"}, Purpose: "查K416方案"},
		{Tool: "delta_gun_build", Args: map[string]any{"gun": "M14"}, Purpose: "查M14方案"},
	})
	out, err := tool.Execute(context.Background(), raw)
	if err != nil {
		t.Fatalf("计划执行失败: %v", err)
	}

	m := out.(map[string]any)
	overview := m["计划执行总览"].([]map[string]any)
	if len(overview) != 2 {
		t.Fatalf("总览应含 2 步, got %d", len(overview))
	}
	if overview[0]["状态"] != "成功" {
		t.Errorf("步骤1状态 = %v, want 成功", overview[0]["状态"])
	}
	data := m["各步骤结果"].([]map[string]any)
	if len(data) != 2 || data[1]["工具"] != "delta_gun_build" {
		t.Errorf("步骤数据不完整: %+v", data)
	}
	mu.Lock()
	if len(calls) != 2 {
		t.Errorf("dispatch 应被调 2 次, got %d", len(calls))
	}
	mu.Unlock()
	// 整体可序列化（回传模型的最终载荷）
	if _, err := json.Marshal(out); err != nil {
		t.Errorf("计划结果序列化失败: %v", err)
	}
}

func TestPlanToolValidation(t *testing.T) {
	reg := newTestPlanRegistry()
	tool := newPlanTool(reg, fakeDispatch(&[]string{}))
	ctx := context.Background()

	// 单步计划拒绝
	if _, err := tool.Execute(ctx, planArgsJSON(t, []planStepArgs{
		{Tool: "delta_gun_build", Args: map[string]any{"gun": "K416"}},
	})); err == nil {
		t.Error("单步计划应被拒绝")
	}

	// 未知工具拒绝
	if _, err := tool.Execute(ctx, planArgsJSON(t, []planStepArgs{
		{Tool: "delta_gun_build", Args: map[string]any{}},
		{Tool: "no_such_tool", Args: map[string]any{}},
	})); err == nil {
		t.Error("未知工具应被拒绝")
	}

	// 嵌套 plan_and_execute 拒绝
	reg.Register(tool)
	if _, err := tool.Execute(ctx, planArgsJSON(t, []planStepArgs{
		{Tool: "delta_gun_build", Args: map[string]any{}},
		{Tool: "plan_and_execute", Args: map[string]any{}},
	})); err == nil {
		t.Error("嵌套 plan_and_execute 应被拒绝")
	}

	// 步骤超上限截断到 5（不报错）
	steps := make([]planStepArgs, 8)
	for i := range steps {
		steps[i] = planStepArgs{Tool: "delta_gun_build", Args: map[string]any{"gun": fmt.Sprintf("枪%d", i)}}
	}
	out, err := tool.Execute(ctx, planArgsJSON(t, steps))
	if err != nil {
		t.Fatalf("超长计划应截断而非报错: %v", err)
	}
	m := out.(map[string]any)
	if got := len(m["计划执行总览"].([]map[string]any)); got != planMaxSteps {
		t.Errorf("计划应截断到 %d 步, got %d", planMaxSteps, got)
	}
}

func TestPlanToolStepFailureMarked(t *testing.T) {
	reg := newTestPlanRegistry()
	tool := newPlanTool(reg, func(_ context.Context, call chat.ChatCompletionMessageToolCall) string {
		if call.Function.Arguments == `{"gun":"M14"}` {
			return toolErrorJSON("工具执行失败：数据源超时")
		}
		return `{"方案":"ok"}`
	})
	out, err := tool.Execute(context.Background(), planArgsJSON(t, []planStepArgs{
		{Tool: "delta_gun_build", Args: map[string]any{"gun": "K416"}, Purpose: "成功步骤"},
		{Tool: "delta_gun_build", Args: map[string]any{"gun": "M14"}, Purpose: "失败步骤"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	overview := out.(map[string]any)["计划执行总览"].([]map[string]any)
	if overview[0]["状态"] != "成功" || overview[1]["状态"] != "失败" {
		t.Errorf("步骤状态标记错误: %+v", overview)
	}
}

func TestIsStepError(t *testing.T) {
	if !isStepError(toolErrorJSON("超时")) {
		t.Error("错误 JSON 应判为失败")
	}
	if isStepError(`{"data":123}`) {
		t.Error("正常 JSON 不应判为失败")
	}
	if !isStepError("not-json") {
		t.Error("非法 JSON 应判为失败")
	}
}
