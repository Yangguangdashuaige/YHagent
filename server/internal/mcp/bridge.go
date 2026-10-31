// Package mcp 把 internal/ai.Registry 中的既有工具桥接为 MCP（Model Context
// Protocol）服务器工具：YHagent 的知识库检索、三角洲数据查询等能力由此可被
// Claude Desktop / Trae / 任意 MCP 客户端直接挂载使用，Agent 工具代码零改动。
//
// 桥接是纯数据转换，两种暴露形态共用本包：
//   - stdio：cmd/mcpserver 编译出独立 yhagent-mcp 二进制，客户端以子进程拉起；
//   - Streamable HTTP：主服务（cmd/server）挂 /mcp 路由，远程挂载。
//
// 转换规则：
//   - 工具定义：豆包 Function 定义（name/description/parameters）→ MCP Tool。
//     parameters 本就是标准 JSON Schema，原样搬运；
//   - 工具调用：MCP 参数是原始 JSON（req.Params.Arguments），与
//     ai.Tool.Execute(ctx, rawArgs) 的入参形态一致，直接透传；
//   - 执行结果：序列化为 JSON 文本内容回传；执行错误映射为 IsError=true 的
//     工具级错误（客户端 LLM 可见、可自纠），不作为协议层错误。
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"yhagent-server/internal/ai"
)

// Version MCP 服务器版本号（build 时可经 -ldflags 注入，默认 dev）。
var Version = "dev"

// DefaultExclude 默认不对外暴露的工具（逗号分隔，环境变量 MCP_EXCLUDE 可覆盖）：
//   - web_search：消耗本服务的方舟联网配额，且 MCP 客户端普遍自带联网搜索；
//   - plan_and_execute：依赖 YHagent 自身会话循环的元工具，外部单独调用无意义。
const DefaultExclude = "web_search,plan_and_execute"

// toolExecuteTimeout 单次工具执行上限，与 Agent 工具循环的 45s 超时对齐，
// 防止异常工具把外部 MCP 客户端无限挂起。
const toolExecuteTimeout = 45 * time.Second

// ExcludeSet 解析逗号分隔的排除名单；空串返回默认名单。
func ExcludeSet(s string) map[string]bool {
	if strings.TrimSpace(s) == "" {
		s = DefaultExclude
	}
	out := map[string]bool{}
	for _, name := range strings.Split(s, ",") {
		if name = strings.TrimSpace(name); name != "" {
			out[name] = true
		}
	}
	return out
}

// NewServer 构造 MCP 服务器实例（stdio 与 HTTP 形态共用）。
func NewServer() *mcpsdk.Server {
	return mcpsdk.NewServer(&mcpsdk.Implementation{
		Name:    "yhagent",
		Version: Version,
	}, nil)
}

// NewHTTPHandler 构造 Streamable HTTP 形态处理器（挂主服务 /mcp 路由）。
// 鉴权与限流由调用方（internal/server）包裹。
func NewHTTPHandler(server *mcpsdk.Server) http.Handler {
	return mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server { return server }, nil)
}

// StdioTransport stdio 传输（类型别名转发，调用方无需直接依赖 SDK）。
type StdioTransport = mcpsdk.StdioTransport

// Mount 把 reg 中不在排除名单内的工具逐一注册到 MCP 服务器，返回注册数量。
func Mount(server *mcpsdk.Server, reg *ai.Registry, exclude map[string]bool) (int, error) {
	mounted := 0
	for _, name := range reg.Names() {
		if exclude[name] {
			continue
		}
		tool, ok := reg.Get(name)
		if !ok {
			continue
		}
		fn := tool.Definition().Function
		if fn.Name == "" {
			fn.Name = name
		}
		// FunctionObject 的 Opt 包装字段（Strict 未设置时）整体序列化会触发
		// 豆包 SDK 的 MarshalJSON bug，改为逐字段直取。
		desc, _ := fn.Description.Get()
		schema := map[string]any{}
		if params, ok := fn.Parameters.Get(); ok {
			for k, raw := range params {
				var v any
				if err := json.Unmarshal([]byte(raw), &v); err != nil {
					return mounted, fmt.Errorf("解析工具 %s 参数 schema 的 %s 字段失败: %w", fn.Name, k, err)
				}
				schema[k] = v
			}
		}
		// MCP 要求 inputSchema.type 必须为 object；NewFunctionTool 产物本就满足，
		// 这里兜底防御其他来源的注册（缺 parameters 时给空对象 schema）。
		if t, _ := schema["type"].(string); t != "object" {
			schema["type"] = "object"
		}
		server.AddTool(&mcpsdk.Tool{
			Name:        fn.Name,
			Description: desc,
			InputSchema: schema,
		}, bridgeHandler(tool))
		mounted++
	}
	return mounted, nil
}

// bridgeHandler 把单个 ai.Tool 的执行包成 MCP 工具处理器。
func bridgeHandler(tool ai.Tool) mcpsdk.ToolHandler {
	return func(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
		ctx, cancel := context.WithTimeout(ctx, toolExecuteTimeout)
		defer cancel()
		out, err := tool.Execute(ctx, req.Params.Arguments)
		if err != nil {
			// 工具执行失败按 MCP 语义返回工具级错误（IsError + 文本说明），
			// 让客户端 LLM 能看到失败原因并自纠；返回 error 会变成协议层错误。
			return &mcpsdk.CallToolResult{
				IsError: true,
				Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: err.Error()}},
			}, nil
		}
		data, err := json.Marshal(out)
		if err != nil {
			return &mcpsdk.CallToolResult{
				IsError: true,
				Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: "工具结果序列化失败: " + err.Error()}},
			}, nil
		}
		return &mcpsdk.CallToolResult{
			Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: string(data)}},
		}, nil
	}
}
