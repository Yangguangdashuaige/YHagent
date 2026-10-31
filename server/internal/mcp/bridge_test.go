package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"yhagent-server/internal/ai"
)

// newTestRegistry 注册两个假工具：一个正常返回、一个恒定报错
func newTestRegistry(t *testing.T) *ai.Registry {
	t.Helper()
	reg := ai.NewRegistry()
	reg.Register(ai.NewFunctionTool("echo_tool", "回显输入文本", map[string]any{
		"text": ai.StrProp("要回显的文本"),
	}, []string{"text"}, func(_ context.Context, raw json.RawMessage) (any, error) {
		var in struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, err
		}
		return map[string]any{"echo": in.Text}, nil
	}))
	reg.Register(ai.NewFunctionTool("boom_tool", "恒定失败的工具", nil, nil, func(context.Context, json.RawMessage) (any, error) {
		return nil, errors.New("上游接口暂时不可用")
	}))
	return reg
}

// startClient 起一个挂好工具的 MCP 服务端，经内存传输连接出客户端会话
func startClient(t *testing.T, reg *ai.Registry, exclude map[string]bool) *mcpsdk.ClientSession {
	t.Helper()
	server := NewServer()
	if _, err := Mount(server, reg, exclude); err != nil {
		t.Fatalf("Mount 失败: %v", err)
	}
	ct, st := mcpsdk.NewInMemoryTransports()
	go func() {
		_ = server.Run(context.Background(), st)
	}()
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test-client", Version: "0"}, nil)
	cs, err := client.Connect(context.Background(), ct, nil)
	if err != nil {
		t.Fatalf("客户端连接失败: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func TestExcludeSet(t *testing.T) {
	// 空串 → 默认名单
	got := ExcludeSet("")
	if !got["web_search"] || !got["plan_and_execute"] || len(got) != 2 {
		t.Errorf("默认名单应为 web_search+plan_and_execute，得到 %v", got)
	}
	// 自定义覆盖
	got = ExcludeSet(" web_search , gun_combo_solve ,")
	if len(got) != 2 || !got["web_search"] || !got["gun_combo_solve"] {
		t.Errorf("自定义名单解析错误: %v", got)
	}
}

func TestMountExcludeAndList(t *testing.T) {
	reg := newTestRegistry(t)
	cs := startClient(t, reg, ExcludeSet("boom_tool"))

	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools 失败: %v", err)
	}
	if len(res.Tools) != 1 {
		t.Fatalf("排除 boom_tool 后应只剩 1 个工具，得到 %d", len(res.Tools))
	}
	tool := res.Tools[0]
	if tool.Name != "echo_tool" || tool.Description != "回显输入文本" {
		t.Errorf("工具元信息应原样透传: %+v", tool)
	}
	// InputSchema 必须是 object 且带 properties
	schema, _ := tool.InputSchema.(map[string]any)
	if schema == nil || schema["type"] != "object" {
		t.Fatalf("inputSchema 应为 object 类型: %v", tool.InputSchema)
	}
	if props, _ := schema["properties"].(map[string]any); props == nil || props["text"] == nil {
		t.Errorf("inputSchema 应携带 text 参数定义: %v", schema)
	}
}

func TestCallRoundTrip(t *testing.T) {
	cs := startClient(t, newTestRegistry(t), nil)

	res, err := cs.CallTool(context.Background(), &mcpsdk.CallToolParams{
		Name:      "echo_tool",
		Arguments: map[string]any{"text": "你好"},
	})
	if err != nil {
		t.Fatalf("CallTool 失败: %v", err)
	}
	if res.IsError {
		t.Fatalf("正常调用不应报错: %+v", res)
	}
	if len(res.Content) != 1 {
		t.Fatalf("应返回 1 段内容，得到 %d", len(res.Content))
	}
	text, ok := res.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("内容应为文本: %T", res.Content[0])
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(text.Text), &payload); err != nil {
		t.Fatalf("工具返回应为 JSON 文本: %q", text.Text)
	}
	if payload["echo"] != "你好" {
		t.Errorf("参数应经 JSON 往返原样到达工具，得到 %v", payload)
	}
}

func TestCallToolErrorIsToolLevel(t *testing.T) {
	cs := startClient(t, newTestRegistry(t), nil)

	// 工具执行失败：IsError=true 的工具级结果（而非协议错误），
	// 客户端 LLM 才能看到失败原因并自纠
	res, err := cs.CallTool(context.Background(), &mcpsdk.CallToolParams{Name: "boom_tool"})
	if err != nil {
		t.Fatalf("工具失败不应上升为协议错误: %v", err)
	}
	if !res.IsError {
		t.Errorf("失败调用应标记 IsError")
	}
	if len(res.Content) != 1 {
		t.Fatalf("应返回 1 段错误说明，得到 %d", len(res.Content))
	}
	if text, _ := res.Content[0].(*mcpsdk.TextContent); text == nil || text.Text != "上游接口暂时不可用" {
		t.Errorf("错误文本应透传工具错误: %+v", res.Content[0])
	}
}
