package ai

import (
	"context"
	"errors"
	"fmt"
	"io"

	"yhagent-server/internal/config"

	"github.com/volcengine/ark-runtime-go/arkruntime"
	"github.com/volcengine/ark-runtime-go/arkruntime/model/chat"
	"github.com/volcengine/ark-runtime-go/arkruntime/model/embedding"
)

type SeedClient struct {
	client *arkruntime.Client
}

func NewSeedClient() *SeedClient {
	return &SeedClient{
		// v0.6.0正确初始化方式，直接调用NewClientWithApiKey
		// 基础地址来自配置 ARK_BASE_URL（未配置时 config 已兜底为北京区默认地址）
		client: arkruntime.NewClientWithApiKey(
			config.AppConfig.DoubaoAPIKey,
			arkruntime.WithBaseUrl(config.AppConfig.DoubaoBaseURL),
		),
	}
}

// ChatStream 流式对话（兼容Wails前端打字机效果）
// 参数预留了Function Call工具位，后续对接三角洲API时不需要修改核心逻辑
func (c *SeedClient) ChatStream(
	ctx context.Context,
	history []chat.ChatCompletionRequestMessage,
	tools []chat.ChatCompletionTool,
	onChunk func(content string),
	onToolCall func(calls []chat.ChatCompletionMessageToolCall) error,
) error {
	temp := chat.NewOptFloat64(0.1)
	optTrue := chat.NewOptBool(true)
	req := chat.ChatCompletionRequest{
		Model:       config.AppConfig.DoubaoEndpoint,
		Messages:    history,
		Temperature: temp,
		Stream:      optTrue,
		Tools:       tools,
	}

	stream, err := c.client.CreateChatCompletionStream(ctx, &req)
	if err != nil {
		return fmt.Errorf("豆包API调用失败: %w", err)
	}
	defer stream.Close()
	// 流式工具调用累积器：key使用int32匹配chunk.Index类型，value使用chunk类型累积
	toolAccumulator := make(map[int32]*chat.ChatCompletionMessageToolCallChunk)
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
			return fmt.Errorf("流式接收中断: %w", err)
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
		// 3. 提前判断工具调用结束标记
		if finishReason, ok := choice.FinishReason.Get(); ok && finishReason == chat.FinishReasonToolCalls {
			completeCalls := buildCompleteToolCalls(toolAccumulator)
			toolAccumulator = make(map[int32]*chat.ChatCompletionMessageToolCallChunk)
			if err := onToolCall(completeCalls); err != nil {
				return fmt.Errorf("工具调用执行失败: %w", err)
			}
			return nil
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

// CreateEmbedding 向量生成
func (c *SeedClient) CreateEmbedding(ctx context.Context, texts embedding.EmbeddingInput) ([][]float32, error) {
	resp, err := c.client.CreateEmbeddings(ctx, &embedding.EmbeddingRequest{
		Model: "doubao-embedding-vision-251215",
		Input: texts,
	})
	if err != nil {
		return nil, err
	}
	embeddings := make([][]float32, 0, len(resp.Data))
	for _, d := range resp.Data {
		if vec, ok := d.Embedding.GetFloat64Array(); ok {
			f32Vec := make([]float32, len(vec))
			for i, v := range vec {
				f32Vec[i] = float32(v)
			}
			embeddings = append(embeddings, f32Vec)
		}
	}
	return embeddings, nil
}
