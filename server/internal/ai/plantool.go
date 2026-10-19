// internal/ai/plantool.go
//
// plan_and_execute 元工具：把 Plan-and-Execute 范式做成模型可选的一个"工具"，
// 而不是架构的必经之路——需要多步规划的复杂任务（对比多把枪、多实体聚合、
// 查完数据再选方案）由模型自己决定调用，一次性下发完整计划；
// 简单任务继续走原有反应式（ReAct）路径，两种范式天然共存。
//
// 执行语义：步骤间无依赖（依赖表达是 DSL 滑坡，刻意不做），全部并行执行，
// 耗时从累加变取最大值；每个步骤复用 executeToolSafe（panic 恢复/超时/
// trace 记录/SSE 工具事件一应俱全）；结果统一打包回传，下一轮直接合成答案。
package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/volcengine/ark-runtime-go/arkruntime/model/chat"
)

const (
	planToolName     = "plan_and_execute"
	planMaxSteps     = 5       // 计划最多步骤数（防模型生成超长计划拖垮请求）
	planStepMaxBytes = 8 << 10 // 单步骤结果回传上限（计划总结果控制在 ~40KB 内）
)

// planStepArgs plan_and_execute 的步骤参数（模型按 schema 生成）
type planStepArgs struct {
	Tool    string         `json:"tool"`
	Args    map[string]any `json:"args"`
	Purpose string         `json:"purpose"`
}

// EnablePlanTool 注册 plan_and_execute 元工具（AGENT_PLAN_TOOL=1 时调用）。
// dispatch 复用 Agent.executeToolSafe 执行单步（panic 恢复/超时/trace/SSE 全继承）；
// 步骤的 trace 轮次固定记 0（执行循环从 1 起，0 专属“规划步骤”语义）。
func (a *Agent) EnablePlanTool() {
	a.tools.Register(newPlanTool(a.tools, func(ctx context.Context, call chat.ChatCompletionMessageToolCall) string {
		return a.executeToolSafe(ctx, 0, call)
	}))
}

func newPlanTool(registry *Registry, dispatch func(context.Context, chat.ChatCompletionMessageToolCall) string) *FunctionTool {
	return NewFunctionTool(
		planToolName,
		"多步任务规划执行器。当任务需要查询多个互不相关的对象（如「对比A和B两把枪的改装方案」"+
			"「AKM、M4A1和SCAR-H哪把适合新手」「查一下M14和SR-25的热门方案再对比价格」）时，"+
			"把全部查询步骤一次性作为计划下发，各步骤会并行执行，结果统一返回后你直接对比作答。"+
			"步骤间不能引用其他步骤的结果（有依赖的任务请走普通的多轮工具调用）；"+
			"简单的单一查询（如「今天的密码」）不需要本工具。每个步骤必须使用真实存在的工具名。",
		map[string]any{
			"steps": map[string]any{
				"type":        "array",
				"description": "执行计划，2-5 个步骤，每个步骤是一次独立的工具调用",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"tool":    StrProp("要调用的工具名，必须从可用工具列表中选择"),
						"args":    map[string]any{"type": "object", "description": "该工具的调用参数（JSON 对象）"},
						"purpose": StrProp("这一步要查什么，一句话"),
					},
					"required": []string{"tool", "args"},
				},
			},
		},
		[]string{"steps"},
		func(ctx context.Context, rawArgs json.RawMessage) (any, error) {
			var args struct {
				Steps []planStepArgs `json:"steps"`
			}
			if err := json.Unmarshal(rawArgs, &args); err != nil {
				return nil, fmt.Errorf("参数格式错误: %w", err)
			}
			if len(args.Steps) < 2 {
				return nil, fmt.Errorf("计划至少需要 2 个步骤；单一查询请直接调用对应工具")
			}
			if len(args.Steps) > planMaxSteps {
				args.Steps = args.Steps[:planMaxSteps]
			}

			// 前置校验：工具必须存在且禁止嵌套规划（防失控递归）
			for i, s := range args.Steps {
				if s.Tool == planToolName {
					return nil, fmt.Errorf("第 %d 步不允许嵌套调用 plan_and_execute", i+1)
				}
				if _, ok := registry.Get(s.Tool); !ok {
					return nil, fmt.Errorf("第 %d 步的工具 %s 不存在，请从可用工具列表中选择", i+1, s.Tool)
				}
				if s.Args == nil {
					args.Steps[i].Args = map[string]any{}
				}
			}

			start := time.Now()
			results := make([]string, len(args.Steps))
			wg := &sync.WaitGroup{}
			for i, s := range args.Steps {
				wg.Add(1)
				go func(i int, s planStepArgs) {
					defer wg.Done()
					argsJSON, err := json.Marshal(s.Args)
					if err != nil {
						results[i] = toolErrorJSON("步骤参数序列化失败")
						return
					}
					// 复用 executeToolSafe：超时/panic 恢复/trace/SSE 工具事件全部继承
					results[i] = dispatch(ctx, chat.ChatCompletionMessageToolCall{
						ID:   fmt.Sprintf("plan_%d", i+1),
						Type: chat.ToolTypeFunction,
						Function: chat.ChatCompletionMessageToolCallFunction{
							Name:      s.Tool,
							Arguments: string(argsJSON),
						},
					})
				}(i, s)
			}
			wg.Wait()

			// 打包：先给计划总览（模型一眼看清各步状态），再给完整数据
			planOverview := make([]map[string]any, len(args.Steps))
			stepData := make([]map[string]any, len(args.Steps))
			for i, s := range args.Steps {
				r := results[i]
				failed := isStepError(r)
				planOverview[i] = map[string]any{
					"步骤":    i + 1,
					"工具":    s.Tool,
					"用途":    s.Purpose,
					"状态":    boolLabel(!failed),
					"结果字节数": len(r),
				}
				stepData[i] = map[string]any{
					"步骤": i + 1,
					"工具": s.Tool,
					"数据": truncateResultBytes(r, planStepMaxBytes),
				}
			}
			return map[string]any{
				"计划执行总览": planOverview,
				"各步骤结果":  stepData,
				"提示": "以上是按计划并行执行的全部结果（总耗时 " + time.Since(start).String() + "）。" +
					"请直接基于这些结果对比、聚合并回答用户，不要对相同对象重复调用工具；" +
					"若某步骤失败且影响结论，向用户说明该部分数据缺失。",
			}, nil
		},
	)
}

func boolLabel(ok bool) string {
	if ok {
		return "成功"
	}
	return "失败"
}

// isStepError 步骤结果是否为错误响应（executeToolSafe 失败路径统一产出 {"error":...}）
func isStepError(resultJSON string) bool {
	var probe struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(resultJSON), &probe); err != nil {
		return true // 连 JSON 都不是，按失败处理
	}
	return probe.Error != ""
}

// truncateResultBytes 按字节截断步骤结果。截断处可能是半个 UTF-8 字符，
// json.Marshal 会把无效字节替换为 U+FFFD，天然安全；上限兜底防单步超大结果
// （executeToolSafe 已有 32KB 硬限，这里进一步压计划打包总大小）
func truncateResultBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "...（截断）"
}
