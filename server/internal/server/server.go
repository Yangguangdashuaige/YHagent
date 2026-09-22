// internal/server/server.go
//
// 远端 Agent 服务（瘦客户端架构的服务端）：
//   - 豆包 Function Call 多轮循环、三角洲工具、共享缓存、singleflight 全部在本进程执行；
//   - 客户端只是“聊天界面 + SSE 流播放器”，不持有任何 API Key；
//   - 仅用 Go 标准库 net/http + SSE，零 CGO、可独立编译成单个服务端二进制部署到云服务器。
//
// SSE 事件协议（与 internal/ai/remote 客户端严格对应）：
//
//	event: chunk
//	data: "一段回答文本（JSON编码字符串）"
//
//	event: done
//	data: ""
//
//	event: error
//	data: "面向用户的中文错误信息（JSON编码字符串）"
package server

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"yhagent-server/internal/ai"
)

const (
	maxChatBodyBytes = 8 << 10 // 单条提问上限 8KB
	sseHeartbeat     = 25 * time.Second
)

// Config 服务端配置
type Config struct {
	Addr  string         // 监听地址，如 ":8080"
	Token string         // 客户端访问令牌（Bearer），必填，拒绝裸奔
	Chat  ai.ChatService // Agent 核心（工具已注册、缓存已挂载）
}

// Server HTTP 服务
type Server struct {
	cfg  Config
	http *http.Server
}

// New 创建服务端实例，token 为空时返回错误（避免把自己的服务器变成开放代理）
func New(cfg Config) (*Server, error) {
	if strings.TrimSpace(cfg.Token) == "" {
		return nil, errors.New("服务端访问令牌 SERVER_TOKEN 未配置，拒绝启动")
	}
	if cfg.Chat == nil {
		return nil, errors.New("ChatService 未注入")
	}
	if cfg.Addr == "" {
		cfg.Addr = ":8080"
	}
	s := &Server{cfg: cfg}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("POST /v1/chat", s.withAuth(s.handleChat))
	mux.HandleFunc("POST /v1/session/reset", s.withAuth(s.handleReset))

	s.http = &http.Server{
		Addr:              cfg.Addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		// 流式响应不能设置 WriteTimeout / IdleTimeout 过短，否则长回答会被掐断
		IdleTimeout: 120 * time.Second,
	}
	return s, nil
}

// Run 启动 HTTP 服务（阻塞）
func (s *Server) Run() error {
	log.Printf("[server] 远端Agent服务监听 %s", s.cfg.Addr)
	return s.http.ListenAndServe()
}

// Shutdown 优雅关闭
func (s *Server) Shutdown(ctx context.Context) error {
	return s.http.Shutdown(ctx)
}

// ---- 中间件 ----

// withAuth 校验 Authorization: Bearer <token>，恒定时间比较防时序探测
func (s *Server) withAuth(next http.HandlerFunc) http.HandlerFunc {
	want := []byte(s.cfg.Token)
	return func(w http.ResponseWriter, r *http.Request) {
		got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if subtle.ConstantTimeCompare([]byte(got), want) != 1 {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "未授权：访问令牌缺失或无效"})
			return
		}
		next(w, r)
	}
}

// ---- 处理器 ----

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type chatRequest struct {
	SessionID string `json:"session_id"`
	Message   string `json:"message"`
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	var req chatRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxChatBodyBytes)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求参数解析失败"})
		return
	}
	if strings.TrimSpace(req.Message) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "提问内容不能为空"})
		return
	}
	if strings.TrimSpace(req.SessionID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "缺少会话ID"})
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "服务器不支持流式响应"})
		return
	}

	h := w.Header()
	h.Set("Content-Type", "text/event-stream; charset=utf-8")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no") // 告诉 Nginx 等反代不要缓冲 SSE
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	// 心跳：模型长时间思考（如工具轮询）时防止中间代理/客户端误判定超时
	stopHeartbeat := make(chan struct{})
	go func() {
		ticker := time.NewTicker(sseHeartbeat)
		defer ticker.Stop()
		for {
			select {
			case <-stopHeartbeat:
				return
			case <-ticker.C:
				if _, err := w.Write([]byte(": ping\n\n")); err != nil {
					return
				}
				flusher.Flush()
			}
		}
	}()
	defer close(stopHeartbeat)

	writeEvent := func(event string, payload string) bool {
		data, _ := json.Marshal(payload) // 文本统一 JSON 编码，换行/特殊字符安全
		if _, err := w.Write([]byte("event: " + event + "\ndata: " + string(data) + "\n\n")); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}

	// 客户端断连时 r.Context() 会取消，上游豆包调用随之终止，不白烧 token
	err := s.cfg.Chat.ChatWithSession(r.Context(), req.SessionID, req.Message, func(chunk string) {
		writeEvent("chunk", chunk)
	})
	if err != nil {
		log.Printf("[server] 会话 %s 对话失败: %v", req.SessionID, err)
		writeEvent("error", err.Error())
		return
	}
	writeEvent("done", "")
}

type resetRequest struct {
	SessionID string `json:"session_id"`
}

func (s *Server) handleReset(w http.ResponseWriter, r *http.Request) {
	var req resetRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&req); err != nil || req.SessionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "缺少会话ID"})
		return
	}
	s.cfg.Chat.Reset(req.SessionID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
