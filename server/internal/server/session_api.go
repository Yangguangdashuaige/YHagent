// internal/server/session_api.go
//
// 会话标识（sessionID）的服务端签发与格式校验。
//
// 背景：会话上下文完全在服务端内存、仅凭 sessionID 寻址。若允许客户端
// 任意指定 ID，持访问码的人可以用 "default"、"1"、"admin" 之类猜测/碰撞
// 他人会话，读到别人的对话上下文（越权）。本文件把 ID 收敛为两类
// 高熵随机标识，猜测成本均为 2^122 以上：
//
//   - sess_<32位hex>：服务端用 crypto/rand 签发（POST /v1/session/new），
//     新网页端与新桌面端走这条路；
//   - UUID v4 规范格式：历史版本客户端（已发布的桌面 EXE、旧浏览器
//     localStorage）本地生成的 ID，过渡期内继续接受，避免强制升级。
//
// 两类 ID 都不可枚举，"客户端指定任意字符串"的越权面被关闭。后续桌面端
// 全量升级后可收紧为只接受 sess_ 前缀（一行正则的事）。
package server

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"regexp"
)

const issuedSessionPrefix = "sess_"

var (
	// reIssuedSession 服务端签发：sess_ + 128bit 随机十六进制
	reIssuedSession = regexp.MustCompile(`^sess_[0-9a-f]{32}$`)
	// reUUIDv4 历史客户端本地生成的 UUID v4（version=4、variant=10xx）
	reUUIDv4 = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
)

// validSessionID 校验会话标识：只接受服务端签发格式或历史 UUID v4，
// 其余一律拒绝（"default"、顺序号、自造串全部挡在创建会话之前）
func validSessionID(id string) bool {
	return reIssuedSession.MatchString(id) || reUUIDv4.MatchString(id)
}

// newSessionResponse 会话签发响应
type newSessionResponse struct {
	SessionID string `json:"session_id"`
}

// handleNewSession 签发一个新的服务端会话标识。
// 会话实体仍惰性创建（首次 chat 才在 ai.Manager 落位），
// 这里只发 ID，因此接口极廉价，成本由按 IP 限流兜底。
func (s *Server) handleNewSession(w http.ResponseWriter, _ *http.Request) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand 读失败只可能是系统熵源异常，极小概率
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "会话初始化失败，请稍后再试"})
		return
	}
	writeJSON(w, http.StatusOK, newSessionResponse{
		SessionID: issuedSessionPrefix + hex.EncodeToString(b[:]),
	})
}
