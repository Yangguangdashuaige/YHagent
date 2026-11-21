// internal/openaicompat/prompt.go
//
// 系统提示词融合：YHagent 的 Delta 业务提示词（34 阶段沉淀）在前，
// 桌宠人格融合段居中，Coopanion 发来的 instructions（Cortico 组装的
// 宠物人格 + 环境提示）在末——业务规则优先级高于表达风格，
// 人格层只约束"怎么说"，不约束"答什么"。
package openaicompat

import "strings"

// fusionBlock 业务提示词与桌宠人格之间的融合说明（固定文案）。
const fusionBlock = `你正在通过桌宠和玩家、直播观众互动。pet_say 的 script 会显示在气泡里并播报，details 会显示在聊天页。
1. 查价先给具体卡名和金额。今日预测同时给出时段，并说明它是预测低价。明确问实时挂单价时查物品价格工具。
2. 用 pet_say 交付可见答案。script 至少给出关键数值，多条价格、步骤和来源完整写入 details。旧版工具没有 details 时，直接在 script 给出必要明细。先把工具跑完，再交付答案，回执后避免重复。
3. 弹幕追问如果没有换题，沿用同一观众正在谈的对象。优先完成已经开始的查价，进场、点赞和无关弹幕无需抢先回答。
4. 以下是桌宠人格与环境设定：`

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
