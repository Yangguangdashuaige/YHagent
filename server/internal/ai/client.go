package ai

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"yhagent-server/internal/config"

	"github.com/volcengine/ark-runtime-go/arkruntime"
	"github.com/volcengine/ark-runtime-go/arkruntime/model/chat"
)

type SeedClient struct {
	client *arkruntime.Client
	http   *http.Client // 多模态向量化端点用标准库 HTTP 调用
}

func NewSeedClient() *SeedClient {
	return &SeedClient{
		// v0.6.0正确初始化方式，直接调用NewClientWithApiKey
		// 基础地址来自配置 ARK_BASE_URL（未配置时 config 已兜底为北京区默认地址）
		client: arkruntime.NewClientWithApiKey(
			config.AppConfig.DoubaoAPIKey,
			arkruntime.WithBaseUrl(config.AppConfig.DoubaoBaseURL),
		),
		http: &http.Client{Timeout: 60 * time.Second},
	}
}

// ChatStream 流式对话（兼容Wails前端打字机效果）
// 参数预留了Function Call工具位，后续对接三角洲API时不需要修改核心逻辑。
// onUsage 在上游返回 usage 时回调一次（请求已开启 stream_options.include_usage，
// 末 chunk 携带 token 用量；上游未返回时不回调），可为 nil。
func (c *SeedClient) ChatStream(
	ctx context.Context,
	history []chat.ChatCompletionRequestMessage,
	tools []chat.ChatCompletionTool,
	onChunk func(content string),
	onToolCall func(calls []chat.ChatCompletionMessageToolCall) error,
	onUsage func(u chat.CompletionUsage),
) error {
	temp := chat.NewOptFloat64(0.1)
	optTrue := chat.NewOptBool(true)
	req := chat.ChatCompletionRequest{
		Model:       config.AppConfig.DoubaoEndpoint,
		Messages:    history,
		Temperature: temp,
		Stream:      optTrue,
		Tools:       tools,
		// 让末 chunk 携带 token 用量（trace 层统计每轮开销）
		StreamOptions: chat.NewOptChatCompletionStreamOptions(chat.ChatCompletionStreamOptions{
			IncludeUsage: optTrue,
		}),
	}

	stream, err := c.client.CreateChatCompletionStream(ctx, &req)
	if err != nil {
		return fmt.Errorf("豆包API调用失败: %w", err)
	}
	defer stream.Close()
	// 流式工具调用累积器：key使用int32匹配chunk.Index类型，value使用chunk类型累积
	toolAccumulator := make(map[int32]*chat.ChatCompletionMessageToolCallChunk)
	// 收到 finish_reason=tool_calls 后不再立即返回：继续读到 EOF，
	// 以便拿到末 chunk 的 usage（否则会提前退出丢掉统计），语义与原先一致
	finished := false
	for {
		recv, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			if len(toolAccumulator) > 0 {
				completeCalls := buildCompleteToolCalls(toolAccumulator)
				if err := onToolCall(completeCalls); err != nil {
					return fmt.Errorf("工具调用执行失败: %w", err)
				}
			}
			return nil
		}
		if err != nil {
			// finish_reason 之后流的任何异常都不影响已完整收到的工具调用，按原语义视为成功
			if finished {
				if len(toolAccumulator) > 0 {
					completeCalls := buildCompleteToolCalls(toolAccumulator)
					if err := onToolCall(completeCalls); err != nil {
						return fmt.Errorf("工具调用执行失败: %w", err)
					}
				}
				return nil
			}
			return fmt.Errorf("流式接收中断: %w", err)
		}
		// usage 只出现在末 chunk（choices 为空），先于下方的 choices 判空提取
		if onUsage != nil {
			if u, ok := recv.Usage.Get(); ok {
				onUsage(u)
			}
		}
		if len(recv.Choices) == 0 {
			continue
		}
		choice := recv.Choices[0]
		// 1. 处理普通文本分片，安全访问OptString值
		if content, ok := choice.Delta.Content.Get(); ok && content != "" {
			onChunk(content)
		}
		// 2. 逐段累积工具调用分片
		if len(choice.Delta.ToolCalls) > 0 {
			for _, toolCall := range choice.Delta.ToolCalls {
				if _, exists := toolAccumulator[toolCall.Index]; !exists {
					toolAccumulator[toolCall.Index] = &chat.ChatCompletionMessageToolCallChunk{
						Index: toolCall.Index,
					}
				}
				// 累积工具ID
				if id, ok := toolCall.ID.Get(); ok && id != "" {
					toolAccumulator[toolCall.Index].ID.SetTo(toolAccumulator[toolCall.Index].ID.Value + id)
				}
				// 累积工具类型
				if typ, ok := toolCall.Type.Get(); ok && typ != "" {
					toolAccumulator[toolCall.Index].Type.SetTo(typ)
				}
				// 累积函数名和参数
				if fn, ok := toolCall.Function.Get(); ok {
					accFn := toolAccumulator[toolCall.Index].Function
					if !accFn.IsSet() {
						accFn.SetTo(chat.ChatCompletionMessageToolCallChunkFunction{})
					}
					accVal := accFn.Value
					if name, ok := fn.Name.Get(); ok && name != "" {
						accVal.Name.SetTo(accVal.Name.Value + name)
					}
					if args, ok := fn.Arguments.Get(); ok && args != "" {
						accVal.Arguments.SetTo(accVal.Arguments.Value + args)
					}
					accFn.SetTo(accVal)
					toolAccumulator[toolCall.Index].Function = accFn
				}
			}
		}
		// 3. 工具调用结束标记：标记后继续读到 EOF/末 chunk（拿 usage），再统一交付工具调用
		if finishReason, ok := choice.FinishReason.Get(); ok && finishReason == chat.FinishReasonToolCalls {
			finished = true
			continue
		}
	}
}

// 辅助方法：把累积器里的分片按索引顺序整理成完整的工具调用列表
func buildCompleteToolCalls(acc map[int32]*chat.ChatCompletionMessageToolCallChunk) []chat.ChatCompletionMessageToolCall {
	calls := make([]chat.ChatCompletionMessageToolCall, 0, len(acc))
	for i := int32(0); i < int32(len(acc)); i++ {
		chunk, ok := acc[i]
		if !ok {
			continue
		}
		call := chat.ChatCompletionMessageToolCall{
			ID:   chunk.ID.Value,
			Type: chat.ToolTypeFunction,
		}
		if fn, ok := chunk.Function.Get(); ok {
			call.Function = chat.ChatCompletionMessageToolCallFunction{
				Name:      fn.Name.Value,
				Arguments: fn.Arguments.Value,
			}
		}
		calls = append(calls, call)
	}
	return calls
}

// CreateEmbedding 批量生成文本向量，返回与入参顺序一致的向量列表。
// 固定走 doubao-embedding-vision 接入点的多模态向量化端点（见 embedding_multimodal.go），
// 模型/接入点来自配置 ARK_EMBEDDING_MODEL。
func (c *SeedClient) CreateEmbedding(ctx context.Context, texts []string) ([][]float32, error) {
	return createMultimodalEmbeddings(ctx, c.http, texts)
}

// SimpleChat 无工具的单轮非流式（内部仍用流式接口收全文）对话，
// 用于"联网资料整理成 Markdown 入库"这类后台场景：一条 system + 一条 user，返回完整文本。
func (c *SeedClient) SimpleChat(ctx context.Context, system, user string) (string, error) {
	return c.SimpleChatStream(ctx, system, user, nil)
}

// SimpleChatStream 无工具的一次性对话，并把流式输出通过 onChunk 实时回调。
// onChunk 为 nil 时与 SimpleChat 等价（入库整理时用它向前端推送撰写进度）。
func (c *SeedClient) SimpleChatStream(ctx context.Context, system, user string, onChunk func(content string)) (string, error) {
	msgs := []chat.ChatCompletionRequestMessage{
		newSystemMessage(system),
		NewUserMessage(user),
	}
	var sb strings.Builder
	err := c.ChatStream(ctx, msgs, nil, func(chunk string) {
		sb.WriteString(chunk)
		if onChunk != nil {
			onChunk(chunk)
		}
	}, func(_ []chat.ChatCompletionMessageToolCall) error { return nil }, nil)
	if err != nil {
		return "", err
	}
	return sb.String(), nil
}
