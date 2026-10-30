package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"yhagent-server/internal/auth"
)

// newAuthTestServer 构造带账号系统的测试服务（纯 HTTP 层，不涉 LLM）
func newAuthTestServer(t *testing.T) *Server {
	t.Helper()
	as, err := auth.Open(filepath.Join(t.TempDir(), "accounts.db"), time.Hour, true)
	if err != nil {
		t.Fatalf("打开账号库: %v", err)
	}
	t.Cleanup(func() { as.Close() })
	s, err := New(Config{
		Token:        "master-token",
		Chat:         stubChat{},
		Auth:         as,
		ChatRPMPerIP: -1, // 关闭限流，聚焦鉴权语义
	})
	if err != nil {
		t.Fatalf("构建服务: %v", err)
	}
	return s
}

func callAuth(t *testing.T, s *Server, method, path, bearer, body string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	if bearer != "" {
		r.Header.Set("Authorization", "Bearer "+bearer)
	}
	w := httptest.NewRecorder()
	s.http.Handler.ServeHTTP(w, r)
	return w
}

// TestAuthHTTPFlow 注册→me→登出 的账号令牌全链路，与主令牌并存
func TestAuthHTTPFlow(t *testing.T) {
	s := newAuthTestServer(t)

	// 注册即登录
	w := callAuth(t, s, "POST", "/v1/auth/register", "", `{"username":"alice","password":"password123"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("注册应 200，得到 %d: %s", w.Code, w.Body.String())
	}
	var reg authResponse
	if err := json.Unmarshal(w.Body.Bytes(), &reg); err != nil || reg.Token == "" || reg.UserID == 0 {
		t.Fatalf("注册响应不完整: %s", w.Body.String())
	}

	// 账号令牌过 withAuth：访问 me
	w = callAuth(t, s, "GET", "/v1/auth/me", reg.Token, "")
	if w.Code != http.StatusOK {
		t.Errorf("me 应 200，得到 %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "alice") {
		t.Errorf("me 应包含用户名: %s", w.Body.String())
	}

	// 主令牌访问账号专属接口 → 401 account_required（匿名身份）
	w = callAuth(t, s, "GET", "/v1/auth/me", "master-token", "")
	if w.Code != http.StatusUnauthorized {
		t.Errorf("主令牌访问 me 应 401，得到 %d", w.Code)
	}

	// 主令牌仍可过基础鉴权（匿名身份访问开放接口）
	w = callAuth(t, s, "POST", "/v1/session/new", "master-token", "")
	if w.Code != http.StatusOK {
		t.Errorf("主令牌签发会话应 200（兼容性），得到 %d", w.Code)
	}

	// 登出后令牌失效
	w = callAuth(t, s, "POST", "/v1/auth/logout", reg.Token, "")
	if w.Code != http.StatusOK {
		t.Errorf("登出应 200，得到 %d", w.Code)
	}
	w = callAuth(t, s, "GET", "/v1/auth/me", reg.Token, "")
	if w.Code != http.StatusUnauthorized {
		t.Errorf("登出后 me 应 401，得到 %d", w.Code)
	}
}

// TestSessionOwnership 越权面：A 的会话，B 拿到 sessionID 也无法 chat/reset
func TestSessionOwnership(t *testing.T) {
	s := newAuthTestServer(t)

	// 注册 A、B 两个账号
	regA := callAuth(t, s, "POST", "/v1/auth/register", "", `{"username":"usera","password":"password123"}`)
	regB := callAuth(t, s, "POST", "/v1/auth/register", "", `{"username":"userb","password":"password123"}`)
	var a, b authResponse
	json.Unmarshal(regA.Body.Bytes(), &a)
	json.Unmarshal(regB.Body.Bytes(), &b)

	// A 登录态签发会话 → 归属 A
	w := callAuth(t, s, "POST", "/v1/session/new", a.Token, "")
	if w.Code != http.StatusOK {
		t.Fatalf("A 签发会话失败: %d", w.Code)
	}
	var ns newSessionResponse
	json.Unmarshal(w.Body.Bytes(), &ns)
	if ns.SessionID == "" {
		t.Fatal("未拿到 sessionID")
	}

	// B 持有效令牌访问 A 的会话 → 403（归属校验先于会话业务逻辑）
	// chat 会先做 body 解析/会话合法性校验，归属拦截在流式开始之前
	w = callAuth(t, s, "POST", "/v1/chat", b.Token, `{"session_id":"`+ns.SessionID+`","message":"偷看"}`)
	if w.Code != http.StatusForbidden {
		t.Errorf("B 访问 A 的会话应 403，得到 %d: %s", w.Code, w.Body.String())
	}
	// reset 同口径
	w = callAuth(t, s, "POST", "/v1/session/reset", b.Token, `{"session_id":"`+ns.SessionID+`"}`)
	if w.Code != http.StatusForbidden {
		t.Errorf("B 重置 A 的会话应 403，得到 %d", w.Code)
	}

	// 主令牌（匿名）访问归属会话同样 403——归属保护不因"管理令牌"豁免
	w = callAuth(t, s, "POST", "/v1/session/reset", "master-token", `{"session_id":"`+ns.SessionID+`"}`)
	if w.Code != http.StatusForbidden {
		t.Errorf("主令牌重置 A 的归属会话应 403，得到 %d", w.Code)
	}

	// A 本人访问：归属校验放行（chat 会继续走到会话/LLM 层，这里只要不是 403）
	// Chat=nil 时 A 的 chat 请求会在归属校验之后才失败，验证点是不返回 403
	w = callAuth(t, s, "POST", "/v1/chat", a.Token, `{"session_id":"`+ns.SessionID+`","message":"自己的"}`)
	if w.Code == http.StatusForbidden {
		t.Errorf("A 访问自己的会话不应 403: %s", w.Body.String())
	}
}

// TestAuthDisabled 未启用账号系统：账号接口 503，主令牌鉴权不受影响（旧行为）
func TestAuthDisabled(t *testing.T) {
	s, err := New(Config{Token: "master-token", Chat: stubChat{}, ChatRPMPerIP: -1})
	if err != nil {
		t.Fatal(err)
	}
	w := callAuth(t, s, "POST", "/v1/auth/register", "", `{"username":"x","password":"password123"}`)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("未启用时注册应 503，得到 %d", w.Code)
	}
	// 坏令牌仍被 401
	w = callAuth(t, s, "POST", "/v1/session/new", "whatever", "")
	if w.Code != http.StatusUnauthorized {
		t.Errorf("坏令牌应 401，得到 %d", w.Code)
	}
	// 主令牌正常
	w = callAuth(t, s, "POST", "/v1/session/new", "master-token", "")
	if w.Code != http.StatusOK {
		t.Errorf("主令牌应正常使用，得到 %d", w.Code)
	}
}
