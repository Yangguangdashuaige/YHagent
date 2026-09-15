package main

import (
	"log"

	"YHagent/internal/ai"
	"YHagent/internal/config"
	// 👉 后续新增模块在这里补充导入
	// "YHagent/internal/data/storage"
	// "YHagent/internal/data/deltaapi"
	// "YHagent/internal/data/cache"
	// "YHagent/internal/ui"
	// "YHagent/internal/hotkey"
)

func main() {
	// 1. 加载配置
	log.Println("[init] 加载配置文件...")
	if err := config.Load(); err != nil {
		log.Fatalf("配置加载失败: %v", err)
	}
	log.Println("[init] 配置加载完成")

	// 2. 初始化豆包AI客户端
	log.Println("[init] 初始化豆包AI客户端...")
	aiClient := ai.NewSeedClient()
	_ = aiClient // 后续传给Agent实例使用
	log.Println("[init] 豆包AI客户端初始化完成")

	// ==============================================
	// 👉 后续模块初始化位置：实现一个解开一个注释即可
	// ==============================================

	// 3. 初始化SQLite存储 + 向量检索
	// log.Println("[init] 初始化本地存储...")
	// store, err := storage.NewSQLite("./data/yhagent.db")
	// if err != nil {
	// 	log.Fatalf("存储初始化失败: %v", err)
	// }
	// defer store.Close()
	// log.Println("[init] 本地存储初始化完成")

	// 4. 初始化三角洲开放平台API客户端
	// log.Println("[init] 初始化三角洲API客户端...")
	// deltaClient := deltaapi.NewClient(config.AppConfig.DeltaAPIKey, config.AppConfig.DeltaAPIBase)
	// log.Println("[init] 三角洲API客户端初始化完成")

	// 5. 初始化缓存模块
	// log.Println("[init] 初始化缓存模块...")
	// cache := cache.NewCache()
	// log.Println("[init] 缓存模块初始化完成")

	// 6. 初始化会话管理器
	// log.Println("[init] 初始化会话管理器...")
	// sessionMgr := ai.NewSessionManager(store)
	// log.Println("[init] 会话管理器初始化完成")

	// 7. 初始化Agent核心，注册工具处理器
	// log.Println("[init] 初始化Agent核心...")
	// agent := ai.NewAgent(aiClient)
	// // 注册三角洲数据查询工具
	// agent.RegisterToolHandler("query_weapon", deltaClient.QueryWeapon)
	// agent.RegisterToolHandler("query_map_point", deltaClient.QueryMapPoint)
	// agent.RegisterToolHandler("calc_profit", deltaClient.CalcProfit)
	// log.Println("[init] Agent核心初始化完成，工具注册成功")

	// 8. 注册全局快捷键
	// log.Println("[init] 注册全局快捷键...")
	// hotkey.Register(config.AppConfig.Hotkey, ui.ToggleWindow)
	// log.Printf("[init] 全局快捷键 %s 注册成功", config.AppConfig.Hotkey)

	// 9. 启动Wails窗口
	// log.Println("[init] 启动悬浮窗...")
	// err := ui.Run(agent, sessionMgr, config.AppConfig)
	// if err != nil {
	// 	log.Fatalf("窗口启动失败: %v", err)
	// }

	// 临时测试：暂时保持程序运行，方便验证初始化流程
	log.Println("[init] 所有基础模块初始化完成，程序启动成功")
	select {}
}
