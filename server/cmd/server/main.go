// cmd/server/main.go
//
// 远端 Agent 服务入口：部署到你的云服务器运行（yhagent-server 模块）。
// 豆包多轮 Function Call（internal/ai）、三角洲23个工具（internal/data/deltaapi）、
// 共享缓存（internal/data/cache）全部只在服务端存在：
//   - 缓存和 API Key 集中在服务器，十万客户端共享一份缓存；
//   - 三角洲客户端已内置 singleflight，缓存过期瞬间只打一次上游；
//   - 对外只暴露 SSE 流式接口，凭 Bearer 令牌访问。
//
// 环境变量（.env 或系统环境变量）：
//
//	SERVER_ADDR   监听地址，默认 :8080
//	SERVER_TOKEN  客户端访问令牌（必填，建议用长随机串）
//	ARK_API_KEY / ARK_MODEL_ENDPOINT / DELTA_API_KEY ... 见 .env.example
package main

import (
	"context"
	"errors"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"yhagent-server/internal/ai"
	"yhagent-server/internal/auth"
	"yhagent-server/internal/bootstrap"
	"yhagent-server/internal/config"
	"yhagent-server/internal/ingest"
	"yhagent-server/internal/mcp"
	"yhagent-server/internal/server"
	"yhagent-server/internal/trace"
	"yhagent-server/internal/websearch"
	"yhagent-server/web"
)

func main() {
	// 1. 加载服务端配置（server/.env，全部业务密钥只存在于服务端）
	log.Println("[server] 加载配置文件...")
	if err := config.Load(); err != nil {
		log.Fatalf("配置加载失败: %v", err)
	}

	// 2. 初始化豆包客户端（Agent 与工具集共用同一实例）
	aiClient := ai.NewSeedClient()

	agentOpts := []ai.Option{}
	// 会话持久化：SESSION_DB 配置后启用（SQLite），重启/空闲淘汰后按 sessionID 无感恢复历史；
	// 未配置则纯内存（旧行为）。历史图片不落盘（占位文本替代），行保留 7 天。
	if dbPath := os.Getenv("SESSION_DB"); dbPath != "" {
		st, err := ai.NewSQLiteSessionStore(dbPath)
		if err != nil {
			log.Fatalf("会话持久化初始化失败: %v", err)
		}
		agentOpts = append(agentOpts, ai.WithSessionStore(st))
		defer st.Close()
		log.Printf("[server] 会话持久化已启用：db=%s", dbPath)
	}
	// 长期记忆：MEMORY_DB 配置后启用（SQLite）。登录用户轮末异步提取用户事实/偏好，
	// 新会话注入画像块——跨会话"认识"用户；未配置或匿名（主令牌）请求无此能力。
	if dbPath := os.Getenv("MEMORY_DB"); dbPath != "" {
		mst, err := ai.NewSQLiteMemoryStore(dbPath)
		if err != nil {
			log.Fatalf("长期记忆初始化失败: %v", err)
		}
		agentOpts = append(agentOpts, ai.WithMemoryStore(mst))
		defer mst.Close()
		log.Printf("[server] 长期记忆已启用：db=%s", dbPath)
	}
	// 反馈闭环 + few-shot 案例库（自学习）：默认启用 data/feedback.db，显式设
	// FEEDBACK_DB=0 关闭。网页端每条回答可 👍👎：点赞问答自动进案例库，后续相似
	// 提问注入成功答法；点踩关联 trace_id 落库供失败模式汇总分析。
	feedbackPath := strings.TrimSpace(os.Getenv("FEEDBACK_DB"))
	if feedbackPath == "" {
		feedbackPath = "data/feedback.db"
	}
	var feedbackSvc *ai.FeedbackService
	if feedbackPath != "0" {
		fst, err := ai.NewSQLiteFeedbackStore(feedbackPath)
		if err != nil {
			log.Fatalf("反馈闭环初始化失败: %v", err)
		}
		feedbackSvc = ai.NewFeedbackService(fst)
		agentOpts = append(agentOpts, ai.WithFeedbackService(feedbackSvc))
		defer fst.Close()
		log.Printf("[server] 反馈闭环已启用：db=%s", feedbackPath)
	}
	agent := ai.NewAgent(aiClient, agentOpts...)

	// 3.1 业务工具统一装配（cmd/mcpserver 共用 internal/bootstrap，避免两处复制漂移）：
	//     三角洲 API / 改枪求解器 / 知识库 / 联网搜索，容错语义见 bootstrap 包注释
	tools, err := bootstrap.AssembleTools(agent.Tools(), aiClient, "", "")
	if err != nil {
		log.Fatalf("[server] 工具装配失败: %v", err)
	}

	// 3.2b Plan-and-Execute 元工具（可选）：对比/多实体类任务由模型自主选择
	// 一次性下发计划并行执行；默认关闭，AGENT_PLAN_TOOL=1 启用（A/B 对比实验开关）
	if os.Getenv("AGENT_PLAN_TOOL") == "1" {
		agent.EnablePlanTool()
		log.Printf("[server] plan_and_execute 元工具已启用（模型可选的规划式执行模式）")
	}

	// 3.3 联网知识采纳入库服务：候选摘要经 AI 整理成 md 后写入补充索引
	var ingester server.Ingester
	if tools.ExtraStore != nil {
		ingester = ingestAdapter{s: ingest.New(tools.ExtraStore, aiClient, aiClient)}
	}

	// 3.5 MCP 服务（可选）：MCP_TOKEN 配置后主服务挂 /mcp 路由（Streamable HTTP），
	//     Claude Desktop / Trae 等外部 MCP 客户端可远程挂载 YHagent 工具集；
	//     令牌独立于 SERVER_TOKEN，工具白名单由 MCP_EXCLUDE 控制（默认排除
	//     web_search / plan_and_execute）。本地子进程形态见 cmd/mcpserver。
	var mcpHandler http.Handler
	if tok := strings.TrimSpace(os.Getenv("MCP_TOKEN")); tok != "" {
		mcpServer := mcp.NewServer()
		n, err := mcp.Mount(mcpServer, agent.Tools(), mcp.ExcludeSet(os.Getenv("MCP_EXCLUDE")))
		if err != nil {
			log.Fatalf("[server] MCP 工具挂载失败: %v", err)
		}
		mcpHandler = mcp.NewHTTPHandler(mcpServer)
		log.Printf("[server] MCP 服务已启用：/mcp（Streamable HTTP，暴露 %d 个工具）", n)
	}

	// 3.4 请求级 trace 记录器：JSONL 按天轮转落盘（默认 data/trace，保留 7 天）；
	//     Agent 内部经 internal/trace 埋点写入，网页版 /trace.html 经 /v1/traces 查看
	traceDir := os.Getenv("TRACE_DIR")
	keepDays := 7
	if v := os.Getenv("TRACE_KEEP_DAYS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			keepDays = n
		}
	}
	// 会话上下文 token 预算：超限触发滚动摘要压缩（不配置则维持固定 20 条窗口）
	if v := os.Getenv("SESSION_TOKEN_BUDGET"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			ai.SessionTokenBudget = n
		}
	}
	if ai.SessionTokenBudget > 0 {
		log.Printf("[server] 会话上下文 token 预算 %d（超限触发滚动摘要压缩）", ai.SessionTokenBudget)
	}
	trace.Default = trace.NewRecorder(traceDir, keepDays)
	if trace.Default != nil {
		defer trace.Default.Close()
		if traceDir == "" {
			traceDir = "data/trace" // 仅用于日志展示，NewRecorder 内部使用同一默认值
		}
		log.Printf("[server] 请求 trace 已启用：目录=%s 保留=%d天", traceDir, keepDays)
	} else {
		log.Printf("[server] 请求 trace 未启用（trace 目录创建失败，仅影响可观测性）")
	}

	log.Printf("[server] Agent初始化完成，已注册 %d 个工具（共享缓存已启用）", len(agent.Tools().Names()))

	// 4. 启动 HTTP/SSE 服务（网页版静态资源编译期嵌入，部署仍是单个二进制）
	webFS, err := fs.Sub(web.Dist, "dist")
	if err != nil {
		log.Fatalf("网页资源加载失败: %v", err)
	}
	// 入口保护（可选环境变量，留空走内置默认值）：
	// CHAT_MAX_CONCURRENT 全局在途对话上限（默认 128）；CHAT_QUEUE_TIMEOUT 满员后
	// 排队等待上限秒数（默认不限时——排队时长由用户决定，网页端可随时「停止排队」；
	// 设置后超时自动拒绝）；CHAT_RPM_PER_IP 单 IP 每分钟请求上限（默认 30，=0 可关）
	maxConcurrent, rpmPerIP, queueWait := 0, 0, time.Duration(0)
	if v := os.Getenv("CHAT_MAX_CONCURRENT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxConcurrent = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("CHAT_QUEUE_TIMEOUT")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			queueWait = time.Duration(n) * time.Second
		}
	}
	// 显式 =0 表示关闭按 IP 限流（配置值映射为 -1 传入）；不配置则为 0 走默认阈值
	if raw, ok := os.LookupEnv("CHAT_RPM_PER_IP"); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil && n >= 0 {
			if n == 0 {
				rpmPerIP = -1
			} else {
				rpmPerIP = n
			}
		}
	}
	// 账号系统：AUTH_DB 配置后启用（未配置则账号接口 503、鉴权退回纯主令牌，旧客户端零影响）
	var authService *auth.Service
	if dbPath := os.Getenv("AUTH_DB"); dbPath != "" {
		// AUTH_TOKEN_TTL 单位为小时，缺省 720（30 天）
		ttlHours := 720
		if raw, ok := os.LookupEnv("AUTH_TOKEN_TTL"); ok {
			if n, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil && n > 0 {
				ttlHours = n
			}
		}
		allowReg := !strings.EqualFold(strings.TrimSpace(os.Getenv("AUTH_ALLOW_REGISTER")), "false")
		authService, err = auth.Open(dbPath, time.Duration(ttlHours)*time.Hour, allowReg, os.Getenv("AUTH_ADMIN_KEY"))
		if err != nil {
			log.Fatalf("账号系统初始化失败: %v", err)
		}
		defer authService.Close()
		adminKeyOn := os.Getenv("AUTH_ADMIN_KEY") != ""
		log.Printf("[server] 账号系统已启用：db=%s 令牌有效期=%dh 注册=%v 管理员密钥=%v",
			dbPath, ttlHours, allowReg, adminKeyOn)
	}
	srv, err := server.New(server.Config{
		Addr:              os.Getenv("SERVER_ADDR"),
		Token:             os.Getenv("SERVER_TOKEN"),
		Chat:              agent,
		Web:               webFS,
		AdminToken:        os.Getenv("ADMIN_TOKEN"),
		Ingester:          ingester,
		Tracer:            trace.Default,
		Feedback:          feedbackSvc,
		MaxConcurrentChat: maxConcurrent,
		ChatQueueWait:     queueWait,
		ChatRPMPerIP:      rpmPerIP,
		Auth:              authService,
		MCP:               mcpHandler,
		MCPToken:          os.Getenv("MCP_TOKEN"),
	})
	if err != nil {
		log.Fatalf("服务初始化失败: %v", err)
	}
	log.Printf("[server] 联网知识入库：%s", enabledText(ingester != nil && os.Getenv("ADMIN_TOKEN") != ""))

	// 5. 监听中断信号，优雅关闭（正在进行的流式回答有时间收尾）
	go func() {
		if err := srv.Run(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("服务运行失败: %v", err)
		}
	}()
	log.Println("[server] 服务已就绪，按 Ctrl+C 退出")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("[server] 优雅关闭失败: %v", err)
	}
	log.Println("[server] 已退出")
}

// ingestAdapter 把 server 层 DTO 转成 ingest.Service 入参（隔离 websearch 包类型），
// 并把 server.IngestProgress 桥接为 ingest.Progress（经 context 注入服务层）。
type ingestAdapter struct {
	s *ingest.Service
}

func (a ingestAdapter) Ingest(ctx context.Context, in server.IngestInput, onProgress func(server.IngestProgress)) (string, int, string, error) {
	sources := make([]websearch.Source, 0, len(in.Sources))
	for _, s := range in.Sources {
		sources = append(sources, websearch.Source{Title: s.Title, URL: s.URL})
	}
	if onProgress != nil {
		ctx = ingest.WithProgress(ctx, func(p ingest.Progress) {
			onProgress(server.IngestProgress{
				Stage: p.Stage,
				Done:  p.Done,
				Total: p.Total,
				Delta: p.Delta,
			})
		})
	}
	return a.s.Ingest(ctx, ingest.Request{
		Query:   in.Query,
		Summary: in.Summary,
		Sources: sources,
	})
}

func enabledText(on bool) string {
	if on {
		return "已开启（ADMIN_TOKEN 已配置）"
	}
	return "未开启（未配置 ADMIN_TOKEN 时采纳接口返回 503）"
}
