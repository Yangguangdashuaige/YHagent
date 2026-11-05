// internal/ai/history.go
//
// 会话历史的客户端视图：网页/桌面端刷新后恢复对话内容。
//
// 设计取舍：走 MarshalJSON → map 解析（与 session_store 落盘同一路线），
// 不直接拆 SDK 的 Sum 类型分支——两类消息形态（纯文本 / 多模态 parts）
// 在 JSON 层天然摊平，代码量少一个量级；图片分片折叠为「[图片]」占位，
// 避免 base64 数据打爆历史接口响应。
package ai

import (
	"encoding/json"
	"log"
	"strings"

	"github.com/volcengine/ark-runtime-go/arkruntime/model/chat"
)

// HistoryMessage 客户端可见的历史消息（只有 user / assistant 两种角色）
type HistoryMessage struct {
	Role    string `json:"role"`    // user / assistant
	Content string `json:"content"` // 文本内容；图片折叠为 [图片] 占位
}

// SessionHistory 提取会话的可见历史。会话不存在时返回空列表；
// 配置了 SESSION_DB 时内存未命中会自动从磁盘恢复（与聊天路径一致）。
func (a *Agent) SessionHistory(uid int64, sessionID string) []HistoryMessage {
	sess := a.sessions.GetOrCreate(sessionID, a.systemPromptFor(uid))
	msgs := sess.historyAfterSystem()
	out := make([]HistoryMessage, 0, len(msgs))
	for _, m := range msgs {
		if hm, ok := clientVisible(m); ok {
			out = append(out, hm)
		}
	}
	return out
}

// SessionSummaries 批量返回会话摘要（多会话列表）。归属过滤在 server 层完成，
// 这里只做读侧：落库后端未启用或 sid 无记录时返回空列表（客户端对本地记录
// 的 sid 自行补占位项）。
func (a *Agent) SessionSummaries(sids []string) []SessionSummary {
	su, ok := a.store.(interface {
		Summaries(sids []string) ([]SessionSummary, error)
	})
	if !ok {
		return nil
	}
	out, err := su.Summaries(sids)
	if err != nil {
		log.Printf("[agent] 会话摘要读取失败: %v", err)
		return nil
	}
	return out
}

// clientVisible 把一条 SDK 消息转换为客户端可见形态。
// user 消息始终入列（空文本纯图消息显示 [图片]）；assistant 消息只在有正文时
// 入列（纯 tool_calls 的中间消息跳过）；system / tool 消息不入列。
func clientVisible(m chat.ChatCompletionRequestMessage) (HistoryMessage, bool) {
	b, err := m.MarshalJSON()
	if err != nil {
		return HistoryMessage{}, false
	}
	var obj map[string]any
	if err := json.Unmarshal(b, &obj); err != nil {
		return HistoryMessage{}, false
	}
	switch role, _ := obj["role"].(string); role {
	case "user":
		return HistoryMessage{Role: "user", Content: contentText(obj["content"])}, true
	case "assistant":
		text := contentText(obj["content"])
		if strings.TrimSpace(text) == "" {
			return HistoryMessage{}, false
		}
		return HistoryMessage{Role: "assistant", Content: text}, true
	default:
		return HistoryMessage{}, false
	}
}

// contentText 从消息 content 字段提取文本：
// 字符串原样返回；数组（多模态）取 text 分片拼接、图片分片折叠为 [图片]。
func contentText(v any) string {
	switch c := v.(type) {
	case string:
		return c
	case []any:
		var sb strings.Builder
		for _, p := range c {
			pm, ok := p.(map[string]any)
			if !ok {
				continue
			}
			switch t, _ := pm["type"].(string); t {
			case "text":
				if s, ok := pm["text"].(string); ok {
					sb.WriteString(s)
				}
			case "image_url":
				sb.WriteString("[图片]")
			}
		}
		return sb.String()
	default:
		return ""
	}
}
