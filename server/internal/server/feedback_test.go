package server

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"yhagent-server/internal/ai"
)

func newFeedbackTestServer(t *testing.T, svc *ai.FeedbackService) *Server {
	t.Helper()
	s, err := New(Config{
		Addr:     "127.0.0.1:0",
		Token:    "access-token",
		Chat:     stubChat{},
		Feedback: svc,
	})
	if err != nil {
		t.Fatalf("New 失败: %v", err)
	}
	return s
}

// TestHandleFeedback 反馈接口：鉴权、参数校验、点赞进案例库全链路
func TestHandleFeedback(t *testing.T) {
	st, err := ai.NewSQLiteFeedbackStore(filepath.Join(t.TempDir(), "feedback.db"))
	if err != nil {
		t.Fatalf("打开反馈库失败: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	svc := ai.NewFeedbackService(st)
	s := newFeedbackTestServer(t, svc)

	do := func(auth string, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/v1/feedback", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if auth != "" {
			req.Header.Set("Authorization", "Bearer "+auth)
		}
		w := httptest.NewRecorder()
		s.http.Handler.ServeHTTP(w, req)
		return w
	}

	// 缺令牌 401
	if w := do("", `{"rating":1}`); w.Code != http.StatusUnauthorized {
		t.Fatalf("缺访问码应 401，实际 %d", w.Code)
	}
	// 非法 rating 400
	if w := do("access-token", `{"rating":3}`); w.Code != http.StatusBadRequest {
		t.Fatalf("rating=3 应 400，实际 %d", w.Code)
	}
	// 坏 JSON 400
	if w := do("access-token", `{not-json`); w.Code != http.StatusBadRequest {
		t.Fatalf("坏 JSON 应 400，实际 %d", w.Code)
	}
	// 正常点赞 200 且进案例库
	body := `{"trace_id":"t_abc","session_id":"sess-1","rating":1,` +
		`"question":"M14 怎么改出据枪+8","answer":"推荐方案如下……"}`
	if w := do("access-token", body); w.Code != http.StatusOK {
		t.Fatalf("正常点赞应 200，实际 %d: %s", w.Code, w.Body.String())
	}
	if block := svc.CaseBlock("m14怎么改出据枪"); !strings.Contains(block, "M14 怎么改出据枪+8") {
		t.Fatalf("点赞后案例库应可检索到该问答，实际: %q", block)
	}
	// 点踩 200（不进案例库）
	if w := do("access-token", `{"trace_id":"t_def","rating":-1,"question":"差评","answer":"不行"}`); w.Code != http.StatusOK {
		t.Fatalf("点踩应 200，实际 %d", w.Code)
	}
}

// TestHandleFeedbackDisabled 未装配反馈服务时返回 503
func TestHandleFeedbackDisabled(t *testing.T) {
	s := newFeedbackTestServer(t, nil)
	req := httptest.NewRequest(http.MethodPost, "/v1/feedback", strings.NewReader(`{"rating":1}`))
	req.Header.Set("Authorization", "Bearer access-token")
	w := httptest.NewRecorder()
	s.http.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("Feedback 为 nil 应 503，实际 %d", w.Code)
	}
}
