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

// newGuardTestServer 用自定义并发/限流参数构造测试服务
func newGuardTestServer(t *testing.T, chat ai.ChatService, maxConcurrent, rpm int) *Server {
	t.Helper()
	s, err := New(Config{
		Addr:              "127.0.0.1:0",
		Token:             "access-token",
		Chat:              chat,
		MaxConcurrentChat: maxConcurrent,
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
	s := newGuardTestServer(t, stubChat{}, 4, 100)

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
	s := newGuardTestServer(t, stubChat{}, 4, 100)
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

// TestChatConcurrencyGate 并发上限=1：第一个在途请求占满名额，第二个立即 503
func TestChatConcurrencyGate(t *testing.T) {
	chat := &blockingChat{entered: make(chan struct{})}
	s := newGuardTestServer(t, chat, 1, 100)

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

	// 名额已满 → 立即 503（不能排队挂起）
	w := doJSON(t, s, http.MethodPost, "/v1/chat",
		`{"session_id":"`+validSessID()+`","message":"插队问题"}`)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("并发满员应 503，实际 %d: %s", w.Code, w.Body.String())
	}
	if w.Header().Get("Retry-After") == "" {
		t.Fatal("503 应带 Retry-After 提示")
	}
	cancel() // 释放第一个在途请求
}

// busyResetChat Reset 永远返回忙，用于 409 验证
type busyResetChat struct{ stubChat }

func (busyResetChat) Reset(sessionID string) error { return ai.ErrSessionBusy }

// TestHandleResetBusy 问答进行中重置会话应返回 409
func TestHandleResetBusy(t *testing.T) {
	s := newGuardTestServer(t, busyResetChat{}, 4, 100)
	body := `{"session_id":"` + validSessID() + `"}`
	w := doJSON(t, s, http.MethodPost, "/v1/session/reset", body)
	if w.Code != http.StatusConflict {
		t.Fatalf("忙时重置应 409，实际 %d: %s", w.Code, w.Body.String())
	}
}

// TestResetInvalidSessionIdempotent 非法 ID 的重置应幂等成功（不泄露 ID 是否存在）
func TestResetInvalidSessionIdempotent(t *testing.T) {
	s := newGuardTestServer(t, stubChat{}, 4, 100)
	w := doJSON(t, s, http.MethodPost, "/v1/session/reset", `{"session_id":"default"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("非法ID重置应幂等 200，实际 %d", w.Code)
	}
}

// TestIPRateLimit 阈值=2：同 IP 第 3 次对话类请求 429，换 IP 放行
func TestIPRateLimit(t *testing.T) {
	s := newGuardTestServer(t, stubChat{}, 4, 2)

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
	s := newGuardTestServer(t, stubChat{}, 4, -1)
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
