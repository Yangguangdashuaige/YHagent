package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"yhagent-server/internal/ai"
	"yhagent-server/internal/mcp"
)

// TestMCPRouteAuth /mcp 路由鉴权：独立 MCP 令牌，主令牌与账号令牌均不能混用
func TestMCPRouteAuth(t *testing.T) {
	reached := 0
	s, err := New(Config{
		Token: "master-token",
		Chat:  stubChat{},
		MCP: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			reached++
			w.WriteHeader(http.StatusOK)
		}),
		MCPToken:     "mcp-secret",
		ChatRPMPerIP: -1, // 关闭限流，聚焦鉴权语义
	})
	if err != nil {
		t.Fatalf("构建服务: %v", err)
	}

	call := func(bearer string) int {
		t.Helper()
		r := httptest.NewRequest("POST", "/mcp", nil)
		if bearer != "" {
			r.Header.Set("Authorization", "Bearer "+bearer)
		}
		w := httptest.NewRecorder()
		s.http.Handler.ServeHTTP(w, r)
		return w.Code
	}

	// 无令牌 / 错误令牌 / 主令牌（SERVER_TOKEN 不是 MCP 凭据）→ 401
	if c := call(""); c != http.StatusUnauthorized {
		t.Errorf("无令牌应 401，得到 %d", c)
	}
	if c := call("wrong"); c != http.StatusUnauthorized {
		t.Errorf("错误令牌应 401，得到 %d", c)
	}
	if c := call("master-token"); c != http.StatusUnauthorized {
		t.Errorf("主令牌不能访问 /mcp，应 401，得到 %d", c)
	}
	if reached != 0 {
		t.Errorf("未鉴权通过前 handler 不应被触达")
	}
	// 正确 MCP 令牌 → 进入 MCP 处理器
	if c := call("mcp-secret"); c != http.StatusOK {
		t.Errorf("正确 MCP 令牌应 200，得到 %d", c)
	}
	if reached != 1 {
		t.Errorf("鉴权通过后 handler 应被触达一次，实际 %d 次", reached)
	}
}

// TestMCPRouteDisabled 未配置 MCP（handler 或令牌为空）时路由不注册 → 404
func TestMCPRouteDisabled(t *testing.T) {
	s, err := New(Config{Token: "t", Chat: stubChat{}, ChatRPMPerIP: -1})
	if err != nil {
		t.Fatalf("构建服务: %v", err)
	}
	// 只配 handler 不配令牌：拒绝裸奔，不注册路由
	s2, err := New(Config{
		Token:        "t",
		Chat:         stubChat{},
		MCP:          http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
		ChatRPMPerIP: -1,
	})
	if err != nil {
		t.Fatalf("构建服务: %v", err)
	}
	for _, srv := range []*Server{s, s2} {
		r := httptest.NewRequest("POST", "/mcp", nil)
		w := httptest.NewRecorder()
		srv.http.Handler.ServeHTTP(w, r)
		if w.Code != http.StatusNotFound {
			t.Errorf("未启用 MCP 时 /mcp 应 404，得到 %d", w.Code)
		}
	}
}

// authRoundTripper 给 MCP 客户端传输注入 Bearer 令牌
type authRoundTripper struct {
	base  http.RoundTripper
	token string
}

func (a *authRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	r.Header.Set("Authorization", "Bearer "+a.token)
	return a.base.RoundTrip(r)
}

// TestMCPHTTPEndToEnd 主服务 /mcp 路由端到端：真实鉴权中间件 + SDK HTTP 客户端
// 完成 initialize → tools/list → tools/call 全链路（不涉 LLM，工具为本地桩）
func TestMCPHTTPEndToEnd(t *testing.T) {
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
	mcpServer := mcp.NewServer()
	if _, err := mcp.Mount(mcpServer, reg, nil); err != nil {
		t.Fatalf("MCP 工具挂载: %v", err)
	}

	s, err := New(Config{
		Token:        "master-token",
		Chat:         stubChat{},
		MCP:          mcp.NewHTTPHandler(mcpServer),
		MCPToken:     "mcp-secret",
		ChatRPMPerIP: -1,
	})
	if err != nil {
		t.Fatalf("构建服务: %v", err)
	}
	ts := httptest.NewServer(s.http.Handler)
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	connect := func(token string) (*mcpsdk.ClientSession, error) {
		client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test-client", Version: "0"}, nil)
		return client.Connect(ctx, &mcpsdk.StreamableClientTransport{
			Endpoint: ts.URL + "/mcp",
			HTTPClient: &http.Client{
				Transport: &authRoundTripper{base: http.DefaultTransport, token: token},
			},
			DisableStandaloneSSE: true, // 纯请求-响应场景，避免测试挂常驻 SSE 流
		}, nil)
	}

	// 错误令牌：initialize 阶段就被 401 拒绝
	if _, err := connect("wrong-token"); err == nil {
		t.Errorf("错误令牌不应建立会话")
	}

	// 正确令牌：全链路
	cs, err := connect("mcp-secret")
	if err != nil {
		t.Fatalf("正确令牌建立会话失败: %v", err)
	}
	defer cs.Close()

	res, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools 失败: %v", err)
	}
	if len(res.Tools) != 1 || res.Tools[0].Name != "echo_tool" {
		t.Fatalf("应能列出挂载工具，得到 %d 个", len(res.Tools))
	}
	call, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "echo_tool",
		Arguments: map[string]any{"text": "hi"},
	})
	if err != nil || call.IsError {
		t.Fatalf("CallTool 失败: err=%v result=%+v", err, call)
	}
	if text, _ := call.Content[0].(*mcpsdk.TextContent); text == nil || !json.Valid([]byte(text.Text)) {
		t.Errorf("工具结果应为 JSON 文本: %+v", call.Content[0])
	}
}
