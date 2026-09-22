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
	"syscall"
	"time"

	"yhagent-server/internal/ai"
	"yhagent-server/internal/config"
	"yhagent-server/internal/data/cache"
	"yhagent-server/internal/data/deltaapi"
	"yhagent-server/internal/server"
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
	log.Printf("[server] Agent初始化完成，已注册 %d 个工具（共享缓存已启用）", len(agent.Tools().Names()))

	// 4. 启动 HTTP/SSE 服务（网页版静态资源编译期嵌入，部署仍是单个二进制）
	webFS, err := fs.Sub(web.Dist, "dist")
	if err != nil {
		log.Fatalf("网页资源加载失败: %v", err)
	}
	srv, err := server.New(server.Config{
		Addr:  os.Getenv("SERVER_ADDR"),
		Token: os.Getenv("SERVER_TOKEN"),
		Chat:  agent,
		Web:   webFS,
	})
	if err != nil {
		log.Fatalf("服务初始化失败: %v", err)
	}

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
