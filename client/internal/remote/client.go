// internal/remote/client.go
//
// 远端 Agent 服务的瘦客户端（yhagent-client 模块内唯一的对话能力实现）。
// Agent 多轮循环 / 工具执行 / RAG / 缓存全部在服务器发生，
// 本机只负责把提问 POST 出去，然后把 SSE 流里的回答片段回调给 UI 层（打字机效果不变）。
//
// 仅依赖标准库，零 CGO；app.go 绑定层与前端事件协议不感知远端细节。
package remote

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

const (
	// sseScanLimit 单个 SSE 事件行的上限（正常文本片段很小，1MB 足够兜底）
	sseScanLimit = 1 << 20
	httpTimeout  = 120 * time.Second // 整轮对话（含工具轮询）上限
)

// Client 远端对话服务客户端，可安全并发使用（每个会话的提问由 UI 层串行化）
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

// New 创建远端客户端：baseURL 如 http://your-server:8080，token 与服务端 SERVER_TOKEN 一致
func New(baseURL, token string) (*Client, error) {
	if strings.TrimSpace(baseURL) == "" {
		return nil, errors.New("远端服务地址 REMOTE_BASE_URL 未配置")
	}
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("远端访问令牌 REMOTE_TOKEN 未配置")
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		http:    &http.Client{Timeout: httpTimeout},
	}, nil
}

// ChatWithSession 实现对话服务接口：发起远端流式对话并把片段通过 onChunk 推给上层
func (c *Client) ChatWithSession(ctx context.Context, sessionID, userText string, onChunk func(content string)) error {
	return c.ChatWithSessionImages(ctx, sessionID, userText, nil, onChunk)
}

// ChatWithSessionImages 与 ChatWithSession 相同，额外携带图片 data URL（截图识别）
func (c *Client) ChatWithSessionImages(ctx context.Context, sessionID, userText string, images []string, onChunk func(content string)) error {
	if strings.TrimSpace(userText) == "" && len(images) == 0 {
		return errors.New("提问内容不能为空")
	}
	if sessionID == "" {
		sessionID = "default"
	}

	payload, _ := json.Marshal(map[string]any{
		"session_id": sessionID,
		"message":    userText,
		"images":     images,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/chat", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("构造远端请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.http.Do(req)
	if err != nil {
		// 网络层错误转成面向用户的友好提示
		if errors.Is(err, context.DeadlineExceeded) || isTimeout(err) {
			return errors.New("远端AI服务响应超时，请检查网络后稍后再试")
		}
		return errors.New("无法连接远端AI服务，请检查网络或服务地址后稍后再试")
	}
	defer resp.Body.Close()

	// 非 200（401 鉴权失败、400 参数错误、5xx 服务异常）读 JSON 错误信息
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		var env struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(body, &env) == nil && env.Error != "" {
			if resp.StatusCode == http.StatusUnauthorized {
				return errors.New("远端服务鉴权失败，请联系管理员更新访问令牌")
			}
			return errors.New(env.Error)
		}
		return fmt.Errorf("远端服务异常（HTTP %d）", resp.StatusCode)
	}

	return parseSSE(resp.Body, onChunk)
}

// NewSession 向服务端申请签发会话ID（sess_ 前缀 + 128bit 随机）。
// 服务端对会话ID有格式白名单（签发格式/历史UUIDv4），新客户端统一走签发，
// 避免客户端自造 ID 撞号读到他人会话上下文。失败由上层决定是否本地兜底。
func (c *Client) NewSession(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/session/new", nil)
	if err != nil {
		return "", fmt.Errorf("构造会话签发请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", errors.New("无法连接远端AI服务，请检查网络或服务地址后稍后再试")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("会话签发失败（HTTP %d）", resp.StatusCode)
	}

	var env struct {
		SessionID string `json:"session_id"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<10)).Decode(&env); err != nil {
		return "", fmt.Errorf("会话签发响应解析失败: %w", err)
	}
	if env.SessionID == "" {
		return "", errors.New("会话签发响应缺少 session_id")
	}
	return env.SessionID, nil
}

// Reset 实现 ai.ChatService：通知服务器清空指定会话上下文（失败只记日志，不打断 UI）
func (c *Client) Reset(sessionID string) {
	if sessionID == "" {
		sessionID = "default"
	}
	payload, _ := json.Marshal(map[string]string{"session_id": sessionID})

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, c.baseURL+"/v1/session/reset", bytes.NewReader(payload))
	if err != nil {
		log.Printf("[remote] 构造清空会话请求失败: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.http.Do(req)
	if err != nil {
		log.Printf("[remote] 清空会话请求失败: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("[remote] 清空会话失败: HTTP %d", resp.StatusCode)
	}
}

// parseSSE 按 W3C SSE 基本格式解析事件流：
//
//	event: <名字>
//	data: <JSON字符串>
//	（空行派发）
//
// 以 ": " 开头的是注释心跳，直接忽略。
func parseSSE(r io.Reader, onChunk func(string)) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64<<10), sseScanLimit)

	var event string
	dispatch := func(data string) error {
		switch event {
		case "chunk":
			var text string
			if err := json.Unmarshal([]byte(data), &text); err != nil {
				return nil // 单片坏数据不影响整体流
			}
			if text != "" && onChunk != nil {
				onChunk(text)
			}
		case "done":
			return errStreamDone
		case "error":
			var msg string
			if err := json.Unmarshal([]byte(data), &msg); err == nil && msg != "" {
				return errors.New(msg)
			}
			return errors.New("AI服务暂时不可用，请稍后再试")
		}
		return nil
	}

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if event != "" {
				if err := dispatch(""); err != nil {
					if errors.Is(err, errStreamDone) {
						return nil
					}
					return err
				}
			}
			event = ""
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue // 心跳注释
		}
		field, value, found := strings.Cut(line, ":")
		if !found {
			field = line
			value = ""
		}
		value = strings.TrimPrefix(value, " ")
		switch field {
		case "event":
			event = value
		case "data":
			if event == "" {
				continue
			}
			if err := dispatch(value); err != nil {
				if errors.Is(err, errStreamDone) {
					return nil
				}
				return err
			}
		}
	}
	if err := scanner.Err(); err != nil {
		if errors.Is(err, context.Canceled) {
			return errors.New("请求已取消")
		}
		return errors.New("远端回答中断，请稍后再试")
	}
	return errors.New("远端回答意外结束，请稍后再试")
}

var errStreamDone = errors.New("sse stream done")

func isTimeout(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "timeout") || strings.Contains(msg, "deadline") || strings.Contains(msg, "超时")
}
