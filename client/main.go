// main.go
//
// YHagent 桌宠客户端（yhagent-client 模块）唯一入口。
// 完全桌宠形态：一个覆盖主屏工作区底部的透明置顶带状窗（Coopanion 几何），
// 宠物沿地面走动、气泡回答、按住左 Alt 说话（SAPI 听写），
// 除宠物命中区外全窗鼠标穿透（dsh-pet 机制）。
// 豆包多轮 Function Call、三角洲工具、缓存全部在服务端执行，
// 本模块不持有任何业务 API Key。
package main

import (
	"context"
	"embed"
	"io/fs"
	"log"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"

	"yhagent-client/internal/asr"
	"yhagent-client/internal/config"
	"yhagent-client/internal/hotkey"
	"yhagent-client/internal/remote"
	"yhagent-client/internal/win"
)

// petWindowTitle 窗口标题（FindWindow 定位用，进程内唯一）
const petWindowTitle = "YHagent-Pet"

// frontendAssets 前端静态资源编译期直接打进 EXE，发布仍然是单文件
//
//go:embed all:frontend/dist
var frontendAssets embed.FS

func main() {
	// 1. 加载客户端配置。缺失不阻断启动（wails build 生成绑定时也会运行 main；
	// 配置错误经桌宠气泡呈现，符合"可观测性失败不阻断业务"范式）
	cfg, cfgErr := config.Load()
	if cfgErr != nil {
		log.Printf("[init] 配置加载失败（继续启动，对话不可用）: %v", cfgErr)
		cfg = &config.Config{PetBandHeight: 260, PetBottomGap: 3, PTTEnabled: true, PTTKeyVK: 0xA4, ASRAutoSend: true}
	} else {
		log.Println("[init] 配置加载完成")
	}

	// 2. 初始化远端 Agent 客户端（提问 POST 到服务器，回答以 SSE 流回传）；
	// 配置缺失时 chat 置 nil，Chat 绑定会给出明确错误而不是崩溃
	var chat *remote.Client
	if cfgErr == nil {
		c, err := remote.New(cfg.RemoteBaseURL, cfg.RemoteToken)
		if err != nil {
			log.Printf("[init] 远端客户端初始化失败（对话不可用）: %v", err)
		} else {
			chat = c
			log.Printf("[init] 远端客户端就绪，目标服务：%s", cfg.RemoteBaseURL)
		}
	}

	// 3. 桌宠基础设施：带状窗控制器 + 听写管理器 + PTT 热键轮询。
	// 注意把 nil *remote.Client 转成 nil 接口，避免"有类型无值"的假非 nil
	var chatSvc chatService
	if chat != nil {
		chatSvc = chat
	}
	configErrText := ""
	if cfgErr != nil {
		configErrText = cfgErr.Error()
	}
	app := NewApp(chatSvc, cfg, configErrText)

	winCtl := win.NewController(petWindowTitle, win.BandConfig{
		Height:     int32(cfg.PetBandHeight),
		BottomGap:  int32(cfg.PetBottomGap),
	})
	winCtl.SetLogger(log.Printf)
	app.AttachWinCtl(winCtl)

	var poller *hotkey.Poller
	if cfg.PTTEnabled {
		asrMgr := asr.NewManager()
		asrMgr.SetLogger(log.Printf)
		app.AttachASR(asrMgr)

		poller = hotkey.NewPoller(cfg.PTTKeyVK, 30*time.Millisecond)
		poller.SetLogger(log.Printf)
		poller.OnDown(app.OnPTTDown) // 按住左 Alt → 开始听写
		poller.OnUp(app.OnPTTUp)     // 松开 → 结束并聚合文本推前端
	}

	// 4. 启动 Wails 透明桌宠窗
	distFS, err := fs.Sub(frontendAssets, "frontend/dist")
	if err != nil {
		log.Fatalf("前端资源加载失败: %v", err)
	}

	log.Println("[init] 启动桌宠...")
	err = wails.Run(&options.App{
		Title:      petWindowTitle,
		Width:      1280, // 初始尺寸仅占位；OnStartup 后由带状窗几何接管贴底
		Height:     260,
		Frameless:  true, // 无边框桌宠形态
		AlwaysOnTop: true, // 悬浮于游戏之上
		DisableResize: true, // 带状窗尺寸由几何巡检管理，不允许用户拖拽
		// 透明背景：WebView 透明 + 窗口背景全透明，视觉全由前端宠物/气泡呈现
		BackgroundColour: options.NewRGBA(0, 0, 0, 0),
		AssetServer: &assetserver.Options{
			Assets: distFS,
		},
		// v2.16 起需显式声明暴露给前端的绑定对象（前端 window.go.main.App 调用）
		Bind:      []interface{}{app},
		OnStartup: func(ctx context.Context) {
			app.startup(ctx)
			// 窗口就绪后再启动桌宠控制（FindWindow 有时序竞争，内部带重试）
			winCtl.Start(200 * time.Millisecond)
			if poller != nil {
				poller.Start()
			}
		},
		OnShutdown: app.shutdown,
		Windows: &windows.Options{
			WebviewIsTransparent: true,
		},
	})
	if err != nil {
		log.Fatalf("窗口启动失败: %v", err)
	}
}
