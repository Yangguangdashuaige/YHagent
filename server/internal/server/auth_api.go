// internal/server/auth_api.go
//
// 账号系统 HTTP 接口：注册 / 登录 / 登出 / 查询本人信息，以及请求身份的
// 中间件解析（主令牌 → 匿名 uid=0；账号令牌 → 注入 UID）。
//
// 兼容性关键点：现有客户端持有的是主令牌（SERVER_TOKEN），全部既有行为不变；
// 账号令牌是叠加的第二种身份，登录用户的会话受归属保护，主令牌会话保持开放。
package server

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"yhagent-server/internal/auth"
)

// uidKey context 键（私有类型防碰撞）
type uidKey struct{}

// uidFromRequest 取当前请求的 UID；0 = 主令牌匿名身份（或账号系统未启用）
func uidFromRequest(r *http.Request) int64 {
	uid, _ := r.Context().Value(uidKey{}).(int64)
	return uid
}

// bearerToken 提取 Authorization: Bearer <token>（无前缀返回空串）
func bearerToken(r *http.Request) string {
	return strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
}

// withAuth 身份解析中间件（升级自纯主令牌校验）：
//   - Bearer == SERVER_TOKEN → 匿名身份（uid=0），既有客户端零改动；
//   - 否则当作账号令牌到 auth.Service 校验，通过 → 注入 UID；
//   - 都不匹配 → 401。
// cfg.Auth 为 nil（账号系统未启用）时行为与旧版完全一致。
func (s *Server) withAuth(next http.HandlerFunc) http.HandlerFunc {
	want := []byte(s.cfg.Token)
	return func(w http.ResponseWriter, r *http.Request) {
		got := bearerToken(r)
		if subtle.ConstantTimeCompare([]byte(got), want) == 1 {
			next(w, r.WithContext(context.WithValue(r.Context(), uidKey{}, int64(0))))
			return
		}
		if s.cfg.Auth != nil {
			if u, ok := s.cfg.Auth.VerifyToken(got); ok {
				next(w, r.WithContext(context.WithValue(r.Context(), uidKey{}, u.ID)))
				return
			}
		}
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "未授权：访问令牌缺失或无效"})
	}
}

// withUserAuth 要求账号登录（主令牌匿名身份不可用账号专属接口）
func (s *Server) withUserAuth(next http.HandlerFunc) http.HandlerFunc {
	return s.withAuth(func(w http.ResponseWriter, r *http.Request) {
		if uidFromRequest(r) == 0 {
			writeJSON(w, http.StatusUnauthorized, map[string]string{
				"error": "此接口需要账号登录",
				"code":  "account_required",
			})
			return
		}
		next(w, r)
	})
}

// ---- 请求/响应 DTO ----

type credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type authResponse struct {
	UserID    int64  `json:"user_id"`
	Username  string `json:"username"`
	Token     string `json:"token"`
	ExpiresAt string `json:"expires_at"` // RFC3339
}

// handleRegister 注册即登录。开放接口（未登录可访问），由按 IP 限流兜底。
func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Auth == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "账号系统未启用"})
		return
	}
	var c credentials
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&c); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求参数解析失败"})
		return
	}
	u, token, exp, err := s.cfg.Auth.Register(strings.TrimSpace(c.Username), c.Password)
	if err != nil {
		s.writeAuthErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, authResponse{
		UserID: u.ID, Username: u.Username, Token: token, ExpiresAt: exp.Format(time.RFC3339),
	})
}

// handleLogin 登录签发令牌。开放接口，由按 IP 限流 + 用户名失败锁定兜底。
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Auth == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "账号系统未启用"})
		return
	}
	var c credentials
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&c); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求参数解析失败"})
		return
	}
	u, token, exp, err := s.cfg.Auth.Login(c.Username, c.Password)
	if err != nil {
		s.writeAuthErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, authResponse{
		UserID: u.ID, Username: u.Username, Token: token, ExpiresAt: exp.Format(time.RFC3339),
	})
}

// handleLogout 注销当前令牌（幂等）
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Auth == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "账号系统未启用"})
		return
	}
	s.cfg.Auth.Logout(bearerToken(r))
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleMe 当前账号信息
func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Auth == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "账号系统未启用"})
		return
	}
	u, ok := s.cfg.Auth.VerifyToken(bearerToken(r))
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "登录状态已失效，请重新登录"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user_id":    u.ID,
		"username":   u.Username,
		"created_at": u.CreatedAt.Format(time.RFC3339),
	})
}

// writeAuthErr 把 auth 包错误映射为对外状态码（不透传内部实现细节）
func (s *Server) writeAuthErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrUserExists):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	case errors.Is(err, auth.ErrBadUsername), errors.Is(err, auth.ErrWeakPassword):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	case errors.Is(err, auth.ErrRegisterClosed):
		writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
	case errors.Is(err, auth.ErrBadCredentials):
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
	case errors.Is(err, auth.ErrLocked):
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": err.Error()})
	default:
		// 内部错误（数据库等）只给通用提示，细节进服务端日志由运维排查
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "服务暂时不可用，请稍后再试"})
	}
}

// checkSessionOwnership 会话归属校验（chat/reset 共用）：
// 会话已绑定 UID 且与请求者不一致 → 403，挡住"拿到 sessionID 就能操作别人对话"。
// 未归属会话（主令牌客户端签发的历史会话）保持开放，行为与升级前一致。
func (s *Server) checkSessionOwnership(w http.ResponseWriter, r *http.Request, sessionID string) bool {
	if s.cfg.Auth == nil {
		return true
	}
	owner, ok := s.cfg.Auth.SessionOwner(sessionID)
	if !ok || owner == uidFromRequest(r) {
		return true
	}
	writeJSON(w, http.StatusForbidden, map[string]string{
		"error": "该会话属于其他账号，无权访问",
		"code":  "session_forbidden",
	})
	return false
}
