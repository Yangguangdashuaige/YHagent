package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"yhagent-server/internal/ai"
)

// newGuardTestServer 用自定义并发/限流/排队参数构造测试服务
func newGuardTestServer(t *testing.T, chat ai.ChatService, maxConcurrent, rpm int, queueWait time.Duration) *Server {
	t.Helper()
	s, err := New(Config{
		Addr:              "127.0.0.1:0",
		Token:             "access-token",
		Chat:              chat,
		MaxConcurrentChat: maxConcurrent,
		ChatQueueWait:     queueWait,
		ChatRPMPerIP:      rpm,
	})
	if err != nil {
		t.Fatalf("New 失败: %v", err)
	}
	return s
}

func doJSON(t *testing.T, s *Server, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	r.RemoteAddr = "192.0.2.10:54321"
	r.Header.Set("Authorization", "Bearer access-token")
	w := httptest.NewRecorder()
	s.http.Handler.ServeHTTP(w, r)
	return w
}

func validSessID() string { return "sess_0123456789abcdef0123456789abcdef" }
func validUUIDv4() string { return "550e8400-e29b-41d4-a716-446655440000" }

// TestValidSessionID 会话标识白名单：签发格式/UUIDv4 通过，任意串被拒
func TestValidSessionID(t *testing.T) {
	ok := []string{
		"sess_" + strings.Repeat("a", 32),
		validSessID(),
		validUUIDv4(),
		"11111111-1111-4111-8111-111111111111", // 全 1 但 version=4、variant=10xx 即合法形态
	}
	for _, id := range ok {
		if !validSessionID(id) {
			t.Errorf("应接受的会话ID被拒: %q", id)
		}
	}
	bad := []string{
		"", "default", "1", "admin", "abc",
		"sess_" + strings.Repeat("a", 31),      // 短一位
		"sess_" + strings.Repeat("g", 32),      // 非 hex
		"Sess_" + strings.Repeat("a", 32),      // 大小写错误
		strings.Repeat("a", 32),                // 缺前缀
		"550e8400-e29b-31d4-a716-446655440000", // version 不是 4
		"550e8400-e29b-41d4-c716-446655440000", // variant 非法
		"550E8400-E29B-41D4-A716-446655440000", // 大写不接受
	}
	for _, id := range bad {
		if validSessionID(id) {
			t.Errorf("应拒绝的会话ID被接受: %q", id)
		}
	}
}

// TestHandleNewSession 签发接口：鉴权 + 每次返回不同的合法 sess_ ID
func TestHandleNewSession(t *testing.T) {
	s := newGuardTestServer(t, stubChat{}, 4, 100, 0)

	// 无访问码 → 401
	r := httptest.NewRequest(http.MethodPost, "/v1/session/new", nil)
	w := httptest.NewRecorder()
	s.http.Handler.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("未鉴权应 401，实际 %d", w.Code)
	}

	seen := map[string]bool{}
	for i := 0; i < 3; i++ {
		w := doJSON(t, s, http.MethodPost, "/v1/session/new", "")
		if w.Code != http.StatusOK {
			t.Fatalf("签发会话应 200，实际 %d: %s", w.Code, w.Body.String())
		}
		var resp newSessionResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("响应解析失败: %v", err)
		}
		if !validSessionID(resp.SessionID) || !strings.HasPrefix(resp.SessionID, issuedSessionPrefix) {
			t.Fatalf("签发的会话ID不合法: %q", resp.SessionID)
		}
		if seen[resp.SessionID] {
			t.Fatalf("两次签发了相同ID: %q", resp.SessionID)
		}
		seen[resp.SessionID] = true
	}
}

// TestHandleChatInvalidSession 非法会话ID必须在 SSE 建连前以 400+code 返回
func TestHandleChatInvalidSession(t *testing.T) {
	s := newGuardTestServer(t, stubChat{}, 4, 100, 0)
	for _, id := range []string{"default", "1", "sess_xyz"} {
		body := `{"session_id":"` + id + `","message":"你好"}`
		w := doJSON(t, s, http.MethodPost, "/v1/chat", body)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("非法会话ID %q 应 400，实际 %d", id, w.Code)
		}
		var resp map[string]string
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		if resp["code"] != "invalid_session" {
			t.Fatalf("应返回 code=invalid_session，实际 %v", resp)
		}
		if strings.Contains(w.Body.String(), "text/event-stream") {
			t.Fatalf("非法ID不应建立 SSE 连接")
		}
	}
}

// blockingChat 进入对话后阻塞，直到 release 或超时；用于验证并发闸门
type blockingChat struct {
	entered chan struct{}
	once    sync.Once
}

func (b *blockingChat) ChatWithSession(ctx context.Context, sessionID, userText string, onChunk func(string)) error {
	return b.ChatWithSessionImages(ctx, sessionID, userText, nil, onChunk)
}
func (b *blockingChat) ChatWithSessionImages(ctx context.Context, sessionID, userText string, images []string, onChunk func(string)) error {
	b.once.Do(func() { close(b.entered) })
	<-ctx.Done()
	return ctx.Err()
}
func (b *blockingChat) Reset(sessionID string) error { return nil }

func (b *blockingChat) SessionHistory(uid int64, sessionID string) []ai.HistoryMessage {
	return nil
}
func (b *blockingChat) SessionSummaries(sids []string) []ai.SessionSummary { return nil }

// TestChatConcurrencyGate 并发上限=1、排队上限 150ms：第一个在途请求占满名额，
// 第二个不再立即 503，而是 SSE 建连收到 queue 排队事件，等待超时后以 error 事件收尾
func TestChatConcurrencyGate(t *testing.T) {
	chat := &blockingChat{entered: make(chan struct{})}
	s := newGuardTestServer(t, chat, 1, 100, 150*time.Millisecond)

	// 用可取消的请求承载第一个在途对话（测完 cancel 释放，避免 goroutine 永久泄漏）
	firstReq := httptest.NewRequest(http.MethodPost, "/v1/chat",
		strings.NewReader(`{"session_id":"`+validSessID()+`","message":"慢问题"}`))
	firstReq.Header.Set("Content-Type", "application/json")
	firstReq.Header.Set("Authorization", "Bearer access-token")
	firstReq.RemoteAddr = "192.0.2.10:54321"
	ctx, cancel := context.WithCancel(firstReq.Context())
	firstReq = firstReq.WithContext(ctx)

	go func() {
		w := httptest.NewRecorder()
		s.http.Handler.ServeHTTP(w, firstReq)
	}()
	select {
	case <-chat.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("首个请求未进入对话处理")
	}

	// 名额已满 → SSE 建连 + queue 排队事件，排队超时后 error 事件（不再 HTTP 503）
	w := doJSON(t, s, http.MethodPost, "/v1/chat",
		`{"session_id":"`+validSessID()+`","message":"插队问题"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("满员应 SSE 200 排队，实际 %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(w.Header().Get("Content-Type"), "text/event-stream") {
		t.Fatalf("应为 SSE 响应，实际 Content-Type=%q", w.Header().Get("Content-Type"))
	}
	if !strings.Contains(body, "event: queue") {
		t.Fatalf("排队期间应收到 queue 事件，响应: %s", body)
	}
	if !strings.Contains(body, "当前咨询人数较多") {
		t.Fatalf("排队超时应以 error 事件拒绝，响应: %s", body)
	}
	cancel() // 释放第一个在途请求
}

// countingChat 每次进入对话发一票通知；blocking=true 时进入后阻塞直到 ctx
// 取消（模拟长对话）。用于验证排队请求在名额释放后入场
type countingChat struct {
	entered  chan struct{} // 每次进入各发一票（带缓冲）
	blocking bool
}

func (c *countingChat) ChatWithSession(ctx context.Context, sessionID, userText string, onChunk func(string)) error {
	return c.ChatWithSessionImages(ctx, sessionID, userText, nil, onChunk)
}
func (c *countingChat) ChatWithSessionImages(ctx context.Context, sessionID, userText string, images []string, onChunk func(string)) error {
	c.entered <- struct{}{}
	if c.blocking {
		<-ctx.Done()
	}
	return nil
}
func (c *countingChat) Reset(sessionID string) error { return nil }

func (c *countingChat) SessionHistory(uid int64, sessionID string) []ai.HistoryMessage {
	return nil
}
func (c *countingChat) SessionSummaries(sids []string) []ai.SessionSummary { return nil }

// waitQueued 轮询等待至少 n 个请求处于排队状态（消除 goroutine 调度竞态）
func waitQueued(t *testing.T, s *Server, n int64) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if s.gate.waiting.Load() >= n {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("请求未进入排队状态（waiting=%d）", s.gate.waiting.Load())
}

// TestChatQueueServedAfterRelease 并发=1、排队不限时（waitLimit=0）：第二个请求
// 排队期间第一个释放名额，第二个应等到名额入场（而不是被拒），
// 且收到 waiting=0 的入场通知
func TestChatQueueServedAfterRelease(t *testing.T) {
	chat := &countingChat{entered: make(chan struct{}, 4), blocking: true}
	s := newGuardTestServer(t, chat, 1, 100, 0)

	// 第一个请求占住唯一名额
	firstReq := httptest.NewRequest(http.MethodPost, "/v1/chat",
		strings.NewReader(`{"session_id":"`+validSessID()+`","message":"第一个"}`))
	firstReq.Header.Set("Content-Type", "application/json")
	firstReq.Header.Set("Authorization", "Bearer access-token")
	firstReq.RemoteAddr = "192.0.2.10:54321"
	ctx1, cancel1 := context.WithCancel(firstReq.Context())
	firstReq = firstReq.WithContext(ctx1)
	go func() {
		w := httptest.NewRecorder()
		s.http.Handler.ServeHTTP(w, firstReq)
	}()
	<-chat.entered // 第一个已入场

	// 第二个请求进入排队（5s 上限内）
	secondReq := httptest.NewRequest(http.MethodPost, "/v1/chat",
		strings.NewReader(`{"session_id":"`+validSessID()+`","message":"第二个"}`))
	secondReq.Header.Set("Content-Type", "application/json")
	secondReq.Header.Set("Authorization", "Bearer access-token")
	secondReq.RemoteAddr = "192.0.2.11:54321"
	ctx2, cancel2 := context.WithCancel(secondReq.Context())
	secondReq = secondReq.WithContext(ctx2)
	secondDone := make(chan string, 1)
	go func() {
		w := httptest.NewRecorder()
		s.http.Handler.ServeHTTP(w, secondReq)
		secondDone <- w.Body.String()
	}()
	waitQueued(t, s, 1) // 确认第二个已在排队，再释放名额

	cancel1() // 释放第一个 → 第二个应入场
	select {
	case <-chat.entered: // 第二个成功入场
	case body := <-secondDone:
		t.Fatalf("排队请求应等到名额入场，实际提前结束: %s", body)
	case <-time.After(3 * time.Second):
		t.Fatal("释放名额后排队请求未入场")
	}
	// 结束第二个请求，检查入场通知（queue waiting=0）确实推送过
	cancel2()
	select {
	case body := <-secondDone:
		if !strings.Contains(body, `"waiting":0`) {
			t.Fatalf("入场后应推送 waiting=0 的 queue 事件，响应: %s", body)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("第二个请求未正常结束")
	}
}

// TestChatQueueClientDisconnect 排队不限时（waitLimit=0）期间客户端断连：
// 等待方立即让位，不占名额也不影响后续请求正常入场
func TestChatQueueClientDisconnect(t *testing.T) {
	chat := &countingChat{entered: make(chan struct{}, 4), blocking: true}
	s := newGuardTestServer(t, chat, 1, 100, 0)

	// 第一个请求占住唯一名额
	firstReq := httptest.NewRequest(http.MethodPost, "/v1/chat",
		strings.NewReader(`{"session_id":"`+validSessID()+`","message":"第一个"}`))
	firstReq.Header.Set("Content-Type", "application/json")
	firstReq.Header.Set("Authorization", "Bearer access-token")
	firstReq.RemoteAddr = "192.0.2.10:54321"
	ctx1, cancel1 := context.WithCancel(firstReq.Context())
	firstReq = firstReq.WithContext(ctx1)
	go func() {
		w := httptest.NewRecorder()
		s.http.Handler.ServeHTTP(w, firstReq)
	}()
	<-chat.entered

	// 第二个请求开始排队，随后客户端断连
	secondReq := httptest.NewRequest(http.MethodPost, "/v1/chat",
		strings.NewReader(`{"session_id":"`+validSessID()+`","message":"排队后离开"}`))
	secondReq.Header.Set("Content-Type", "application/json")
	secondReq.Header.Set("Authorization", "Bearer access-token")
	secondReq.RemoteAddr = "192.0.2.11:54321"
	ctx2, cancel2 := context.WithCancel(secondReq.Context())
	secondReq = secondReq.WithContext(ctx2)
	go func() {
		w := httptest.NewRecorder()
		s.http.Handler.ServeHTTP(w, secondReq)
	}()
	waitQueued(t, s, 1) // 确认第二个已进入排队等待
	cancel2()           // 客户端断连

	// 释放第一个名额：断连的等待方已让位，名额应立即可用（不残留等待）
	cancel1()

	// 第三个请求应零等待直接入场
	thirdReq := httptest.NewRequest(http.MethodPost, "/v1/chat",
		strings.NewReader(`{"session_id":"`+validSessID()+`","message":"第三个"}`))
	thirdReq.Header.Set("Content-Type", "application/json")
	thirdReq.Header.Set("Authorization", "Bearer access-token")
	thirdReq.RemoteAddr = "192.0.2.12:54321"
	thirdCtx, cancel3 := context.WithCancel(thirdReq.Context())
	thirdReq = thirdReq.WithContext(thirdCtx)
	go func() {
		w := httptest.NewRecorder()
		s.http.Handler.ServeHTTP(w, thirdReq)
	}()
	select {
	case <-chat.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("断连让位后名额应立即可用")
	}
	cancel3()
}

// busyResetChat Reset 永远返回忙，用于 409 验证
type busyResetChat struct{ stubChat }

func (busyResetChat) Reset(sessionID string) error { return ai.ErrSessionBusy }

// TestHandleResetBusy 问答进行中重置会话应返回 409
func TestHandleResetBusy(t *testing.T) {
	s := newGuardTestServer(t, busyResetChat{}, 4, 100, 0)
	body := `{"session_id":"` + validSessID() + `"}`
	w := doJSON(t, s, http.MethodPost, "/v1/session/reset", body)
	if w.Code != http.StatusConflict {
		t.Fatalf("忙时重置应 409，实际 %d: %s", w.Code, w.Body.String())
	}
}

// TestResetInvalidSessionIdempotent 非法 ID 的重置应幂等成功（不泄露 ID 是否存在）
func TestResetInvalidSessionIdempotent(t *testing.T) {
	s := newGuardTestServer(t, stubChat{}, 4, 100, 0)
	w := doJSON(t, s, http.MethodPost, "/v1/session/reset", `{"session_id":"default"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("非法ID重置应幂等 200，实际 %d", w.Code)
	}
}

// TestIPRateLimit 阈值=2：同 IP 第 3 次对话类请求 429，换 IP 放行
func TestIPRateLimit(t *testing.T) {
	s := newGuardTestServer(t, stubChat{}, 4, 2, 0)

	call := func(ip string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/v1/session/new", nil)
		r.RemoteAddr = ip + ":1000"
		r.Header.Set("Authorization", "Bearer access-token")
		w := httptest.NewRecorder()
		s.http.Handler.ServeHTTP(w, r)
		return w
	}
	if w := call("10.0.0.1"); w.Code != http.StatusOK {
		t.Fatalf("第1次请求应放行，实际 %d", w.Code)
	}
	if w := call("10.0.0.1"); w.Code != http.StatusOK {
		t.Fatalf("第2次请求应放行，实际 %d", w.Code)
	}
	if w := call("10.0.0.1"); w.Code != http.StatusTooManyRequests {
		t.Fatalf("第3次请求应 429，实际 %d", w.Code)
	}
	// 不同 IP 独立计数，不受影响
	if w := call("10.0.0.2"); w.Code != http.StatusOK {
		t.Fatalf("其他 IP 不应被限流，实际 %d", w.Code)
	}
}

// TestIPRateLimitDisabled ChatRPMPerIP=-1 时限流关闭，超阈值仍全部放行
func TestIPRateLimitDisabled(t *testing.T) {
	s := newGuardTestServer(t, stubChat{}, 4, -1, 0)
	for i := 0; i < 5; i++ {
		if w := doJSON(t, s, http.MethodPost, "/v1/session/new", ""); w.Code != http.StatusOK {
			t.Fatalf("限流关闭时第%d次请求应放行，实际 %d", i+1, w.Code)
		}
	}
}

// TestClientIP XFF 优先取首个地址；无 XFF 回退 RemoteAddr 去端口
func TestClientIP(t *testing.T) {
	cases := map[string]string{
		"1.2.3.4:5678":       "1.2.3.4",
		"[2001:db8::1]:8080": "2001:db8::1",
	}
	for remote, want := range cases {
		r := httptest.NewRequest(http.MethodPost, "/v1/session/new", nil)
		r.RemoteAddr = remote
		if got := clientIP(r); got != want {
			t.Errorf("RemoteAddr=%s 解析出 %s，期望 %s", remote, got, want)
		}
	}
	r := httptest.NewRequest(http.MethodPost, "/v1/session/new", nil)
	r.RemoteAddr = "10.0.0.9:9999"
	r.Header.Set("X-Forwarded-For", "203.0.113.7, 10.0.0.1")
	if got := clientIP(r); got != "203.0.113.7" {
		t.Errorf("XFF 链应取首个地址，实际 %s", got)
	}
}
