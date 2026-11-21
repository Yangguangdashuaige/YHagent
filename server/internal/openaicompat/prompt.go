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
2. 问怎么卡战备时，直接给一套能照着买和装的方案。首条 script 说出具体枪、哪些配件装在哪把枪上和其他装备，再报预计花费与实际战备值。用 details 列出每个部位、重复件和兑换项。问非固定门槛（如 187500）按原数值调自定义工具，换门槛时为新门槛重新查询。
3. 问怎么做、买什么、任务要带什么时，首条可见回复就给具体步骤、物品或条件。script 先说玩家马上可以执行的内容，多个步骤、数值和来源完整写入 details。旧版工具没有 details 时，必要明细全部写入 script。
4. 一次发起所有互不依赖的查询。查完马上用 pet_say 交付，省去「查到了」「有几套方案」之类的空泛开场。收到成功回执后结束，避免再复述。如果查询失败，说明哪项缺失。
5. 弹幕追问如果没有换题，沿用同一观众正在谈的对象。优先完成当前问答，进场、点赞和无关弹幕无需抢先回答。
6. 以下是桌宠人格与环境设定：`

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
