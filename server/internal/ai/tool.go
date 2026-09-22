// internal/ai/tool.go
package ai

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/go-faster/jx"
	"github.com/volcengine/ark-runtime-go/arkruntime/model/chat"
)

// Tool 是 Agent 可调用工具的统一抽象。
// 后续新增任何能力（三角洲API、本地计算等）只需要实现该接口并注册到 Registry，
// Agent 核心循环不需要做任何改动。
type Tool interface {
	// Definition 返回豆包 Function Call 规范的工具定义（名称、描述、参数JSON Schema）
	Definition() chat.ChatCompletionTool
	// Execute 执行工具，rawArgs 为模型生成的原始 JSON 入参，返回值会被序列化成 JSON 回传给模型
	Execute(ctx context.Context, rawArgs json.RawMessage) (any, error)
}

// ToolHandler 是函数型工具的执行签名，配合 NewFunctionTool 使用，避免每个工具都写一个结构体
type ToolHandler func(ctx context.Context, rawArgs json.RawMessage) (any, error)

// FunctionTool 最常用的工具实现：一份豆包 Function 定义 + 一个处理函数
type FunctionTool struct {
	definition chat.ChatCompletionTool
	handler    ToolHandler
}

// NewFunctionTool 构造一个函数型工具。
// name 只能包含 a-z A-Z 0-9 下划线和中划线；description 写给模型看，决定何时调用；
// properties 为参数 JSON Schema 的 properties 片段；required 为必填参数名列表，无必填传 nil。
func NewFunctionTool(name, description string, properties map[string]any, required []string, handler ToolHandler) *FunctionTool {
	// 按豆包 Function Call 规范组装 parameters：{"type":"object","properties":{...},"required":[...]}
	schema := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}

	return &FunctionTool{
		definition: chat.ChatCompletionTool{
			Type: chat.ToolTypeFunction,
			Function: chat.FunctionObject{
				Name:        name,
				Description: chat.NewOptString(description),
				Parameters:  chat.NewOptFunctionObjectParameters(toFunctionParameters(schema)),
			},
		},
		handler: handler,
	}
}

// Name 返回工具名，注册中心内部使用
func (t *FunctionTool) Name() string {
	return t.definition.Function.Name
}

func (t *FunctionTool) Definition() chat.ChatCompletionTool {
	return t.definition
}

func (t *FunctionTool) Execute(ctx context.Context, rawArgs json.RawMessage) (any, error) {
	return t.handler(ctx, rawArgs)
}

// toFunctionParameters 把普通 map 形式的 JSON Schema 转成 SDK 需要的 map[string]jx.Raw
func toFunctionParameters(schema map[string]any) chat.FunctionObjectParameters {
	params := make(chat.FunctionObjectParameters, len(schema))
	for key, value := range schema {
		raw, err := json.Marshal(value)
		if err != nil {
			// schema 都是代码里写死的字面量，理论上不会走到这里
			raw = jx.Raw(`null`)
		}
		params[key] = jx.Raw(raw)
	}
	return params
}

// ---- JSON Schema 片段辅助函数，让参数定义保持简洁且符合豆包规范 ----

// StrProp 字符串类型参数描述
func StrProp(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

// IntProp 整数类型参数描述
func IntProp(description string) map[string]any {
	return map[string]any{"type": "integer", "description": description}
}

// EnumProp 带枚举候选值的字符串参数描述
func EnumProp(description string, values ...string) map[string]any {
	return map[string]any{"type": "string", "description": description, "enum": values}
}

// EnumIntProp 带枚举候选值的整数参数描述
func EnumIntProp(description string, values ...int) map[string]any {
	return map[string]any{"type": "integer", "description": description, "enum": values}
}

// Registry 工具统一注册中心，线程安全。
// Agent 只依赖注册中心拿到工具定义和执行入口，不感知任何具体工具。
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
	order []string // 保留注册顺序，保证每次下发给模型的工具列表稳定
}

// NewRegistry 创建空的工具注册中心
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

// Register 注册一个工具，重名会被覆盖
func (r *Registry) Register(tool Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	name := tool.Definition().Function.Name
	if _, exists := r.tools[name]; !exists {
		r.order = append(r.order, name)
	}
	r.tools[name] = tool
}

// Get 按名称查找工具
func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tool, ok := r.tools[name]
	return tool, ok
}

// Names 返回已注册的全部工具名（按注册顺序）
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, len(r.order))
	copy(names, r.order)
	return names
}

// Definitions 返回下发给豆包的全部工具定义（按注册顺序）
func (r *Registry) Definitions() []chat.ChatCompletionTool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	defs := make([]chat.ChatCompletionTool, 0, len(r.order))
	for _, name := range r.order {
		defs = append(defs, r.tools[name].Definition())
	}
	return defs
}
