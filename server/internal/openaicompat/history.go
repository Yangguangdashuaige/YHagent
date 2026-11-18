// internal/openaicompat/history.go
//
// 请求 messages（OpenAI wire 形态）→ 方舟 SDK 消息的转换。
// Coopanion 每次请求重发全量历史（无状态语义），门面据此重建模型可见上下文：
//
//   - system   抽出为 SystemText（Cortico instructions：宠物人格+环境提示），
//              由 prompt.go 融合后作为首条 system 消息下发，不原样透传；
//   - user     字符串或 text|image_url parts（Cortico 已把工具结果中的图片
//              预移到相邻 user 消息，见其 chat.ts imagesAfterResults），
//              复用 ai.NewUserMessageWithImages 构造多模态消息；
//   - assistant content + tool_calls 合并进同一信封（Cortico 把相邻正文与
//              function_call 合并）；reasoning_content 剥离——方舟思考模型
//              不接受跨轮回放推理（与现有 Agent 循环行为一致）；
//   - tool     content + tool_call_id 原样转结果消息（content 理论上恒为
//              字符串，数组形态防御性提取文本）。
package openaicompat

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/volcengine/ark-runtime-go/arkruntime/model/chat"

	"yhagent-server/internal/ai"
)

// maxImagesPerUserMessage 单条 user 消息允许携带的图片数上限（防滥用；
// Coopanion 正常路径 1~2 张：截图提问 / pet_see_self 附图）。
const maxImagesPerUserMessage = 8

// ParsedHistory 一次请求的历史解析结果。
type ParsedHistory struct {
	// SystemText 所有 system 消息合并的提示词（多条以空行分隔，防御性支持）。
	SystemText string
	// Messages 按原顺序转换的非 system 消息（user/assistant/tool）。
	Messages []chat.ChatCompletionRequestMessage
}

// ParseHistory 把 wire 消息序列解析为「提示词 + 方舟消息列表」。
// 遇到无法理解的消息形态返回错误（HTTP 层转为 400），不做静默丢弃——
// 历史残缺会让模型的工具调用配对错乱，宁可显式失败。
func ParseHistory(msgs []WireMessage) (ParsedHistory, error) {
	out := ParsedHistory{Messages: make([]chat.ChatCompletionRequestMessage, 0, len(msgs))}
	var sysParts []string
	for i, m := range msgs {
		switch m.Role {
		case "system", "developer":
			// developer 归一为 system（Cortico native-input 的映射惯例）
			if t := contentString(m.Content); t != "" {
				sysParts = append(sysParts, t)
			}
		case "user":
			msg, err := userMessage(m)
			if err != nil {
				return out, fmt.Errorf("第%d条消息（user）解析失败: %w", i+1, err)
			}
			out.Messages = append(out.Messages, msg)
		case "assistant":
			out.Messages = append(out.Messages, assistantMessage(m))
		case "tool":
			id := strings.TrimSpace(m.ToolCallID)
			if id == "" {
				return out, fmt.Errorf("第%d条消息（tool）缺少 tool_call_id", i+1)
			}
			out.Messages = append(out.Messages, ai.NewToolResultMessage(id, m.Name, contentString(m.Content)))
		default:
			return out, fmt.Errorf("第%d条消息角色非法: %q", i+1, m.Role)
		}
	}
	out.SystemText = strings.Join(sysParts, "\n\n")
	return out, nil
}

// userMessage 构造 user 消息：content 为字符串时走纯文本；
// 为数组时解析 text|image_url parts 并复用现有多模态构造。
func userMessage(m WireMessage) (chat.ChatCompletionRequestMessage, error) {
	if len(m.Content) == 0 || m.Content[0] != '[' {
		return ai.NewUserMessage(contentString(m.Content)), nil
	}
	var parts []WireContentPart
	if err := json.Unmarshal(m.Content, &parts); err != nil {
		return chat.ChatCompletionRequestMessage{}, fmt.Errorf("content 不是合法的字符串或分片数组: %w", err)
	}
	var text strings.Builder
	images := make([]ai.UserImage, 0, len(parts))
	for _, p := range parts {
		switch p.Type {
		case "text":
			text.WriteString(p.Text)
		case "image_url":
			if p.ImageURL != nil && strings.TrimSpace(p.ImageURL.URL) != "" {
				images = append(images, ai.UserImage{DataURL: p.ImageURL.URL})
			}
		default:
			// refusal 等其他分片类型：忽略，保历史其余部分可用
		}
	}
	if len(images) > maxImagesPerUserMessage {
		return chat.ChatCompletionRequestMessage{}, fmt.Errorf("单条消息图片数 %d 超上限 %d", len(images), maxImagesPerUserMessage)
	}
	return ai.NewUserMessageWithImages(text.String(), images), nil
}

// assistantMessage 构造 assistant 消息：正文与工具调用合并进同一信封，
// reasoning_content 剥离（跨轮回放思考对方舟无意义）。
func assistantMessage(m WireMessage) chat.ChatCompletionRequestMessage {
	calls := make([]chat.ChatCompletionMessageToolCall, 0, len(m.ToolCalls))
	for _, c := range m.ToolCalls {
		if c.Function == nil {
			continue // 空壳调用防御：跳过，避免下游配对错乱
		}
		calls = append(calls, chat.ChatCompletionMessageToolCall{
			ID:   c.ID,
			Type: chat.ToolTypeFunction,
			Function: chat.ChatCompletionMessageToolCallFunction{
				Name:      c.Function.Name,
				Arguments: c.Function.Arguments,
			},
		})
	}
	content := contentString(m.Content)
	if len(calls) == 0 {
		return ai.NewAssistantTextMessage(content)
	}
	return ai.NewAssistantMessageWithToolCalls(content, calls)
}

// contentString 从 RawMessage 形态的 content 提取文本：
// 字符串原样返回；数组取 text 分片拼接（图片折叠为占位保持可读性，
// 图片本体已由 userMessage 分流处理，这里兜底工具消息的数组形态）。
func contentString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	switch raw[0] {
	case '"':
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return ""
		}
		return s
	case '[':
		var parts []WireContentPart
		if err := json.Unmarshal(raw, &parts); err != nil {
			return ""
		}
		var sb strings.Builder
		for _, p := range parts {
			switch p.Type {
			case "text":
				sb.WriteString(p.Text)
			case "image_url":
				sb.WriteString("[图片]")
			}
		}
		return sb.String()
	default:
		return ""
	}
}
