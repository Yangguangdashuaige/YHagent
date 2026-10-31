// Package bootstrap 统一装配 YHagent 业务工具集（三角洲 API、改枪组合求解器、
// 知识库检索、联网搜索）。cmd/server（主服务）与 cmd/mcpserver（MCP stdio
// 服务器）共用同一段装配逻辑，避免两处复制漂移。
//
// 装配策略与容错语义（沿用原 cmd/server 行为）：
//   - 三角洲 API 工具恒注册（客户端内部自带上游缓存与重试）；
//   - 改枪求解器：数据集 data/compat/gunparts.json 缺失时跳过注册，其余功能不受影响；
//   - 知识库：主索引加载失败/为空时 search_knowledge 不注册（服务整体可用）；
//     补充知识库初始化失败视为致命错误（返回 error，由调用方决定退出）；
//   - 联网搜索恒注册（未开通方舟插件时工具返回友好错误）。
package bootstrap

import (
	"fmt"
	"log"
	"os"

	"yhagent-server/internal/ai"
	"yhagent-server/internal/config"
	"yhagent-server/internal/data/cache"
	"yhagent-server/internal/data/deltaapi"
	"yhagent-server/internal/gunparts"
	"yhagent-server/internal/knowledge"
	"yhagent-server/internal/knowledge/embedded"
	"yhagent-server/internal/websearch"
)

// 装配产物的默认数据路径（相对运行目录；主服务部署时 systemd WorkingDirectory
// 为 /opt/yhagent，MCP stdio 形态则相对客户端拉起时的工作目录）。
const (
	DefaultKBPath       = "data/index.gob"
	DefaultExtraDir     = "data/kb-extra"
	DefaultGunpartsPath = "data/compat/gunparts.json"
)

// Tools 装配产物：调用方据此决定附带能力是否可用
type Tools struct {
	// ExtraStore 运行时可写的补充知识库；主索引未加载时为 nil
	// （主服务据此关闭联网知识采纳入库）。
	ExtraStore *knowledge.ExtraStore
	// KBLoaded 主索引是否成功加载（决定 search_knowledge 是否已注册）。
	KBLoaded bool
}

// AssembleTools 构造全部业务工具并注册到 reg，返回装配产物。
// aiClient 由调用方创建后传入（主服务的 Agent 与工具集共用同一实例）；
// kbPath / gunpartsPath 传空串用默认值。
func AssembleTools(reg *ai.Registry, aiClient *ai.SeedClient, kbPath, gunpartsPath string) (*Tools, error) {
	if kbPath == "" {
		kbPath = DefaultKBPath
	}
	if gunpartsPath == "" {
		gunpartsPath = DefaultGunpartsPath
	}
	out := &Tools{}

	// 1. 三角洲数据工具：全用户共享缓存 + singleflight，上游调用量与用户数脱钩
	deltaClient := deltaapi.NewClient(config.AppConfig.DeltaAPIKey, config.AppConfig.DeltaAPIBase)
	sharedCache := cache.NewCache()
	deltaClient.WithCache(sharedCache)
	deltaapi.RegisterTools(reg, deltaClient)

	// 2. 改枪组合求解器：数据集由 cmd/partssync 生成（产物不入 git，随部署包分发）
	if gp, err := gunparts.NewStore(gunpartsPath); err != nil {
		log.Printf("[tools] 配件数据集加载失败，gun_combo_solve 未注册（运行 cmd/partssync 生成）: %v", err)
	} else {
		gunparts.RegisterTools(reg, gp)
		log.Printf("[tools] 改枪组合求解器已启用：%d 把武器（数据版本 %s）", gp.WeaponCount(), gp.Version())
	}

	// 3. 静态知识库主索引：优先 data/index.gob（部署包分发，产物不进 git），
	//    回退编译期 embedded 兜底（stub，正常为空）；再叠加运行时可写的补充知识库
	//    （玩家确认采纳的联网知识存在 KB_EXTRA_DIR，默认 data/kb-extra，重启不丢）。
	kbBytes, kbErr := knowledge.LoadIndexBytes(kbPath, embedded.IndexGOB)
	if kbErr == nil && len(kbBytes) == 0 {
		kbErr = fmt.Errorf("未找到 %s（部署包需携带 data/index.gob，或本地先运行 cmd/indexer 生成）", kbPath)
	}
	var kbStore *knowledge.Store
	if kbErr == nil {
		kbStore, kbErr = knowledge.DecodeGOB(kbBytes)
	}
	if kbErr != nil {
		log.Printf("[tools] 知识库主索引加载失败，search_knowledge 不可用: %v", kbErr)
	} else if kbStore.Len() == 0 {
		log.Printf("[tools] 静态知识库为空，search_knowledge 未注册（放文档后运行 cmd/indexer 建库）")
	} else {
		extraDir := os.Getenv("KB_EXTRA_DIR")
		if extraDir == "" {
			extraDir = DefaultExtraDir
		}
		extraStore, err := knowledge.LoadExtraStore(kbStore, extraDir)
		if err != nil {
			return out, fmt.Errorf("补充知识库初始化失败: %w", err)
		}
		// 检索重排器：配置了 VikingDB AK/SK 走专用重排模型（优先），
		// 否则配置了 ARK_RERANK_LLM_MODEL 走 LLM 重排；都未配置为 nil（不重排）
		var reranker ai.Reranker
		if v := ai.NewVikingReranker(); v != nil {
			reranker = v
		} else if l := ai.NewLLMReranker(aiClient); l != nil {
			reranker = l
		}
		if reranker != nil {
			log.Printf("[tools] 知识库检索重排已启用：%s", reranker.Name())
		}
		knowledge.RegisterTool(reg, extraStore, aiClient, reranker)
		out.ExtraStore = extraStore
		out.KBLoaded = true
		log.Printf("[tools] 静态知识库已加载：%d 主切片 + %d 补充切片（%s，%d维）",
			kbStore.Len(), extraStore.ExtraLen(), kbStore.Model, kbStore.Dimension)
	}

	// 4. 联网搜索（需在火山方舟控制台开通「联网内容插件」，未开通时工具返回友好错误）
	webClient := websearch.New(
		config.AppConfig.DoubaoAPIKey,
		config.AppConfig.DoubaoBaseURL,
		config.AppConfig.DoubaoEndpoint,
	)
	websearch.RegisterTool(reg, webClient)

	return out, nil
}
