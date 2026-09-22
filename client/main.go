// main.go
//
// YHagent 瘦客户端（yhagent-client 模块）唯一入口。
// 客户端只做两件事：显示透明悬浮窗 UI、通过 SSE 与远端 Agent 服务通信。
// 豆包多轮 Function Call、三角洲工具、缓存全部在服务端执行，
// 本模块依赖中不存在豆包 SDK / 三角洲代码，也不持有任何业务 API Key。
package main

import (
	"embed"
	"io/fs"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"

	"yhagent-client/internal/config"
	"yhagent-client/internal/remote"
)

// frontendAssets 前端静态资源编译期直接打进 EXE，发布仍然是单文件
//
//go:embed all:frontend/dist
var frontendAssets embed.FS

func main() {
	// 1. 加载客户端配置（远端地址 + 令牌，缺失则直接退出）
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("配置加载失败: %v", err)
	}
	log.Println("[init] 配置加载完成")

	// 2. 初始化远端 Agent 客户端（提问 POST 到服务器，回答以 SSE 流回传）
	chat, err := remote.New(cfg.RemoteBaseURL, cfg.RemoteToken)
	if err != nil {
		log.Fatalf("远端客户端初始化失败: %v", err)
	}
	log.Printf("[init] 远端客户端就绪，目标服务：%s", cfg.RemoteBaseURL)

	// 3. 创建 Wails 前端绑定层（chatService 由 remote.Client 实现）
	app := NewApp(chat)

	// 4. 启动 Wails 透明悬浮窗
	distFS, err := fs.Sub(frontendAssets, "frontend/dist")
	if err != nil {
		log.Fatalf("前端资源加载失败: %v", err)
	}

	log.Println("[init] 启动悬浮窗...")
	err = wails.Run(&options.App{
		Title:     "三角洲行动AI助手 · 小小轩",
		Width:     cfg.DefaultWidth,
		Height:    cfg.DefaultHeight,
		MinWidth:  360,
		MinHeight: 480,
		Frameless: true, // 无边框：标题栏/拖动/关闭由前端自绘，贴合游戏悬浮窗形态
		// 透明背景：WebView 透明 + 窗口背景全透明，实际视觉由前端 CSS 的圆角半透明面板呈现
		BackgroundColour: options.NewRGBA(0, 0, 0, 0),
		AssetServer: &assetserver.Options{
			Assets: distFS,
		},
		// v2.16 起需显式声明暴露给前端的绑定对象（前端 window.go.main.App 调用）
		Bind:      []interface{}{app},
		OnStartup: app.startup,
		Windows: &windows.Options{
			WebviewIsTransparent: true,
		},
	})
	if err != nil {
		log.Fatalf("窗口启动失败: %v", err)
	}
}
