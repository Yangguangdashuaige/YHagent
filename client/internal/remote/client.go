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
	"net/url"
	"strings"
	"time"
)

const (
	// sseScanLimit 单个 SSE 事件行的上限（正常文本片段很小，1MB 足够兜底）
	sseScanLimit = 1 << 20
	httpTimeout  = 120 * time.Second // 整轮对话（含工具轮询）上限

	// resumeMaxRetry 断线续传最大重试次数：拿到 turn_id 后，网络/读流失败可凭
	// 服务端 WAL 回放补发（不重发上游、不重复耗 token）；耗尽则返回最后一次错误。
	resumeMaxRetry = 5
)

// resumeBackoff 续传重试退避序列（索引对应第几次重试，越界取最后一项）
var resumeBackoff = []time.Duration{
	500 * time.Millisecond,
	1 * time.Second,
	2 * time.Second,
	3 * time.Second,
	5 * time.Second,
}

// InterruptedError 表示本轮回答被中断（服务端 WAL 终态为 interrupted，
// 通常因服务重启 / 上游崩溃，豆包无状态无法接字续生成）。上层据此渲染
// 「重新生成」而非当成普通错误——已生成的半截回答完整保留。
type InterruptedError struct {
	Reason string // 中断原因（服务端给出）
	TurnID string // 本轮 turn_id（= trace_id），供观测关联
}

func (e *InterruptedError) Error() string {
	if e != nil && e.Reason != "" {
		return "回答中断：" + e.Reason
	}
	return "回答中断"
}

// sleepCtx 可被 ctx 取消的 sleep；ctx 提前取消返回 false。
func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}

// Client 远端对话服务客户端，可安全并发使用（每个会话的提问由 UI 层串行化）
type Client struct {
	baseURL string
	token   string
	http    *http.Client
	// onClarify ask_user 澄清事件回调（app 层注入：透传 Wails 事件给前端渲染）；
	// nil 时 clarify 事件被忽略（回答流不受影响）
	onClarify func(payload clarifyPayload)
}

// SetOnClarify 注册澄清事件回调（须在首次 Chat 前设置）
func (c *Client) SetOnClarify(fn func(question string, options []ClarifyOpt)) {
	if fn == nil {
		c.onClarify = nil
		return
	}
	c.onClarify = func(p clarifyPayload) { fn(p.Text, p.Options) }
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

// ChatWithSessionImages 与 ChatWithSession 相同，额外携带图片 data URL（截图识别）。
// 内置断线续传：trace 事件给出 turn_id 后，网络/读流失败会退避并用 resume_turn
// 回放服务端 WAL 补发未收分片（不重发上游、不重复耗 token）；回合被判中断时
// 返回 *InterruptedError，由上层渲染「重新生成」。
func (c *Client) ChatWithSessionImages(ctx context.Context, sessionID, userText string, images []string, onChunk func(content string)) error {
	if strings.TrimSpace(userText) == "" && len(images) == 0 {
		return errors.New("提问内容不能为空")
	}
	if sessionID == "" {
		sessionID = "default"
	}

	var (
		turnID  string // 本轮 turn_id（trace 事件给出），续传的凭据
		got     int    // 已收到的内容事件数（chunk+thought），跨重试累计
		lastErr error
	)
	for attempt := 0; attempt <= resumeMaxRetry; attempt++ {
		// 首次正常发问；之后凭 turn_id + 已收条数走 WAL 回放
		var rt string
		var ra int
		if attempt > 0 {
			if turnID == "" {
				break // 没拿到 turn_id 就没有续传凭据，直接报错
			}
			rt = turnID
			ra = got
		}
		err := c.streamOnce(ctx, sessionID, userText, images, rt, ra, onChunk,
			func(id string) { if id != "" { turnID = id } },
			func() { got++ })
		if err == nil {
			return nil
		}
		// 回合被判中断：已生成内容保留，上抛让上层渲染「重新生成」，不重试
		var ie *InterruptedError
		if errors.As(err, &ie) {
			return err
		}
		// 用户取消（窗口关闭/主动停止）：不重试
		if errors.Is(err, context.Canceled) {
			return err
		}
		lastErr = err
		if turnID == "" || attempt >= resumeMaxRetry {
			break
		}
		if !sleepCtx(ctx, resumeBackoff[min(attempt, len(resumeBackoff)-1)]) {
			return lastErr // 退避期间 ctx 取消
		}
	}
	if lastErr == nil {
		lastErr = errors.New("远端回答中断，请稍后再试")
	}
	return lastErr
}

// streamOnce 发起一次 /v1/chat 请求并消费 SSE 流。
// resumeTurn 非空时 message/images 传空，服务端据此走 WAL 纯回放分支。
// onTrace 回调 turn_id；onContent 每收到一个内容事件（chunk/thought）+1，
// 用于续传时上报已收条数（与服务端 WAL 内容计数对齐，至少一次语义）。
func (c *Client) streamOnce(ctx context.Context, sessionID, userText string, images []string,
	resumeTurn string, resumeAfter int, onChunk func(string),
	onTrace func(string), onContent func()) error {

	message := userText
	if resumeTurn != "" {
		message = ""
		images = nil
	}
	payload, _ := json.Marshal(map[string]any{
		"session_id":   sessionID,
		"message":      message,
		"images":       images,
		"resume_turn":  resumeTurn,
		"resume_after": resumeAfter,
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

	return parseSSE(resp.Body, onChunk, c.onClarify, onTrace, onContent)
}

// ClarifyOpt ask_user 候选选项（与前端 clarify 卡片对应）
type ClarifyOpt struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// clarifyPayload clarify 事件载荷（app 层透传给前端渲染）
type clarifyPayload struct {
	Text    string       `json:"text"`
	Options []ClarifyOpt `json:"options"`
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

// SessionSummary 服务端会话摘要（多会话列表项）
type SessionSummary struct {
	SessionID string `json:"session_id"`
	Title     string `json:"title"`
	UpdatedAt int64  `json:"updated_at"`
	MsgCount  int    `json:"msg_count"`
}

// ListSessions 批量拉取会话摘要：客户端上报本地记录的 sid 集合，
// 服务端归属过滤后返回标题/最后活跃/消息数。网络失败返回错误由 UI 静默降级。
func (c *Client) ListSessions(ctx context.Context, sids []string) ([]SessionSummary, error) {
	if len(sids) == 0 {
		return nil, nil
	}
	u := c.baseURL + "/v1/sessions?ids=" + url.QueryEscape(strings.Join(sids, ","))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("构造会话列表请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, errors.New("无法连接远端AI服务，请检查网络后稍后再试")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("会话列表获取失败（HTTP %d）", resp.StatusCode)
	}
	var env struct {
		Sessions []SessionSummary `json:"sessions"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&env); err != nil {
		return nil, fmt.Errorf("会话列表解析失败: %w", err)
	}
	return env.Sessions, nil
}

// DeleteSession 删除单个会话（服务端清上下文+落库+解绑）。删除当前会话后
// 由前端自动开新对话。
func (c *Client) DeleteSession(sessionID string) error {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodDelete,
		c.baseURL+"/v1/session?session_id="+url.QueryEscape(sessionID), nil)
	if err != nil {
		return fmt.Errorf("构造删除会话请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.http.Do(req)
	if err != nil {
		return errors.New("无法连接远端AI服务，请稍后再试")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("删除会话失败（HTTP %d）", resp.StatusCode)
	}
	return nil
}

// HistoryMsg 会话历史的客户端可见消息（user / assistant 文本）
type HistoryMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// SessionHistory 拉取指定会话的可见历史（切换会话/重开窗口时恢复对话内容）。
// 无历史返回空列表；失败返回错误由 UI 静默降级。
func (c *Client) SessionHistory(ctx context.Context, sessionID string) ([]HistoryMsg, error) {
	u := c.baseURL + "/v1/session/history?session_id=" + url.QueryEscape(sessionID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("构造历史请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, errors.New("无法连接远端AI服务，请稍后再试")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("历史获取失败（HTTP %d）", resp.StatusCode)
	}
	var env struct {
		Messages []HistoryMsg `json:"messages"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&env); err != nil {
		return nil, fmt.Errorf("历史解析失败: %w", err)
	}
	return env.Messages, nil
}

// SubmitFeedback 提交一条回答反馈（rating: 1=赞 / -1=踩）。
// 桌面端拿不到本轮 trace_id（SSE 解析只关心回答流），传空串——
// 服务端按 session_id + 问答文本落库，点赞问答同样进 few-shot 案例库。
func (c *Client) SubmitFeedback(sessionID string, rating int, question, answer string) error {
	payload, _ := json.Marshal(map[string]any{
		"session_id": sessionID,
		"trace_id":   "",
		"rating":     rating,
		"question":   question,
		"answer":     answer,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/feedback", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("构造反馈请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.http.Do(req)
	if err != nil {
		return errors.New("无法连接远端AI服务，反馈暂时无法提交")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("反馈提交失败（HTTP %d）", resp.StatusCode)
	}
	return nil
}

// parseSSE 按 W3C SSE 基本格式解析事件流：
//
//	event: <名字>
//	data: <JSON字符串>
//	（空行派发）
//
// 以 ": " 开头的是注释心跳，直接忽略。
// onTrace 回调 trace 事件的 turn_id（断线续传凭据）；onContent 每收到一个
// 内容事件（chunk/thought）+1，用于续传时上报已收条数；
// interrupted 事件返回 *InterruptedError（上层据此渲染「重新生成」）。
func parseSSE(r io.Reader, onChunk func(string), onClarify func(clarifyPayload),
	onTrace func(string), onContent func()) error {
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
			// 每个 chunk 事件都计为一条内容（含空串），与服务端 WAL 内容计数对齐
			if onContent != nil {
				onContent()
			}
			if text != "" && onChunk != nil {
				onChunk(text)
			}
		case "thought":
			// 思考模型推理流：桌面端不渲染，仅计数用于续传对齐。
			// data 为空时跳过：空行也会触发一次 dispatch，必须防重复计数，
			// 否则 resume_after 会与服务端 WAL 内容计数错位。
			if data == "" {
				return nil
			}
			if onContent != nil {
				onContent()
			}
		case "trace":
			var p struct {
				TraceID string `json:"trace_id"`
			}
			if err := json.Unmarshal([]byte(data), &p); err == nil && p.TraceID != "" && onTrace != nil {
				onTrace(p.TraceID)
			}
		case "interrupted":
			// 续传回放结束但回合被中断：上抛 *InterruptedError，不算普通错误
			var p struct {
				Reason string `json:"reason"`
				TurnID string `json:"turn_id"`
			}
			_ = json.Unmarshal([]byte(data), &p)
			return &InterruptedError{Reason: p.Reason, TurnID: p.TurnID}
		case "clarify":
			// ask_user 候选选项：解析后交给回调（未注册则忽略，不影响回答流）
			var p clarifyPayload
			if err := json.Unmarshal([]byte(data), &p); err == nil && onClarify != nil && len(p.Options) > 0 {
				onClarify(p)
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
