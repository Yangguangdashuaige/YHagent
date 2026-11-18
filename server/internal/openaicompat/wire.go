// internal/openaicompat/wire.go
//
// OpenAI Chat Completions 兼容协议的线上类型与 SSE 分块构造器。
// 服务对象是 Coopanion（Cortico 传输层）：它把门面当作一个普通的
// OpenAI 兼容模型端点（POST /chat/completions + GET /models），
// 每次请求携带自己的全量对话历史与客户端工具（pet_* 等）schema，
// 无状态语义（服务端不维护会话，上下文以每次请求的 messages 为准）。
//
// 协议形状以 Cortico 源码为准（src/providers/transport/）：
//   - 请求：stream=true + stream_options.include_usage；messages 含
//     system / user（字符串或 text|image_url parts）/ assistant
//     （content + reasoning_content + tool_calls）/ tool（content + tool_call_id）；
//   - 响应：标准 OpenAI chunk 流。缺 finish_reason 会触发客户端
//     ResponseProtocolError（response-assembly.ts 硬校验），usage 由末 chunk
//     携带；客户端只解析 data: 行、[DONE] 终止，未知请求字段一律容忍忽略。
package openaicompat

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"time"
)

// ---- 请求侧类型 ----

// ChatRequest POST /chat/completions 请求体。仅声明门面消费的字段，
// 其余字段（top_p/response_format 等）由 json 解码天然忽略。
type ChatRequest struct {
	Model            string          `json:"model"`
	Messages         []WireMessage   `json:"messages"`
	Tools            []WireTool      `json:"tools,omitempty"`
	Stream           bool            `json:"stream,omitempty"`
	StreamOptions    *WireStreamOpts `json:"stream_options,omitempty"`
	Temperature      *float64        `json:"temperature,omitempty"`
	MaxTokens        *int            `json:"max_tokens,omitempty"`
	ReasoningEffort  string          `json:"reasoning_effort,omitempty"`
	ToolChoice       json.RawMessage `json:"tool_choice,omitempty"` // 门面忽略（模型侧不透传强制选择）
	ParallelToolCall *bool           `json:"parallel_tool_calls,omitempty"`
}

// WireStreamOpts stream_options（include_usage）。门面恒按开启 usage 处理。
type WireStreamOpts struct {
	IncludeUsage bool `json:"include_usage,omitempty"`
}

// WireMessage 一条 wire 层消息。Content 用 RawMessage 承接
// 字符串与 parts 数组两种形态，由 history.go 的解析器分流。
type WireMessage struct {
	Role             string          `json:"role"` // system|user|assistant|tool
	Content          json.RawMessage `json:"content,omitempty"`
	ReasoningContent string          `json:"reasoning_content,omitempty"` // 思考回放，门面侧剥离
	ToolCalls        []WireToolCall  `json:"tool_calls,omitempty"`        // 仅 assistant
	ToolCallID       string          `json:"tool_call_id,omitempty"`      // 仅 tool
	Name             string          `json:"name,omitempty"`              // tool 消息的可选工具名
}

// WireContentPart user 消息多模态分片（text 或 image_url）。
type WireContentPart struct {
	Type     string        `json:"type"` // text|image_url
	Text     string        `json:"text,omitempty"`
	ImageURL *WireImageRef `json:"image_url,omitempty"`
}

// WireImageRef 图片分片的载荷（url 为 data URL 或 https 地址）。
type WireImageRef struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"` // high|low|auto，门面忽略
}

// WireToolCall assistant 消息携带的一次工具调用（完整形态，非流式分片）。
type WireToolCall struct {
	ID       string          `json:"id"`
	Type     string          `json:"type"` // function
	Function *WireToolCallFn `json:"function,omitempty"`
}

// WireToolCallFn 工具调用的函数名与参数（参数为 JSON 字符串）。
type WireToolCallFn struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// WireTool 请求携带的工具定义（客户端工具 schema 原样透传）。
type WireTool struct {
	Type     string         `json:"type"` // function
	Function *WireToolFnDef `json:"function,omitempty"`
}

// WireToolFnDef 工具定义的函数体（parameters 为完整 JSON Schema 对象）。
type WireToolFnDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// ---- 响应侧类型（SSE chunk）----

// ChunkResponse 一个流式分块。与标准 OpenAI 形态一致：
// finish_reason 未结束时为 null；usage 分块 choices 为空数组。
type ChunkResponse struct {
	ID      string        `json:"id"`
	Object  string        `json:"object"` // chat.completion.chunk
	Created int64         `json:"created"`
	Model   string        `json:"model"`
	Choices []ChunkChoice `json:"choices"`
	Usage   *WireUsage    `json:"usage,omitempty"`
}

// ChunkChoice 单个选择位（门面恒单选，index=0）。
type ChunkChoice struct {
	Index        int        `json:"index"`
	Delta        ChunkDelta `json:"delta"`
	FinishReason *string    `json:"finish_reason"` // null=未结束；stop|tool_calls|length
}

// ChunkDelta 分块载荷。字段均为可选：文本、思考流、工具调用分片。
type ChunkDelta struct {
	Content          string              `json:"content,omitempty"`
	ReasoningContent string              `json:"reasoning_content,omitempty"`
	ToolCalls        []WireToolCallChunk `json:"tool_calls,omitempty"`
}

// WireToolCallChunk 流式工具调用分片。一次性携带完整调用信息（id/name/
// arguments 全量）是合法形态——客户端按 index 累积分片，单块全量等价于
// 拼接结果（Cortico response-assembly.ts 的 pendingCalls 累积器语义）。
type WireToolCallChunk struct {
	Index    int              `json:"index"`
	ID       string           `json:"id,omitempty"`
	Type     string           `json:"type,omitempty"`
	Function *WireToolCallFn `json:"function,omitempty"`
}

// WireUsage token 用量（末 chunk 携带，Cortico 据此记账）。
type WireUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ---- SSE 分块构造器（门面循环 → HTTP 层共用）----

// FinishStop / FinishToolCalls / FinishLength 结束原因常量。
const (
	FinishStop      = "stop"
	FinishToolCalls = "tool_calls"
	FinishLength    = "length"
)

// NewResponseID 生成一次 HTTP 响应内复用的 chunk id（chatcmpl-yhx-<rand>）。
func NewResponseID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// 随机源失败时退化为时间戳，仅影响日志可读性不影响协议
		return "chatcmpl-yhx-" + hex.EncodeToString([]byte(time.Now().Format("150405.000000000")))
	}
	return "chatcmpl-yhx-" + hex.EncodeToString(b[:])
}

// newTextChunk 正文文本分块。
func newTextChunk(id, model, text string) ChunkResponse {
	return ChunkResponse{
		ID: id, Object: "chat.completion.chunk", Created: time.Now().Unix(), Model: model,
		Choices: []ChunkChoice{{Index: 0, Delta: ChunkDelta{Content: text}}},
	}
}

// newReasoningChunk 思考流分块（DeepSeek 风格 reasoning_content，
// V4.1 Flash 的推理过程经此透传，Coopanio 显示"在想"动画）。
func newReasoningChunk(id, model, reasoning string) ChunkResponse {
	return ChunkResponse{
		ID: id, Object: "chat.completion.chunk", Created: time.Now().Unix(), Model: model,
		Choices: []ChunkChoice{{Index: 0, Delta: ChunkDelta{ReasoningContent: reasoning}}},
	}
}

// newToolCallsChunk 工具调用分块：把一组已完整的客户端工具调用
// 以单块全量形态发出（索引按切片顺序 0..n-1）。
func newToolCallsChunk(id, model string, calls []WireToolCall) ChunkResponse {
	chunks := make([]WireToolCallChunk, 0, len(calls))
	for i, c := range calls {
		chunks = append(chunks, WireToolCallChunk{
			Index: i, ID: c.ID, Type: "function", Function: c.Function,
		})
	}
	return ChunkResponse{
		ID: id, Object: "chat.completion.chunk", Created: time.Now().Unix(), Model: model,
		Choices: []ChunkChoice{{Index: 0, Delta: ChunkDelta{ToolCalls: chunks}}},
	}
}

// newHeartbeatChunk 空 delta 心跳分块：服务端工具最长执行 45s，
// 期间无内容输出时定期发送，防止客户端按空闲断流。
// 空分块对标准 OpenAI 客户端是 no-op（Cortico 对空 delta 跳过）。
func newHeartbeatChunk(id, model string) ChunkResponse {
	return ChunkResponse{
		ID: id, Object: "chat.completion.chunk", Created: time.Now().Unix(), Model: model,
		Choices: []ChunkChoice{{Index: 0, Delta: ChunkDelta{}}},
	}
}

// newFinishChunk 结束分块：空 delta + finish_reason。
func newFinishChunk(id, model, reason string) ChunkResponse {
	return ChunkResponse{
		ID: id, Object: "chat.completion.chunk", Created: time.Now().Unix(), Model: model,
		Choices: []ChunkChoice{{Index: 0, Delta: ChunkDelta{}, FinishReason: &reason}},
	}
}

// newUsageChunk 用量分块：choices 为空数组、携带 usage（include_usage 语义的末块）。
func newUsageChunk(id, model string, u WireUsage) ChunkResponse {
	return ChunkResponse{
		ID: id, Object: "chat.completion.chunk", Created: time.Now().Unix(), Model: model,
		Choices: []ChunkChoice{}, Usage: &u,
	}
}

// ---- GET /models 响应 ----

// ModelsResponse 模型列表响应（Cortico 设置页下拉填充用）。
type ModelsResponse struct {
	Object string       `json:"object"` // list
	Data   []ModelEntry `json:"data"`
}

// ModelEntry 单个模型条目。id 为门面对外暴露的模型名（env 可配）。
type ModelEntry struct {
	ID      string `json:"id"`
	Object  string `json:"object"` // model
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

// newModelsResponse 构造只含单模型的列表响应。
func newModelsResponse(modelName string) ModelsResponse {
	return ModelsResponse{
		Object: "list",
		Data:   []ModelEntry{{ID: modelName, Object: "model", Created: time.Now().Unix(), OwnedBy: "yhagent"}},
	}
}
