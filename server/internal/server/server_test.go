package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"yhagent-server/internal/ai"
	"yhagent-server/internal/trace"
)

// stubChat 仅满足 New() 的非空校验，trace 接口测试不触发对话
type stubChat struct{}

func (stubChat) ChatWithSession(ctx context.Context, sessionID, userText string, onChunk func(string)) error {
	return nil
}
func (stubChat) ChatWithSessionImages(ctx context.Context, sessionID, userText string, images []string, onChunk func(string)) error {
	return nil
}
func (stubChat) Reset(sessionID string) error { return nil }

// 编译期确认 stubChat 满足 ChatService 接口
var _ ai.ChatService = stubChat{}

func newTraceTestServer(t *testing.T, rec *trace.Recorder) *Server {
	t.Helper()
	s, err := New(Config{
		Addr:       "127.0.0.1:0",
		Token:      "access-token",
		Chat:       stubChat{},
		AdminToken: "admin-token",
		Tracer:     rec,
	})
	if err != nil {
		t.Fatalf("New 失败: %v", err)
	}
	return s
}

// TestHandleTraces 走完整 mux 验证双鉴权（访问码 + 管理员口令）与数据返回
func TestHandleTraces(t *testing.T) {
	dir := t.TempDir()
	rec := trace.NewRecorder(dir, 7)
	t.Cleanup(rec.Close)

	// 经全局 Default 写一条真实 trace，再由查看接口读回
	trace.Default = rec
	t.Cleanup(func() { trace.Default = nil })
	span := trace.Start("sess-1", "M14 怎么压枪", 0)
	if span == nil {
		t.Fatalf("trace.Default 已装配，Start 不应返回 nil")
	}
	span.LLMRound(1200, 800, 200, 1000)
	span.ToolCall(1, "search_knowledge", `{"query":"M14压枪"}`, 300, 2048, true)
	span.NoteCacheHit()
	span.AnswerChars(180)
	span.End("")

	s := newTraceTestServer(t, rec)
	do := func(auth, admin string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/v1/traces?limit=10", nil)
		if auth != "" {
			req.Header.Set("Authorization", "Bearer "+auth)
		}
		if admin != "" {
			req.Header.Set("X-Admin-Token", admin)
		}
		w := httptest.NewRecorder()
		s.http.Handler.ServeHTTP(w, req)
		return w
	}

	if w := do("", "admin-token"); w.Code != http.StatusUnauthorized {
		t.Fatalf("缺访问码应 401，实际 %d", w.Code)
	}
	if w := do("access-token", ""); w.Code != http.StatusForbidden {
		t.Fatalf("缺管理员口令应 403，实际 %d", w.Code)
	}
	if w := do("access-token", "wrong"); w.Code != http.StatusForbidden {
		t.Fatalf("错误管理员口令应 403，实际 %d", w.Code)
	}

	w := do("access-token", "admin-token")
	if w.Code != http.StatusOK {
		t.Fatalf("双令牌正确应 200，实际 %d: %s", w.Code, w.Body.String())
	}
	var payload struct {
		Traces []trace.Record `json:"traces"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("响应解析失败: %v", err)
	}
	if len(payload.Traces) != 1 {
		t.Fatalf("期望 1 条 trace，实际 %d", len(payload.Traces))
	}
	got := payload.Traces[0]
	if got.SessionID != "sess-1" || got.LLMRounds != 1 || len(got.Tools) != 1 || !got.OK {
		t.Fatalf("trace 内容异常: %+v", got)
	}
	if got.Tools[0].Name != "search_knowledge" || got.Tools[0].ResultBytes != 2048 {
		t.Fatalf("工具调用内容异常: %+v", got.Tools[0])
	}
}

// TestHandleTracesDisabled 未装配 Tracer 时返回 503（与知识入库接口同款行为）
func TestHandleTracesDisabled(t *testing.T) {
	s := newTraceTestServer(t, nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/traces", nil)
	req.Header.Set("Authorization", "Bearer access-token")
	req.Header.Set("X-Admin-Token", "admin-token")
	w := httptest.NewRecorder()
	s.http.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("Tracer 为 nil 应 503，实际 %d", w.Code)
	}
}
