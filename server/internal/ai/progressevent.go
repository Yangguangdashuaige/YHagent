// internal/ai/progressevent.go
//
// Agent 执行进度事件：把「当前第几轮、正在思考还是执行工具、模型的推理过程、
// 计划步骤」经 context 回调实时推给 SSE 层（服务端转发 event: round/thought/plan），
// 让用户在等待回答时看到完整的执行过程，而不是一个不动的「思考中」。
// 与 ToolEvent 同款模式：context 注入、无回调时静默跳过、零接口侵入。
package ai

import "context"

// ProgressKind 进度事件类型
const (
	// ProgressRound 一轮 LLM 循环的状态变化（phase: thinking=模型思考中 / tools=工具执行中）
	ProgressRound = "round"
	// ProgressThought 模型思考模型的推理流分片（reasoning_content，仅思考模型产出）
	ProgressThought = "thought"
	// ProgressPlan plan_and_execute 下发的计划步骤列表
	ProgressPlan = "plan"
	// ProgressTrace 本轮请求的 trace 标识（前端反馈接口据此关联 trace 观测数据）
	ProgressTrace = "trace"
	// ProgressClarify ask_user 工具推给前端的候选选项（玩家点选或自定义输入）
	ProgressClarify = "clarify"
)

// PlanStepInfo 计划步骤概要（plan 事件携带）
type PlanStepInfo struct {
	Index   int    `json:"index"`           // 步骤序号（1 起）
	Tool    string `json:"tool"`            // 工具名
	Purpose string `json:"purpose"`         // 这一步要查什么（模型生成的一句话）
	Label   string `json:"label,omitempty"` // 展示用中文标签
}

// ProgressEvent 一次执行进度变化（服务端 → 网页端 SSE）
type ProgressEvent struct {
	Kind    string            `json:"kind"`                // round / thought / plan / trace / clarify
	Round   int               `json:"round,omitempty"`     // round：当前 LLM 轮次（1 起）
	Phase   string            `json:"phase,omitempty"`     // round：thinking / tools
	Count   int               `json:"count,omitempty"`     // round+tools：本轮工具调用数
	Text    string            `json:"text,omitempty"`      // thought：推理流分片；clarify：问题文本
	TraceID string            `json:"trace_id,omitempty"`  // trace：本轮请求标识（trace 未启用为空）
	Steps   []PlanStepInfo    `json:"steps,omitempty"`     // plan：计划步骤
	Options []ClarifyOption   `json:"options,omitempty"`   // clarify：候选选项
}

type progressFuncKey struct{}

// WithProgressFunc 把进度回调注入 context（SSE 层注入；f 为 nil 时原样返回）
func WithProgressFunc(ctx context.Context, f func(ProgressEvent)) context.Context {
	if f == nil {
		return ctx
	}
	return context.WithValue(ctx, progressFuncKey{}, f)
}

func emitProgress(ctx context.Context, e ProgressEvent) {
	if f, ok := ctx.Value(progressFuncKey{}).(func(ProgressEvent)); ok && f != nil {
		f(e)
	}
}
