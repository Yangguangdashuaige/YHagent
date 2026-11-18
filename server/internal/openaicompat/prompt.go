// internal/openaicompat/prompt.go
//
// 系统提示词融合：YHagent 的 Delta 业务提示词（34 阶段沉淀）在前，
// 桌宠人格融合段居中，Coopanion 发来的 instructions（Cortico 组装的
// 宠物人格 + 环境提示）在末——业务规则优先级高于表达风格，
// 人格层只约束"怎么说"，不约束"答什么"。
package openaicompat

import "strings"

// fusionBlock 业务提示词与桌宠人格之间的融合说明（固定文案）。
const fusionBlock = `你当前正以"桌面宠物"形态与玩家实时互动，回复正文会以打字机气泡逐字显示。
补充规则：
1. 正文保持口语化、短句优先，避免长篇大论；多条目数据（价格对比、改枪配件清单）仍按业务规则用 Markdown 表格。
2. 需要让宠物说话、移动、做动作、向玩家提问或展示选项时，调用对应的 pet_* 工具——它们在玩家电脑上执行，调用后等待回执再继续。
3. 以下是你当前的桌宠人格与环境设定，遵守其中的表达风格与称呼习惯：`

// fuseSystemPrompt 融合系统提示词。PassthroughSystem=true 时纯透传
// 客户端 instructions（调试模式：观察 Coopanion 原生行为）；
// 客户端未发 instructions 时（异构客户端直连）退化为纯业务提示词。
func fuseSystemPrompt(cfg Config, clientSystem string) string {
	clientSystem = strings.TrimSpace(clientSystem)
	if cfg.PassthroughSystem {
		return clientSystem
	}
	if clientSystem == "" {
		return cfg.BaseSystemPrompt
	}
	return cfg.BaseSystemPrompt + "\n\n" + fusionBlock + "\n\n" + clientSystem
}
