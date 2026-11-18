// internal/openaicompat/handler.go
//
// HTTP/SSE 层：POST /v1/chat/completions 与 GET /v1/models。
// 鉴权与按 IP 限流由 internal/server 包的中间件包裹（与 /v1/chat 同一闸门），
// 本层只负责协议写出。流式写出全程持锁（心跳 goroutine 与回调并发），
// 上游错误发生在流开始前 → 标准 OpenAI JSON 错误响应（HTTP 状态码）；
// 流开始后 → 尽力写一个 error data chunk 再断流（客户端自带流中断重试）。
package openaicompat

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
)

// maxChatBodyBytes 请求体上限（base64 图片走请求体，需容纳多图）。
const maxChatBodyBytes = 8 << 20

// Handler 门面 HTTP 处理器（由 server 包挂路由）。
type Handler struct {
	facade *Facade
}

// NewHandler 创建门面 HTTP 处理器。
func NewHandler(f *Facade) *Handler {
	return &Handler{facade: f}
}

// ServeModels GET /v1/models：单模型列表（Cortico 设置页下拉填充）。
func (h *Handler) ServeModels(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, newModelsResponse(h.facade.cfg.ModelName))
}

// ServeChat POST /v1/chat/completions。支持 stream=true（Cortico 恒走流式）
// 与 stream=false（curl 调试友好）两种响应形态。
func (h *Handler) ServeChat(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxChatBodyBytes))
	if err != nil {
		writeWireError(w, http.StatusBadRequest, "请求体读取失败: "+err.Error())
		return
	}
	var req ChatRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeWireError(w, http.StatusBadRequest, "请求体不是合法 JSON: "+err.Error())
		return
	}
	if len(req.Messages) == 0 {
		writeWireError(w, http.StatusBadRequest, "messages 不能为空")
		return
	}

	// 流式：先写头（此后错误只能走 error chunk / 断流）
	if req.Stream {
		h.serveStreaming(w, r, req)
		return
	}
	h.serveNonStreaming(w, r, req)
}

// serveStreaming 流式响应：SSE chunk 序列 + [DONE]。
func (h *Handler) serveStreaming(w http.ResponseWriter, r *http.Request, req ChatRequest) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeWireError(w, http.StatusInternalServerError, "当前连接不支持流式响应")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	s := &sseWriter{w: w, flusher: flusher, id: NewResponseID(), model: h.facade.cfg.ModelName}
	// 客户端断连：取消回合（上游不再耗 token）
	ctx := r.Context()

	result, err := h.facade.RunTurn(ctx, req, TurnEvents{
		OnReasoning: func(text string) { s.writeChunk(newReasoningChunk(s.id, s.model, text)) },
		OnContent:   func(text string) { s.writeChunk(newTextChunk(s.id, s.model, text)) },
		OnHeartbeat: func() { s.writeChunk(newHeartbeatChunk(s.id, s.model)) },
	})
	if err != nil {
		// 流中错误：尽力写 OpenAI 风格 error chunk 后断流——Cortico 的
		// 解析器会按协议异常处理并触发其自带的重试机制
		s.writeErrorChunk(err)
		log.Printf("[openaicompat] 回合失败: %v", err)
		return
	}

	if len(result.ClientCalls) > 0 {
		s.writeChunk(newToolCallsChunk(s.id, s.model, result.ClientCalls))
	}
	s.writeChunk(newFinishChunk(s.id, s.model, result.FinishReason))
	s.writeChunk(newUsageChunk(s.id, s.model, result.Usage))
	s.writeDone()
}

// serveNonStreaming 非流式响应（调试友好）：聚合为单个 completion 对象。
func (h *Handler) serveNonStreaming(w http.ResponseWriter, r *http.Request, req ChatRequest) {
	result, err := h.facade.RunTurn(r.Context(), req, TurnEvents{
		OnReasoning: func(string) {},
		OnContent:   func(string) {},
		OnHeartbeat: func() {},
	})
	if err != nil {
		writeWireError(w, http.StatusBadGateway, err.Error())
		return
	}
	choices := []map[string]any{{
		"index":         0,
		"message":       nonStreamingMessage(result),
		"finish_reason": result.FinishReason,
	}}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":      NewResponseID(),
		"object":  "chat.completion",
		"model":   h.facade.cfg.ModelName,
		"choices": choices,
		"usage":   result.Usage,
	})
}

// nonStreamingMessage 构造非流式响应的 message 字段。
func nonStreamingMessage(result TurnResult) map[string]any {
	msg := map[string]any{"role": "assistant"}
	if result.Text != "" {
		msg["content"] = result.Text
	}
	if len(result.ClientCalls) > 0 {
		msg["tool_calls"] = result.ClientCalls
	}
	return msg
}

// sseWriter SSE 写出器：并发安全（心跳与回调交错），写失败置 stopped 后丢弃。
type sseWriter struct {
	mu      sync.Mutex
	w       io.Writer
	flusher http.Flusher
	id      string
	model   string
	stopped bool
}

// writeChunk 写一个 data 分块。客户端断连后的残余回调静默丢弃。
func (s *sseWriter) writeChunk(c ChunkResponse) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return
	}
	b, err := json.Marshal(c)
	if err != nil {
		return
	}
	if _, err := fmt.Fprintf(s.w, "data: %s\n\n", b); err != nil {
		s.stopped = true
		return
	}
	s.flusher.Flush()
}

// writeErrorChunk OpenAI 风格错误分块（尽力而为，客户端按协议异常处理）。
func (s *sseWriter) writeErrorChunk(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return
	}
	payload, _ := json.Marshal(map[string]any{"error": map[string]any{
		"message": err.Error(),
		"type":    "server_error",
	}})
	_, _ = fmt.Fprintf(s.w, "data: %s\n\n", payload)
	s.flusher.Flush()
}

// writeDone 终止标记 [DONE]。
func (s *sseWriter) writeDone() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return
	}
	_, _ = fmt.Fprint(s.w, "data: [DONE]\n\n")
	s.flusher.Flush()
}

// writeJSON 统一 JSON 响应辅助。
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeWireError OpenAI 风格错误响应（流开始前的 4xx/5xx）。
func writeWireError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]any{"message": msg, "type": "invalid_request_error"},
	})
}
