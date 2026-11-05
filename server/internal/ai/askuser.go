// internal/ai/askuser.go
//
// 交互式澄清（类似 Trae 的 AskUserQuestion）：模型对需求拿不准时，
// 调用 ask_user 列出 2~4 个候选方案——前端渲染成按钮，玩家点选
// （或自己输入文字）后作为下一轮消息发回，对话继续。
//
// 流程：
//   模型调 ask_user(question, options[]) → 工具经 ProgressEvent(clarify)
//   推给前端（SSE event: clarify）渲染选项卡 → 工具返回给模型
//   "选项已展示，等待玩家选择" → 模型输出一句简短引导结束本轮 →
//   玩家点按钮/自定义输入 → 前端把所选文本作为普通用户消息发送。
//
// 不做服务端阻塞等待：选项选择就是下一轮对话（会话锁天然串行），
// 模型在历史里能看到 ask_user 的调用与玩家明示的选择，上下文自然连贯。
package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// askUserToolName 工具名（前端据此识别，不渲染普通工具进度条）
const askUserToolName = "ask_user"

// ClarifyOption 一个候选选项（label 简短；description 补充说明可为空）
type ClarifyOption struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// maxClarifyOptions 选项数上限（太多反而增加玩家选择负担）
const maxClarifyOptions = 4

// registerAskUser 注册交互式澄清工具（Agent 构造时调用）。
// 执行副作用只有 emitProgress——把选项推给前端渲染；返回给模型的
// 是"等待玩家选择"的说明，模型据此收尾本轮输出。
func registerAskUser(a *Agent) {
	props := map[string]any{
		"question": StrProp("要向玩家确认的问题，一句话说清（如：你更看重哪个方向？）"),
		"options": map[string]any{
			"type":        "array",
			"description": "候选选项列表（2~4 个）",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"label":       StrProp("选项短语（按钮文案，尽量短）"),
					"description": StrProp("选项补充说明（可空）"),
				},
				"required": []string{"label"},
			},
		},
	}
	a.tools.Register(NewFunctionTool(
		askUserToolName,
		"当玩家的需求存在多种理解、缺少关键约束（如预算/优先级/口径），或你准备给出的方案之间差异很大、不确定玩家想要哪种时，调用本工具向玩家展示候选选项让 TA 选择或自己补充——不要凭猜测直接给单一方案。问题清晰、只有一种合理理解时不要调用。",
		props, []string{"question", "options"},
		func(ctx context.Context, rawArgs json.RawMessage) (any, error) {
			var in struct {
				Question string          `json:"question"`
				Options  []ClarifyOption `json:"options"`
			}
			if err := json.Unmarshal(rawArgs, &in); err != nil {
				return nil, fmt.Errorf("参数解析失败: %w", err)
			}
			in.Question = strings.TrimSpace(in.Question)
			// 清洗：剥换行控制字符（选项文案会渲染进 DOM/回传模型，按不可信输入处理）
			opts := make([]ClarifyOption, 0, len(in.Options))
			for _, o := range in.Options {
				o.Label = feedbackClean(o.Label, 40)
				o.Description = feedbackClean(o.Description, 80)
				if o.Label != "" {
					opts = append(opts, o)
				}
				if len(opts) >= maxClarifyOptions {
					break
				}
			}
			if in.Question == "" || len(opts) < 2 {
				return map[string]any{
					"status": "invalid", "message": "question 与至少 2 个有效 options 必填；请修正后重试，或直接给出你的最佳方案",
				}, nil
			}
			// 推给前端渲染选项卡（trace 未启用/无人监听时为 no-op）
			emitProgress(ctx, ProgressEvent{
				Kind:    ProgressClarify,
				Text:    feedbackClean(in.Question, 200),
				Options: opts,
			})
			return map[string]any{
				"status":  "shown",
				"message": "选项已展示给玩家。请用一两句话说明你在等待 TA 的选择（例如列出方案权衡的简短提示），不要自行展开任何方案，结束本轮回复。",
			}, nil
		},
	))
}
