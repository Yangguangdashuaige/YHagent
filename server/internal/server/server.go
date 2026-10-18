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
//	event: candidate
//	data: {"query":"...","summary":"...","sources":[{...}],"searched_at":"..."}
//	      Agent 调用 web_search 联网核实时推送的候选知识卡片，
//	      网页端可凭管理员口令 POST /v1/knowledge/ingest 采纳入库。
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
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"yhagent-server/internal/ai"
	"yhagent-server/internal/trace"
	"yhagent-server/internal/websearch"
)

const (
	maxChatBodyBytes = 12 << 20 // 单条请求上限 12MB：容纳最多3张压缩后的截图（base64）
	maxChatImages    = 3        // 单条消息最多携带图片数
	maxImageBytes    = 4 << 20  // 单张图片解码后上限 4MB
	sseHeartbeat     = 25 * time.Second
	// knowledgeIngestTimeout 采纳入库总超时：AI 整理文档（思考型模型实测约3分钟）+向量化
	knowledgeIngestTimeout = 240 * time.Second
)

// Config 服务端配置
type Config struct {
	Addr  string         // 监听地址，如 ":8080"
	Token string         // 客户端访问令牌（Bearer），必填，拒绝裸奔
	Chat  ai.ChatService // Agent 核心（工具已注册、缓存已挂载）
	// Web 网页版静态资源根（含 index.html）。传入后根路径提供网页入口；
	// 传 nil 则只提供 /v1/* API（网页功能关闭）。
	Web fs.FS
	// AdminToken 管理员口令：把联网知识确认入库需要 X-Admin-Token。
	// 留空则采纳接口直接关闭（返回 503），防止普通访问码用户污染知识库。
	AdminToken string
	// Ingester 联网知识采纳入库能力；nil 时采纳接口关闭。
	Ingester Ingester
	// Tracer 请求级 trace 记录器（internal/trace）；nil 时 /v1/traces 查看接口关闭。
	// trace 写入本身在 Agent 内部完成，此处只用于查看端读取。
	Tracer *trace.Recorder
}

// IngestSource 采纳请求里的引用来源
type IngestSource struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

// IngestInput 采纳入库入参（服务端层 DTO，与 websearch 包类型解耦）
type IngestInput struct {
	Query   string         `json:"query"`
	Summary string         `json:"summary"`
	Sources []IngestSource `json:"sources"`
}

// IngestProgress 入库进度事件（SSE 实时推给网页端）。
// Stage 取值：organizing（AI 开始整理）、writing（Delta 为 AI 撰写增量）、
// chunking（切片中）、embedding（Done/Total 为分批向量化进度）。
type IngestProgress struct {
	Stage string `json:"stage"`
	Done  int    `json:"done,omitempty"`
	Total int    `json:"total,omitempty"`
	Delta string `json:"delta,omitempty"`
}

// Ingester 采纳入库接口（由 internal/ingest.Service 经适配后注入）。
// onProgress 可为 nil；非 nil 时会被同步回调推送入库阶段进度。
type Ingester interface {
	Ingest(ctx context.Context, in IngestInput, onProgress func(IngestProgress)) (source string, chunks int, err error)
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
	mux.HandleFunc("POST /v1/knowledge/ingest", s.withAuth(s.handleKnowledgeIngest))
	// 请求级 trace 查看接口（调用链路观测，需管理员口令，见 handleTraces）
	mux.HandleFunc("GET /v1/traces", s.withAuth(s.handleTraces))
	// 网页版"访问码"登录时据此校验令牌是否有效，避免发消息才发现 401
	mux.HandleFunc("GET /v1/auth/check", s.withAuth(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}))

	// 网页入口：根路径提供静态页面。
	// ServeMux 按最长前缀匹配，/v1/、/healthz 等更具体的路由优先命中，API 不受影响。
	if cfg.Web != nil {
		mux.Handle("GET /", http.FileServerFS(cfg.Web))
	}

	s.http = &http.Server{
		Addr:              cfg.Addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second, // 图片消息体积较大，给慢网络上传留足时间
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
	SessionID string   `json:"session_id"`
	Message   string   `json:"message"`
	Images    []string `json:"images"` // 可选，元素为 data URL：data:image/jpeg;base64,...
}

// validateImages 校验图片数量、data URL 形态与解码后大小，返回清洗后的列表
func validateImages(images []string) ([]string, error) {
	if len(images) == 0 {
		return nil, nil
	}
	if len(images) > maxChatImages {
		return nil, fmt.Errorf("单次最多发送 %d 张图片", maxChatImages)
	}
	out := make([]string, 0, len(images))
	for _, raw := range images {
		d := strings.TrimSpace(raw)
		if d == "" {
			continue
		}
		comma := strings.IndexByte(d, ',')
		if !strings.HasPrefix(d, "data:image/") || comma < 0 {
			return nil, errors.New("图片格式不合法，仅支持 data URL 形式的图片")
		}
		head := d[:comma]
		// 允许常见截图格式；;base64 标记必须存在
		allowed := strings.Contains(head, "image/jpeg") || strings.Contains(head, "image/jpg") ||
			strings.Contains(head, "image/png") || strings.Contains(head, "image/webp") ||
			strings.Contains(head, "image/gif")
		if !allowed || !strings.Contains(head, ";base64") {
			return nil, errors.New("仅支持 JPEG/PNG/WebP/GIF 格式的图片")
		}
		decoded, err := base64.StdEncoding.DecodeString(d[comma+1:])
		if err != nil {
			return nil, errors.New("图片数据解析失败，请重新选择图片")
		}
		if len(decoded) > maxImageBytes {
			return nil, fmt.Errorf("单张图片不能超过 %dMB", maxImageBytes>>20)
		}
		out = append(out, d)
	}
	return out, nil
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	var req chatRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxChatBodyBytes)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求参数解析失败或体积过大"})
		return
	}
	images, err := validateImages(req.Images)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if strings.TrimSpace(req.Message) == "" && len(images) == 0 {
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

	writeEvent := func(event string, payload any) bool {
		data, _ := json.Marshal(payload) // 文本/结构体统一 JSON 编码，换行/特殊字符安全
		if _, err := w.Write([]byte("event: " + event + "\ndata: " + string(data) + "\n\n")); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}

	// 客户端断连时 r.Context() 会取消，上游豆包调用随之终止，不白烧 token。
	// 注入候选知识回调：Agent 调用 web_search 时，把候选卡片通过 SSE 推给网页端。
	ctx := websearch.WithCandidateFunc(r.Context(), func(cand websearch.Candidate) {
		writeEvent("candidate", cand)
	})
	err = s.cfg.Chat.ChatWithSessionImages(ctx, req.SessionID, req.Message, images, func(chunk string) {
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

const maxIngestBodyBytes = 256 << 10 // 候选卡片只有摘要+若干链接，256KB 足够

type ingestRequestBody struct {
	Query   string         `json:"query"`
	Summary string         `json:"summary"`
	Sources []IngestSource `json:"sources"`
}

// handleKnowledgeIngest 管理员确认联网知识入库：
// 需同时持有访问码（withAuth）和 X-Admin-Token 管理员口令。
func (s *Server) handleKnowledgeIngest(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Ingester == nil || strings.TrimSpace(s.cfg.AdminToken) == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "知识入库功能未开启（需配置 ADMIN_TOKEN 并装配入库服务）"})
		return
	}
	admin := strings.TrimSpace(r.Header.Get("X-Admin-Token"))
	if subtle.ConstantTimeCompare([]byte(admin), []byte(strings.TrimSpace(s.cfg.AdminToken))) != 1 {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "管理员口令缺失或无效"})
		return
	}

	var req ingestRequestBody
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxIngestBodyBytes)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求参数解析失败或体积过大"})
		return
	}
	if strings.TrimSpace(req.Summary) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "待入库摘要不能为空"})
		return
	}
	if len(req.Sources) > 10 {
		req.Sources = req.Sources[:10]
	}

	// AI 整理（思考型模型首字较慢，实测约3分钟）+ 分批向量化可能耗时数分钟，
	// 用独立超时兜底，避免异常情况下请求无限挂起、前端永久转圈。
	ctx, cancel := context.WithTimeout(r.Context(), knowledgeIngestTimeout)
	defer cancel()
	log.Printf("[server] 收到知识入库请求：query=%q 摘要=%d字 来源=%d条",
		req.Query, len([]rune(req.Summary)), len(req.Sources))

	// 鉴权通过后切换为 SSE：先回 ready 让前端立刻知道口令正确，
	// 之后实时推送 AI 撰写/切片/向量化进度（全程可能数分钟，靠心跳保活）。
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "服务器不支持流式响应"})
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream; charset=utf-8")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	var writeMu sync.Mutex
	send := func(event string, payload any) {
		writeMu.Lock()
		defer writeMu.Unlock()
		var buf strings.Builder
		fmt.Fprintf(&buf, "event: %s\ndata: ", event)
		_ = json.NewEncoder(&buf).Encode(payload)
		_, _ = io.WriteString(w, buf.String()+"\n")
		flusher.Flush()
	}

	// 心跳：AI 整理阶段可能长时间没有事件，防止反代/浏览器按空闲连接断开
	stopBeat := make(chan struct{})
	go func() {
		t := time.NewTicker(sseHeartbeat)
		defer t.Stop()
		for {
			select {
			case <-stopBeat:
				return
			case <-ctx.Done():
				return
			case <-t.C:
				writeMu.Lock()
				_, _ = io.WriteString(w, ": ping\n\n")
				flusher.Flush()
				writeMu.Unlock()
			}
		}
	}()

	// ready = 管理员口令校验通过（前端据此给出即时正反馈并缓存口令）
	send("ready", map[string]string{"status": "ok"})

	source, n, err := s.cfg.Ingester.Ingest(ctx, IngestInput{
		Query:   req.Query,
		Summary: req.Summary,
		Sources: req.Sources,
	}, func(p IngestProgress) {
		if p.Stage == "writing" {
			// AI 撰写增量单独走 doc 事件，避免与步骤状态混在一起
			send("doc", map[string]string{"delta": p.Delta})
			return
		}
		send("progress", p)
	})
	close(stopBeat)
	if err != nil {
		log.Printf("[server] 联网知识入库失败: %v", err)
		msg := "知识入库失败：" + err.Error()
		if errors.Is(err, context.DeadlineExceeded) {
			msg = "资料整理超时（AI 响应过慢），请稍后重试"
		}
		send("error", map[string]string{"error": msg})
		return
	}
	log.Printf("[server] 联网知识已入库：%s（%d 个切片）", source, n)
	send("done", map[string]any{
		"status": "ok",
		"source": source,
		"chunks": n,
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// ---- 请求级 trace 查看接口 ----

const (
	tracesDefaultLimit = 50  // 默认返回最近 50 条
	tracesMaxLimit     = 200 // 单次最多返回 200 条
)

// handleTraces 返回最近 N 条请求 trace（管理员查看调用链路观测数据）。
// 与知识入库接口同款双鉴权：访问码（withAuth）+ X-Admin-Token；
// trace 含各会话的提问内容，不开放给普通访问码用户。
func (s *Server) handleTraces(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Tracer == nil || strings.TrimSpace(s.cfg.AdminToken) == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "trace 查看功能未开启（需配置 ADMIN_TOKEN 并启用 trace 记录）"})
		return
	}
	admin := strings.TrimSpace(r.Header.Get("X-Admin-Token"))
	if subtle.ConstantTimeCompare([]byte(admin), []byte(strings.TrimSpace(s.cfg.AdminToken))) != 1 {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "管理员口令缺失或无效"})
		return
	}

	limit := tracesDefaultLimit
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > tracesMaxLimit {
		limit = tracesMaxLimit
	}
	traces := s.cfg.Tracer.Recent(limit)
	if traces == nil {
		traces = []trace.Record{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"traces": traces})
}
