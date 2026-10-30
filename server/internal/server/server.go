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
//	event: tool
//	data: {"name":"web_search","phase":"start","label":"联网搜索"}
//	data: {"name":"web_search","phase":"end","ms":3200,"ok":true}
//	      Agent 每个工具调用开始/结束时推送，网页端显示实时执行进度。
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
	"yhagent-server/internal/auth"
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
	// MaxConcurrentChat /v1/chat 全局在途对话上限（并发闸门），<=0 用默认值 16。
	// 把上游 LLM 并发成本钉死在可预期范围，满了快速返回 503 而非排队拖死。
	MaxConcurrentChat int
	// ChatRPMPerIP 单 IP 每分钟对话类请求（chat/会话签发/重置）上限，<=0 用默认值 30。
	ChatRPMPerIP int
	// Auth 账号系统（internal/auth）；nil 时账号接口全部 503、鉴权退回纯主令牌
	// （已有客户端零改动）。启用后：主令牌=匿名身份（uid=0），账号令牌=注入 UID，
	// 登录用户签发的会话受归属保护。
	Auth *auth.Service
}

// errInvalidSession 会话标识非法时的统一响应体（前端据 code 自动重新签发并重试）
func errInvalidSession() map[string]string {
	return map[string]string{
		"error": "会话已失效，请重新开始对话",
		"code":  "invalid_session",
	}
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
// 第三个返回值为注入扫描警示（空 = 干净），随 done 事件透传给管理员。
type Ingester interface {
	Ingest(ctx context.Context, in IngestInput, onProgress func(IngestProgress)) (source string, chunks int, warning string, err error)
}

// Server HTTP 服务
type Server struct {
	cfg     Config
	http    *http.Server
	gate    *chatGate  // /v1/chat 全局并发闸门
	limiter *ipLimiter // 对话类接口按 IP 限流
}

// withRateLimit 对话类接口的按 IP 限流中间件（超阈返回 429）
func (s *Server) withRateLimit(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.limiter != nil && !s.limiter.allow(clientIP(r)) {
			w.Header().Set("Retry-After", "60")
			writeJSON(w, http.StatusTooManyRequests, map[string]string{
				"error": "请求过于频繁，请稍候再试",
			})
			return
		}
		next(w, r)
	}
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
	s := &Server{
		cfg:  cfg,
		gate: newChatGate(cfg.MaxConcurrentChat),
	}
	// ChatRPMPerIP：0=默认阈值；负数=显式关闭限流
	if cfg.ChatRPMPerIP >= 0 {
		s.limiter = newIPLimiter(cfg.ChatRPMPerIP)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	// 账号系统四件套：注册/登录开放（仅按 IP 限流兜底），登出/查本人需账号令牌
	mux.HandleFunc("POST /v1/auth/register", s.withRateLimit(s.handleRegister))
	mux.HandleFunc("POST /v1/auth/login", s.withRateLimit(s.handleLogin))
	mux.HandleFunc("POST /v1/auth/logout", s.withUserAuth(s.handleLogout))
	mux.HandleFunc("GET /v1/auth/me", s.withUserAuth(s.handleMe))
	// 对话类三件套统一叠加鉴权 + 按 IP 限流（中间件从外到内：先鉴权再限流）
	mux.HandleFunc("POST /v1/chat", s.withAuth(s.withRateLimit(s.handleChat)))
	mux.HandleFunc("POST /v1/session/new", s.withAuth(s.withRateLimit(s.handleNewSession)))
	mux.HandleFunc("POST /v1/session/reset", s.withAuth(s.withRateLimit(s.handleReset)))
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
// withAuth 身份解析中间件已升级支持账号令牌，见 auth_api.go。

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
	// 会话标识必须来自服务端签发或历史 UUID v4：挡掉任意字符串撞号越权。
	// 必须在写出 SSE 200 头之前以普通 JSON 400 返回，前端识别 code 后自动换新 ID 重试。
	if !validSessionID(req.SessionID) {
		writeJSON(w, http.StatusBadRequest, errInvalidSession())
		return
	}
	// 会话归属：登录用户签发的会话只有归属者本人可用（挡 sessionID 盗用越权）
	if !s.checkSessionOwnership(w, r, req.SessionID) {
		return
	}
	// 全局并发闸门：满员快速失败（503 + Retry-After），不占着连接排队，
	// 也防止流量尖峰把上游 LLM RPM 配额与本机内存一起打爆。
	if !s.gate.tryAcquire() {
		w.Header().Set("Retry-After", "5")
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "当前咨询人数较多，请稍后再试",
		})
		return
	}
	defer s.gate.release()

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

	// 心跳：模型长时间思考（如工具轮询）时防止中间代理/客户端误判定超时。
	// 同轮多工具并行执行后，chunk/candidate/tool 回调可能来自不同 goroutine，
	// 心跳协程也会并发写：所有 SSE 写入统一走 writeMu 串行化，防止响应撕裂。
	var writeMu sync.Mutex
	stopHeartbeat := make(chan struct{})
	go func() {
		ticker := time.NewTicker(sseHeartbeat)
		defer ticker.Stop()
		for {
			select {
			case <-stopHeartbeat:
				return
			case <-ticker.C:
				writeMu.Lock()
				_, err := w.Write([]byte(": ping\n\n"))
				if err == nil {
					flusher.Flush()
				}
				writeMu.Unlock()
				if err != nil {
					return
				}
			}
		}
	}()
	defer close(stopHeartbeat)

	writeEvent := func(event string, payload any) bool {
		data, _ := json.Marshal(payload) // 文本/结构体统一 JSON 编码，换行/特殊字符安全
		writeMu.Lock()
		defer writeMu.Unlock()
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
	// 注入工具状态回调：每个工具开始/结束时推 tool 事件，网页端据此显示
	// 「联网搜索中… 3.2s」实时进度，消除工具执行期的静默体感
	ctx = ai.WithToolEventFunc(ctx, func(ev ai.ToolEvent) {
		if ev.Phase == "start" {
			ev.Label = toolLabel(ev.Name)
		}
		writeEvent("tool", ev)
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

// toolLabel 工具名 → 网页端进度条展示的中文标签（未命中映射的 delta 工具按前缀归并）
func toolLabel(name string) string {
	switch name {
	case "web_search":
		return "联网搜索"
	case "search_knowledge":
		return "知识库检索"
	case "gun_build_advisor":
		return "改枪方案生成"
	case "recognize_screenshot":
		return "截图识别"
	}
	if strings.HasPrefix(name, "delta_") {
		return "游戏数据查询"
	}
	return name
}

func (s *Server) handleReset(w http.ResponseWriter, r *http.Request) {
	var req resetRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&req); err != nil || req.SessionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "缺少会话ID"})
		return
	}
	if !validSessionID(req.SessionID) {
		// 非法 ID 无需重置：直接按成功返回，避免向调用方确认 ID 是否存在
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}
	// 归属校验：清空他人会话同样是越权，与 chat 同口径
	if !s.checkSessionOwnership(w, r, req.SessionID) {
		return
	}
	if err := s.cfg.Chat.Reset(req.SessionID); err != nil {
		if errors.Is(err, ai.ErrSessionBusy) {
			// 会话正在问答中：告诉客户端稍后重试，别在流式输出期间清空历史
			writeJSON(w, http.StatusConflict, map[string]string{"error": "对话正在进行中，请稍后再清空"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "清空会话失败"})
		return
	}
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

	source, n, warn, err := s.cfg.Ingester.Ingest(ctx, IngestInput{
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
	if warn != "" {
		log.Printf("[server] 联网知识入库含疑似注入文本（%s）：%s", source, warn)
	}
	log.Printf("[server] 联网知识已入库：%s（%d 个切片）", source, n)
	send("done", map[string]any{
		"status":  "ok",
		"source":  source,
		"chunks":  n,
		"warning": warn,
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
