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
	"sort"
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
	// AdminToken 管理员口令（传统通道）：联网知识入库/trace 查看需 X-Admin-Token。
	// 新通道：账号 role=admin 直接通过（注册/登录时提供 AUTH_ADMIN_KEY 获得），
	// 两条通道满足其一即可；都未配置时管理接口一律 403。
	AdminToken string
	// Ingester 联网知识采纳入库能力；nil 时采纳接口关闭。
	Ingester Ingester
	// Tracer 请求级 trace 记录器（internal/trace）；nil 时 /v1/traces 查看接口关闭。
	// trace 写入本身在 Agent 内部完成，此处只用于查看端读取。
	Tracer *trace.Recorder
	// Feedback 反馈闭环服务（internal/ai.FeedbackService）；nil 时 POST /v1/feedback
	// 返回 503。收集 👍👎 反馈：点赞问答自动进 few-shot 案例库（Agent 侧注入），
	// 点踩与 trace_id 关联落库供失败模式分析。
	Feedback *ai.FeedbackService
	// MaxConcurrentChat /v1/chat 全局在途对话上限（并发闸门），<=0 用默认值 128。
	// 满员后新请求进入有界排队（ChatQueueWait）而非立刻拒绝，把流量尖峰削平；
	// 该值主要保护上游 LLM 的 RPM/并发配额，按方舟配额调整。
	MaxConcurrentChat int
	// ChatQueueWait 并发满员后排队等待的时长上限，<=0（默认）= 不限时——排队时长
	// 交给用户决定（网页端「停止排队」随时退出）；配置后超时以 SSE error 拒绝。
	ChatQueueWait time.Duration
	// ChatRPMPerIP 单 IP 每分钟对话类请求（chat/会话签发/重置）上限，<=0 用默认值 30。
	ChatRPMPerIP int
	// Auth 账号系统（internal/auth）；nil 时账号接口全部 503、鉴权退回纯主令牌
	// （已有客户端零改动）。启用后：主令牌=匿名身份（uid=0），账号令牌=注入 UID，
	// 登录用户签发的会话受归属保护。
	Auth *auth.Service
	// MCP Streamable HTTP 形态的 MCP 服务处理器（internal/mcp 构造）；
	// 与 MCPToken 同时非空时挂 /mcp 路由，供 Claude Desktop / Trae 等外部
	// MCP 客户端远程挂载 YHagent 工具集。两者任一为空则路由不启用（旧行为）。
	MCP http.Handler
	// MCPToken MCP 客户端访问令牌，独立于 SERVER_TOKEN（Bearer，恒定时间比较）。
	MCPToken string
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

// withMCPToken /mcp 路由的 MCP 令牌校验中间件：独立于主令牌与账号体系
// （MCP 客户端只有这一种凭据），恒定时间比较防时序侧信道
func (s *Server) withMCPToken(next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		got := bearerToken(r)
		if subtle.ConstantTimeCompare([]byte(got), []byte(s.cfg.MCPToken)) != 1 {
			w.Header().Set("WWW-Authenticate", "Bearer")
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "MCP 访问令牌无效"})
			return
		}
		next.ServeHTTP(w, r)
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
		gate: newChatGate(cfg.MaxConcurrentChat, cfg.ChatQueueWait),
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
	mux.HandleFunc("GET /v1/session/history", s.withAuth(s.withRateLimit(s.handleSessionHistory)))
	// 多会话管理：列表（批量摘要）+ 删除单个会话；「开新对话」复用 /v1/session/new
	mux.HandleFunc("GET /v1/sessions", s.withAuth(s.withRateLimit(s.handleSessions)))
	mux.HandleFunc("DELETE /v1/session", s.withAuth(s.withRateLimit(s.handleSessionDelete)))
	// 反馈闭环：回答气泡 👍👎 落库（点赞进案例库、点踩关联 trace 供分析）
	mux.HandleFunc("POST /v1/feedback", s.withAuth(s.withRateLimit(s.handleFeedback)))
	mux.HandleFunc("POST /v1/knowledge/ingest", s.withAuth(s.handleKnowledgeIngest))
	// 请求级 trace 查看接口（调用链路观测，需管理员口令，见 handleTraces）
	mux.HandleFunc("GET /v1/traces", s.withAuth(s.handleTraces))
	// 网页版"访问码"登录时据此校验令牌是否有效，避免发消息才发现 401
	mux.HandleFunc("GET /v1/auth/check", s.withAuth(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}))
	// MCP 服务（可选）：Streamable HTTP 形态，独立令牌 + 按 IP 限流，
	// 每次工具调用都会消耗上游 API（embedding/三角洲数据），不能裸奔。
	// 注意必须按方法显式注册：全方法 "/mcp" 会与 "GET /" 静态页兜底
	// 模式冲突（Go 1.22 ServeMux 规则，New 直接 panic）；
	// Streamable HTTP 协议只用 POST（上行消息）/ GET（SSE 下行流）/ DELETE（会话终止）
	if cfg.MCP != nil && strings.TrimSpace(cfg.MCPToken) != "" {
		s.cfg.MCPToken = strings.TrimSpace(cfg.MCPToken)
		mcpHandler := s.withRateLimit(s.withMCPToken(cfg.MCP))
		mux.Handle("POST /mcp", mcpHandler)
		mux.Handle("GET /mcp", mcpHandler)
		mux.Handle("DELETE /mcp", mcpHandler)
	}

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

// queueEvent SSE queue 事件载荷：Waiting>0 = 正在排队（含自己在内的人数），
// =0 = 已入场。前端据此显示「排队中，前方约 N-1 人」并恢复思考态。
type queueEvent struct {
	Waiting int64 `json:"waiting"`
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
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "服务器不支持流式响应"})
		return
	}

	// 校验通过即建立 SSE 连接：并发满员时请求也不再快速失败，而是在
	// 心跳保活下持续收到排队位置推送（queue 事件），直到入场或等待超时
	// （不认识 queue 事件的旧客户端只表现为稍长的等待，行为向后兼容）。
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

	// 全局并发闸门：快路径直接抢名额（多数请求零等待）；满员则 FIFO 排队，
	// 期间每 5s 推一次 queue 事件（waiting 含自己在内的排队人数）。
	// 排队时长由用户决定：网页端「停止排队」随时退出（断连即让位）；
	// 配置了 ChatQueueWait 时超时也会拒绝（SSE error 事件）。
	if !s.gate.tryAcquire() {
		if !s.gate.acquire(r.Context(), func(waiting int64) {
			writeEvent("queue", queueEvent{Waiting: waiting})
		}) {
			log.Printf("[server] 会话 %s 退出排队（用户停止排队/断连，或超过上限 %s）", req.SessionID, s.gate.waitLimit)
			writeEvent("error", "当前咨询人数较多，请稍后重试")
			return
		}
		// 已入场：通知前端把「排队中」恢复成「思考中」（首个 chunk 可能还需数秒）
		writeEvent("queue", queueEvent{Waiting: 0})
	}
	defer s.gate.release()

	// 客户端断连时 r.Context() 会取消，上游豆包调用随之终止，不白烧 token。
	// 注入候选知识回调：Agent 调用 web_search 时，把候选卡片通过 SSE 推给网页端。
	ctx := websearch.WithCandidateFunc(r.Context(), func(cand websearch.Candidate) {
		writeEvent("candidate", cand)
	})
	// 注入工具状态回调：每个工具开始/结束时推 tool 事件（含入参摘要与轮次），
	// 网页端据此显示「联网搜索: M14改枪… 3.2s」实时进度
	ctx = ai.WithToolEventFunc(ctx, func(ev ai.ToolEvent) {
		if ev.Phase == "start" {
			ev.Label = toolLabel(ev.Name)
		}
		writeEvent("tool", ev)
	})
	// 注入进度回调：轮次状态（第 N 轮思考/执行工具）、思考模型推理流、计划步骤
	// 实时推给网页端（event: round / thought / plan），等待期不再只有一个不动的进度条
	ctx = ai.WithProgressFunc(ctx, func(ev ai.ProgressEvent) {
		switch ev.Kind {
		case ai.ProgressRound:
			writeEvent("round", ev)
		case ai.ProgressThought:
			writeEvent("thought", ev)
		case ai.ProgressPlan:
			writeEvent("plan", ev)
		case ai.ProgressTrace:
			// 本轮 trace 标识：前端反馈接口据此关联观测数据（trace 未启用时不推）
			writeEvent("trace", ev)
		case ai.ProgressClarify:
			// ask_user 候选选项：前端渲染选项卡（玩家点选或自定义输入）
			writeEvent("clarify", ev)
		}
	})
	// 账号身份透传：登录用户（账号令牌）的会话注入长期记忆画像、轮末异步提取用户事实；
	// 主令牌匿名请求 uid=0，长期记忆能力自动关闭，行为与未启用时一致
	ctx = ai.WithUID(ctx, uidFromRequest(r))
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

// feedbackRequest 反馈提交入参（前端回答气泡 👍👎 按钮）
type feedbackRequest struct {
	TraceID   string `json:"trace_id"`  // 可选：本轮 trace 标识（trace 未启用/桌面端为空）
	SessionID string `json:"session_id"`
	Rating    int    `json:"rating"`    // 1=赞 / -1=踩
	Question  string `json:"question"`  // 本轮用户提问原文（案例库检索键）
	Answer    string `json:"answer"`    // 本轮回答原文（点赞时进案例库）
}

// handleFeedback 收集回答反馈：点赞问答对自动进入 few-shot 案例库（后续相似
// 问题注入成功答法），点踩与 trace_id 关联落库供失败模式汇总分析。
// 带鉴权 + 按 IP 限流；反馈是增值能力，任何失败都不影响对话主链路。
func (s *Server) handleFeedback(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Feedback == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "反馈功能未启用"})
		return
	}
	var req feedbackRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求参数解析失败"})
		return
	}
	if req.Rating != 1 && req.Rating != -1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "rating 只支持 1（赞）或 -1（踩）"})
		return
	}
	if err := s.cfg.Feedback.Submit(ai.Feedback{
		TraceID:   req.TraceID,
		SessionID: req.SessionID,
		UID:       uidFromRequest(r),
		Rating:    req.Rating,
		Question:  req.Question,
		Answer:    req.Answer,
	}); err != nil {
		log.Printf("[server] 反馈落库失败: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "反馈记录失败，请稍后再试"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
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

// handleSessionHistory 返回会话的客户端可见历史（网页刷新后恢复对话用）。
// GET /v1/session/history?session_id=xxx。
// 会话正在问答中时也允许读取：拿到的是「截至上一轮完成」的历史，
// 进行中的回答由前端流式通道继续接收，两不冲突。
func (s *Server) handleSessionHistory(w http.ResponseWriter, r *http.Request) {
	sid := r.URL.Query().Get("session_id")
	if sid == "" || !validSessionID(sid) {
		// 非法/缺失 ID：按空历史返回而不是报错——前端刷新流程拿空列表
		// 走欢迎语分支即可，不必弹错误打扰用户
		writeJSON(w, http.StatusOK, historyResponse{Messages: []ai.HistoryMessage{}})
		return
	}
	if !s.checkSessionOwnership(w, r, sid) {
		return
	}
	writeJSON(w, http.StatusOK, historyResponse{
		Messages: s.cfg.Chat.SessionHistory(uidFromRequest(r), sid),
	})
}

// historyResponse 历史查询响应
type historyResponse struct {
	Messages []ai.HistoryMessage `json:"messages"`
}

// handleSessions 多会话列表：GET /v1/sessions?ids=sess_a,sess_b。
// 客户端上报自己本地记录的 sid 集合（登录用户也可换设备后拿服务端归属
// 重新发现，但当前实现以客户端记录为准），服务端逐个做归属过滤后返回
// 批量摘要（标题/最后活跃/消息数），按最后活跃倒序。
func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	raw := strings.TrimSpace(r.URL.Query().Get("ids"))
	var sids []string
	if raw != "" {
		sids = strings.Split(raw, ",")
	}
	if len(sids) > 50 {
		sids = sids[:50]
	}
	uid := uidFromRequest(r)
	allowed := make([]string, 0, len(sids))
	for _, sid := range sids {
		sid = strings.TrimSpace(sid)
		if !validSessionID(sid) {
			continue
		}
		// 归属过滤：绑定了他人账号的会话不进列表（静默跳过，不泄露存在性）
		if s.cfg.Auth != nil {
			if owner, ok := s.cfg.Auth.SessionOwner(sid); ok && owner != uid {
				continue
			}
		}
		allowed = append(allowed, sid)
	}
	items := s.cfg.Chat.SessionSummaries(allowed)
	if items == nil {
		items = []ai.SessionSummary{}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].UpdatedAt > items[j].UpdatedAt })
	writeJSON(w, http.StatusOK, map[string]any{"sessions": items})
}

// handleSessionDelete 删除单个会话：DELETE /v1/session?session_id=xxx。
// 语义 = 原地清空并移除（内存上下文 + 磁盘落库 + 账号归属解绑）；
// 「开新对话不动旧档」由客户端直接换新 sid 实现（POST /v1/session/new），
// 不走本接口。会话问答中返回 409，与 reset 同口径。
func (s *Server) handleSessionDelete(w http.ResponseWriter, r *http.Request) {
	sid := strings.TrimSpace(r.URL.Query().Get("session_id"))
	if sid == "" || !validSessionID(sid) {
		// 非法/缺失 ID 按成功返回，不向调用方确认 ID 是否存在
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}
	if !s.checkSessionOwnership(w, r, sid) {
		return
	}
	if err := s.cfg.Chat.Reset(sid); err != nil {
		if errors.Is(err, ai.ErrSessionBusy) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "对话正在进行中，请稍后再删除"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "删除会话失败"})
		return
	}
	if s.cfg.Auth != nil {
		s.cfg.Auth.UnbindSession(sid)
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
// 管理员账号登录（role=admin）直接通过；否则需要 X-Admin-Token 管理员口令。
func (s *Server) handleKnowledgeIngest(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Ingester == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "知识入库功能未开启（知识库未加载）"})
		return
	}
	if !s.requireAdmin(w, r) {
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
// 管理员账号登录（role=admin）直接通过；否则需要 X-Admin-Token 管理员口令。
// trace 含各会话的提问内容，不开放给普通访问码用户。
func (s *Server) handleTraces(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Tracer == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "trace 查看功能未开启（需启用 trace 记录）"})
		return
	}
	if !s.requireAdmin(w, r) {
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
