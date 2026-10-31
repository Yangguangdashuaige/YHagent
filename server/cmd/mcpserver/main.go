// cmd/mcpserver/main.go
//
// yhagent-mcp：把 YHagent 工具集（知识库检索、三角洲数据、改枪求解器等）
// 暴露为标准 MCP（Model Context Protocol）服务器，stdio 形态——
// Claude Desktop / Trae 等 MCP 客户端以子进程方式拉起本二进制直接挂载。
//
// 与主服务（cmd/server）共用 internal/bootstrap 装配与 internal/mcp 桥接：
// 同一份工具代码，两种暴露形态（本进程 stdio / 主服务 /mcp Streamable HTTP）。
//
// 运行要求：工作目录可访问 data/（index.gob、kb-extra、compat/gunparts.json），
// 环境变量提供与主服务相同的密钥（.env：ARK_API_KEY / DELTA_API_KEY ...）。
// 注意：本进程 stdout 是 MCP 协议通道（JSON-RPC），所有日志走 stderr。
//
// 配置：
//
//	MCP_EXCLUDE  不暴露的工具名单（逗号分隔），默认 "web_search,plan_and_execute"
//
// 客户端接入（Claude Desktop / Trae 等 mcpServers 配置）：
//
//	"yhagent": { "command": "yhagent-mcp", "env": { "ARK_API_KEY": "..." } }
package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"syscall"

	"yhagent-server/internal/ai"
	"yhagent-server/internal/bootstrap"
	"yhagent-server/internal/config"
	"yhagent-server/internal/mcp"
)

func main() {
	// stdio 形态：stdout 是 JSON-RPC 协议通道，log 默认输出 stderr 不受影响
	if err := config.Load(); err != nil {
		log.Fatalf("[mcp] 配置加载失败: %v", err)
	}

	// 与主服务共用同一段工具装配（三角洲 API / 改枪求解器 / 知识库 / 联网搜索）
	reg := ai.NewRegistry()
	if _, err := bootstrap.AssembleTools(reg, ai.NewSeedClient(), "", ""); err != nil {
		log.Fatalf("[mcp] 工具装配失败: %v", err)
	}

	server := mcp.NewServer()
	n, err := mcp.Mount(server, reg, mcp.ExcludeSet(os.Getenv("MCP_EXCLUDE")))
	if err != nil {
		log.Fatalf("[mcp] 工具挂载失败: %v", err)
	}
	log.Printf("[mcp] yhagent MCP 服务器就绪（stdio）：已暴露 %d/%d 个工具", n, len(reg.Names()))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatalf("[mcp] 服务退出: %v", err)
	}
}
