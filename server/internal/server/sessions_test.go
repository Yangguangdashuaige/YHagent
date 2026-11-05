package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"yhagent-server/internal/ai"
)

// sessionsChat 会话接口测试桩：返回固定摘要并记录 Reset 调用
type sessionsChat struct {
	stubChat
	summaries []ai.SessionSummary
	resetSID  string
	resetCnt  int
}

func (s *sessionsChat) SessionSummaries(sids []string) []ai.SessionSummary {
	return s.summaries
}

func (s *sessionsChat) Reset(id string) error {
	s.resetSID = id
	s.resetCnt++
	return nil
}

func newSessionsTestServer(t *testing.T, chat *sessionsChat) *Server {
	t.Helper()
	s, err := New(Config{
		Addr:  "127.0.0.1:0",
		Token: "access-token",
		Chat:  chat,
	})
	if err != nil {
		t.Fatalf("New 失败: %v", err)
	}
	return s
}

// TestHandleSessions 会话列表：鉴权、非法 sid 过滤、按最后活跃倒序
func TestHandleSessions(t *testing.T) {
	chat := &sessionsChat{summaries: []ai.SessionSummary{
		{SessionID: "sess_0123456789abcdef0123456789abcd01", Title: "老会话", UpdatedAt: 100, MsgCount: 4},
		{SessionID: "sess_0123456789abcdef0123456789abcd02", Title: "新会话", UpdatedAt: 200, MsgCount: 2},
	}}
	s := newSessionsTestServer(t, chat)

	do := func(auth, query string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/v1/sessions"+query, nil)
		if auth != "" {
			req.Header.Set("Authorization", "Bearer "+auth)
		}
		w := httptest.NewRecorder()
		s.http.Handler.ServeHTTP(w, req)
		return w
	}

	// 缺令牌 401
	if w := do("", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("缺访问码应 401，实际 %d", w.Code)
	}
	// 正常列表：倒序（新在前）
	w := do("access-token", "?ids=sess_0123456789abcdef0123456789abcd01,sess_0123456789abcdef0123456789abcd02")
	if w.Code != http.StatusOK {
		t.Fatalf("列表应 200，实际 %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if strings.Index(body, "sess_0123456789abcdef0123456789abcd02") > strings.Index(body, "sess_0123456789abcdef0123456789abcd01") {
		t.Fatalf("应按最后活跃倒序: %s", body)
	}
	// 非法 sid 混入：不影响请求成功
	if w := do("access-token", "?ids=bad!sid,sess_0123456789abcdef0123456789abcd02"); w.Code != http.StatusOK {
		t.Fatalf("含非法 sid 应仍 200，实际 %d", w.Code)
	}
}

// TestHandleSessionDelete 删除会话：调 Chat.Reset 清档；缺失 sid 静默成功
func TestHandleSessionDelete(t *testing.T) {
	chat := &sessionsChat{}
	s := newSessionsTestServer(t, chat)

	req := httptest.NewRequest(http.MethodDelete, "/v1/session?session_id=sess_0123456789abcdef0123456789abcdaa", nil)
	req.Header.Set("Authorization", "Bearer access-token")
	w := httptest.NewRecorder()
	s.http.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("删除应 200，实际 %d: %s", w.Code, w.Body.String())
	}
	if chat.resetSID != "sess_0123456789abcdef0123456789abcdaa" || chat.resetCnt != 1 {
		t.Fatalf("应调用 Reset 一次，实际 sid=%s cnt=%d", chat.resetSID, chat.resetCnt)
	}

	// 无 sid：按成功返回且不触发删除
	req2 := httptest.NewRequest(http.MethodDelete, "/v1/session", nil)
	req2.Header.Set("Authorization", "Bearer access-token")
	w2 := httptest.NewRecorder()
	s.http.Handler.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK || chat.resetCnt != 1 {
		t.Fatalf("无 sid 应静默成功且不删，实际 code=%d cnt=%d", w2.Code, chat.resetCnt)
	}
}
