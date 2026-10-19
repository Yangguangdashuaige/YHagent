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
	"syscall"
	"time"

	"yhagent-server/internal/ai"
	"yhagent-server/internal/config"
	"yhagent-server/internal/data/cache"
	"yhagent-server/internal/data/deltaapi"
	"yhagent-server/internal/ingest"
	"yhagent-server/internal/knowledge"
	"yhagent-server/internal/knowledge/embedded"
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

	// 2. 初始化豆包客户端 + Agent + 三角洲工具
	aiClient := ai.NewSeedClient()
	deltaClient := deltaapi.NewClient(config.AppConfig.DeltaAPIKey, config.AppConfig.DeltaAPIBase)

	// 3. 全用户共享缓存：挂载后所有人命中同一份数据，配合 singleflight 上游调用量与用户数脱钩
	sharedCache := cache.NewCache()
	deltaClient.WithCache(sharedCache)

	agent := ai.NewAgent(aiClient)
	deltaapi.RegisterTools(agent.Tools(), deltaClient)

	// 3.1 加载编译期嵌入的静态知识库主索引；再叠加运行时可写的补充知识库
	//     （玩家确认采纳的联网知识存在 KB_EXTRA_DIR，默认 data/kb-extra，重启不丢）。
	kbStore, kbErr := knowledge.DecodeGOB(embedded.IndexGOB)
	var kbSearcher knowledge.Searcher
	var extraStore *knowledge.ExtraStore
	var loadErr error
	if kbErr != nil {
		log.Printf("[server] 知识库主索引加载失败，search_knowledge/web_search 入库能力不可用: %v", kbErr)
	} else if kbStore.Len() > 0 {
		extraDir := os.Getenv("KB_EXTRA_DIR")
		if extraDir == "" {
			extraDir = "data/kb-extra" // 相对于 systemd WorkingDirectory（部署为 /opt/yhagent）
		}
		extraStore, loadErr = knowledge.LoadExtraStore(kbStore, extraDir)
		if loadErr != nil {
			log.Fatalf("[server] 补充知识库初始化失败: %v", loadErr)
		}
		kbSearcher = extraStore
		// 检索重排器：配置了 VikingDB AK/SK 走专用重排模型（优先），
		// 否则配置了 ARK_RERANK_LLM_MODEL 走 LLM 重排；都未配置为 nil（不重排）
		var reranker ai.Reranker
		if v := ai.NewVikingReranker(); v != nil {
			reranker = v
		} else if l := ai.NewLLMReranker(aiClient); l != nil {
			reranker = l
		}
		if reranker != nil {
			log.Printf("[server] 知识库检索重排已启用：%s", reranker.Name())
		}
		knowledge.RegisterTool(agent.Tools(), kbSearcher, aiClient, reranker)
		log.Printf("[server] 静态知识库已加载：%d 主切片 + %d 补充切片（%s，%d维）",
			kbStore.Len(), extraStore.ExtraLen(), kbStore.Model, kbStore.Dimension)
	} else {
		log.Printf("[server] 静态知识库为空，search_knowledge 未注册（放文档后运行 cmd/indexer 建库）")
	}

	// 3.2 注册联网搜索工具（需在火山方舟控制台开通「联网内容插件」，未开通时工具返回友好错误）
	webClient := websearch.New(
		config.AppConfig.DoubaoAPIKey,
		config.AppConfig.DoubaoBaseURL,
		config.AppConfig.DoubaoEndpoint,
	)
	websearch.RegisterTool(agent.Tools(), webClient)

	// 3.3 联网知识采纳入库服务：候选摘要经 AI 整理成 md 后写入补充索引
	var ingester server.Ingester
	if extraStore != nil {
		ingester = ingestAdapter{s: ingest.New(extraStore, aiClient, aiClient)}
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
	srv, err := server.New(server.Config{
		Addr:       os.Getenv("SERVER_ADDR"),
		Token:      os.Getenv("SERVER_TOKEN"),
		Chat:       agent,
		Web:        webFS,
		AdminToken: os.Getenv("ADMIN_TOKEN"),
		Ingester:   ingester,
		Tracer:     trace.Default,
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
