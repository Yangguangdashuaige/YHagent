// internal/ai/toolevent.go
//
// 工具执行状态回调：Agent 每次工具调用的开始/结束经 context 回调通知
// SSE 层（服务端注入，推 event: tool 给网页端做实时进度显示）。
// 与 websearch 的候选卡片回调同款模式：context 注入、无回调时静默跳过、零接口侵入。
package ai

import "context"

// ToolEvent 一次工具调用的状态变化（服务端 → 网页端 SSE tool 事件）
type ToolEvent struct {
	Name  string `json:"name"`            // 工具名，如 web_search
	Phase string `json:"phase"`           // "start" 开始执行 / "end" 执行结束
	MS    int64  `json:"ms,omitempty"`    // end 时携带：执行耗时
	OK    bool   `json:"ok,omitempty"`    // end 时携带：是否成功
	Label string `json:"label,omitempty"` // 展示用中文标签（由 SSE 层映射，如「联网搜索」）
}

type toolEventFuncKey struct{}

// WithToolEventFunc 把工具状态回调注入 context（SSE 层注入；f 为 nil 时原样返回）
func WithToolEventFunc(ctx context.Context, f func(ToolEvent)) context.Context {
	if f == nil {
		return ctx
	}
	return context.WithValue(ctx, toolEventFuncKey{}, f)
}

func emitToolEvent(ctx context.Context, e ToolEvent) {
	if f, ok := ctx.Value(toolEventFuncKey{}).(func(ToolEvent)); ok && f != nil {
		f(e)
	}
}
