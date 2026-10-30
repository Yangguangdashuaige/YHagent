// internal/auth/auth.go
//
// 账号系统：用户注册/登录 + 访问令牌 + 会话归属。
//
// 定位（分层记忆三步走的第一步）：
//   - UID 是后续长期记忆的隔离维度（用户画像、情景记忆都按 UID 存储）；
//   - 会话归属（sessionID → UID）关闭"拿到 sessionID 就能读别人对话"的越权面，
//     使服务端签发的高熵 ID 之外多一层身份绑定；
//   - 现有"主令牌"（SERVER_TOKEN）客户端完全不受影响：主令牌请求保持匿名身份
//     （uid=0），其创建的会话无归属，行为与升级前一致。
//
// 存储：SQLite（modernc.org/sqlite 纯 Go，无 CGO，交叉编译流程不变）。
// 三张表：users（密码 bcrypt 哈希）、auth_tokens（只存令牌 SHA-256，泄露数据库
// 也无法伪造令牌）、user_sessions（会话归属）。
//
// 防爆破：按用户名的失败锁定（连续 5 次 → 锁 15 分钟，进程内存实现）；
// 外层另有 server 层按 IP 限流兜底。
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

// 预定义错误：HTTP 层据此映射状态码与提示，不透传内部细节
var (
	ErrRegisterClosed  = errors.New("注册通道已关闭")
	ErrUserExists      = errors.New("用户名已被注册")
	ErrBadUsername     = errors.New("用户名不合法")
	ErrWeakPassword    = errors.New("密码强度不足")
	ErrBadCredentials  = errors.New("用户名或密码错误")
	ErrLocked          = errors.New("失败次数过多，账号已临时锁定")
)

const (
	bcryptCost    = 10
	lockThreshold = 5              // 连续失败 N 次锁定
	lockDuration  = 15 * time.Minute
	defaultTokenTTL = 30 * 24 * time.Hour
)

// User 账号信息（不含敏感字段）
type User struct {
	ID        int64
	Username  string
	CreatedAt time.Time
}

// Service 账号服务：SQLite 存储 + 进程内失败锁定。并发安全。
type Service struct {
	db            *sql.DB
	tokenTTL      time.Duration
	allowRegister bool

	mu       sync.Mutex
	failures map[string]*failRecord // username_norm → 失败记录
}

type failRecord struct {
	count int
	until time.Time // 非零表示已锁定到此时刻
}

// Open 打开（或初始化）账号数据库。dbPath 的父目录不存在时自动创建。
// tokenTTL 为 0 时使用默认 30 天（负值原样生效，用于测试瞬时过期）；
// allowRegister=false 时注册接口拒绝新用户。
func Open(dbPath string, tokenTTL time.Duration, allowRegister bool) (*Service, error) {
	if dir := filepath.Dir(dbPath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("创建账号数据库目录: %w", err)
		}
	}
	dsn := "file:" + filepath.ToSlash(dbPath) + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("打开账号数据库: %w", err)
	}
	// SQLite 单写者：单连接彻底规避 SQLITE_BUSY，账号流量微小，串行无感知
	db.SetMaxOpenConns(1)
	s := &Service{
		db:            db,
		tokenTTL:      tokenTTL,
		allowRegister: allowRegister,
		failures:      map[string]*failRecord{},
	}
	if s.tokenTTL == 0 {
		s.tokenTTL = defaultTokenTTL
	}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	go s.cleanupLoop()
	return s, nil
}

// migrate 建表（幂等）。时间为 Unix 秒（整数比较简单，无时区歧义）。
func (s *Service) migrate() error {
	const ddl = `
CREATE TABLE IF NOT EXISTS users (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	username      TEXT NOT NULL,
	username_norm TEXT NOT NULL UNIQUE,
	password_hash TEXT NOT NULL,
	created_at    INTEGER NOT NULL,
	updated_at    INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS auth_tokens (
	token_hash  TEXT PRIMARY KEY,
	user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	created_at  INTEGER NOT NULL,
	expires_at  INTEGER NOT NULL,
	last_used_at INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_auth_tokens_user ON auth_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_auth_tokens_expires ON auth_tokens(expires_at);
CREATE TABLE IF NOT EXISTS user_sessions (
	session_id TEXT PRIMARY KEY,
	user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_user_sessions_user ON user_sessions(user_id);
`
	if _, err := s.db.Exec(ddl); err != nil {
		return fmt.Errorf("初始化账号表: %w", err)
	}
	// 顺带清掉已过期令牌（下次定时清理前先减一次库存）
	if _, err := s.db.Exec(`DELETE FROM auth_tokens WHERE expires_at < ?`, time.Now().Unix()); err != nil {
		return fmt.Errorf("清理过期令牌: %w", err)
	}
	return nil
}

// Close 关闭底层连接（与 Open 配对；清理协程随进程退出）
func (s *Service) Close() error {
	return s.db.Close()
}

// cleanupLoop 每小时清理过期令牌与过期锁定记录（守护协程，随进程退出）
func (s *Service) cleanupLoop() {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for range t.C {
		if _, err := s.db.Exec(`DELETE FROM auth_tokens WHERE expires_at < ?`, time.Now().Unix()); err != nil {
			// 清理失败不影响服务，下个周期再试
			continue
		}
		s.mu.Lock()
		now := time.Now()
		for name, rec := range s.failures {
			if rec.until.IsZero() && rec.count == 0 {
				delete(s.failures, name)
			} else if !rec.until.IsZero() && now.After(rec.until) {
				delete(s.failures, name)
			}
		}
		s.mu.Unlock()
	}
}

// ---- 校验 ----

// validateUsername 允许 3~32 个字符：字母（含中文）/数字/下划线/短横线/点号
func validateUsername(name string) error {
	n := utf8.RuneCountInString(name)
	if n < 3 || n > 32 {
		return ErrBadUsername
	}
	for _, r := range name {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), r == '_', r == '-', r == '.':
		default:
			return ErrBadUsername
		}
	}
	return nil
}

// validatePassword 长度 8~72 字节（bcrypt 只取前 72 字节，超长部分静默截断，故直接拒绝）
func validatePassword(pw string) error {
	if l := len(pw); l < 8 || l > 72 {
		return ErrWeakPassword
	}
	return nil
}

// ---- 注册 / 登录 ----

// Register 注册并直接签发登录令牌（注册即登录，减少一次往返）
func (s *Service) Register(username, password string) (User, string, time.Time, error) {
	var u User
	if !s.allowRegister {
		return u, "", time.Time{}, ErrRegisterClosed
	}
	if err := validateUsername(username); err != nil {
		return u, "", time.Time{}, err
	}
	if err := validatePassword(password); err != nil {
		return u, "", time.Time{}, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return u, "", time.Time{}, fmt.Errorf("密码哈希失败: %w", err)
	}
	now := time.Now()
	norm := strings.ToLower(username)
	res, err := s.db.Exec(
		`INSERT INTO users (username, username_norm, password_hash, created_at, updated_at) VALUES (?,?,?,?,?)`,
		username, norm, string(hash), now.Unix(), now.Unix())
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return u, "", time.Time{}, ErrUserExists
		}
		return u, "", time.Time{}, fmt.Errorf("创建用户: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return u, "", time.Time{}, fmt.Errorf("获取用户ID: %w", err)
	}
	u = User{ID: id, Username: username, CreatedAt: now}
	token, exp, err := s.issueToken(id)
	if err != nil {
		return u, "", time.Time{}, err
	}
	return u, token, exp, nil
}

// Login 校验密码并签发令牌。失败计入锁定计数；锁定期内直接拒绝。
func (s *Service) Login(username, password string) (User, string, time.Time, error) {
	var u User
	norm := strings.ToLower(strings.TrimSpace(username))
	if norm == "" {
		return u, "", time.Time{}, ErrBadCredentials
	}
	if err := s.checkLocked(norm); err != nil {
		return u, "", time.Time{}, err
	}
	var (
		id        int64
		name      string
		hash      string
		createdAt int64
	)
	err := s.db.QueryRow(
		`SELECT id, username, password_hash, created_at FROM users WHERE username_norm = ?`, norm,
	).Scan(&id, &name, &hash, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		s.noteFailure(norm)
		// 不区分"用户不存在/密码错误"，统一模糊提示
		return u, "", time.Time{}, ErrBadCredentials
	}
	if err != nil {
		return u, "", time.Time{}, fmt.Errorf("查询用户: %w", err)
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		s.noteFailure(norm)
		return u, "", time.Time{}, ErrBadCredentials
	}
	s.clearFailure(norm)
	u = User{ID: id, Username: name, CreatedAt: time.Unix(createdAt, 0)}
	token, exp, err := s.issueToken(id)
	if err != nil {
		return u, "", time.Time{}, err
	}
	return u, token, exp, nil
}

// issueToken 生成 256bit 随机令牌（明文只出现一次，落库的是 SHA-256）
func (s *Service) issueToken(userID int64) (string, time.Time, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", time.Time{}, fmt.Errorf("生成令牌失败: %w", err)
	}
	token := hex.EncodeToString(raw[:])
	sum := sha256.Sum256([]byte(token))
	now := time.Now()
	exp := now.Add(s.tokenTTL)
	if _, err := s.db.Exec(
		`INSERT INTO auth_tokens (token_hash, user_id, created_at, expires_at, last_used_at) VALUES (?,?,?,?,?)`,
		hex.EncodeToString(sum[:]), userID, now.Unix(), exp.Unix(), now.Unix()); err != nil {
		return "", time.Time{}, fmt.Errorf("写入令牌: %w", err)
	}
	return token, exp, nil
}

// VerifyToken 校验访问令牌，返回所属用户。无效/过期返回 ok=false（不区分原因）。
func (s *Service) VerifyToken(token string) (User, bool) {
	var u User
	if len(token) < 16 {
		return u, false
	}
	sum := sha256.Sum256([]byte(token))
	var (
		uid       int64
		name      string
		createdAt int64
	)
	err := s.db.QueryRow(
		`SELECT u.id, u.username, u.created_at FROM auth_tokens t JOIN users u ON u.id = t.user_id
		 WHERE t.token_hash = ? AND t.expires_at >= ?`,
		hex.EncodeToString(sum[:]), time.Now().Unix(),
	).Scan(&uid, &name, &createdAt)
	if err != nil {
		return u, false
	}
	// 刷新使用时间（审计用，失败不影响校验）
	s.db.Exec(`UPDATE auth_tokens SET last_used_at = ? WHERE token_hash = ?`, time.Now().Unix(), hex.EncodeToString(sum[:]))
	return User{ID: uid, Username: name, CreatedAt: time.Unix(createdAt, 0)}, true
}

// Logout 注销令牌（幂等：令牌不存在也返回成功）
func (s *Service) Logout(token string) {
	sum := sha256.Sum256([]byte(token))
	s.db.Exec(`DELETE FROM auth_tokens WHERE token_hash = ?`, hex.EncodeToString(sum[:]))
}

// ---- 会话归属 ----

// BindSession 把服务端签发的 sessionID 归属到用户（重复绑定以先到先得，防"抢注"他人会话）
func (s *Service) BindSession(sessionID string, userID int64) error {
	if _, err := s.db.Exec(
		`INSERT INTO user_sessions (session_id, user_id, created_at) VALUES (?,?,?)
		 ON CONFLICT(session_id) DO NOTHING`,
		sessionID, userID, time.Now().Unix()); err != nil {
		return fmt.Errorf("绑定会话归属: %w", err)
	}
	return nil
}

// SessionOwner 查询会话归属。ok=false 表示无归属（主令牌客户端的历史/匿名会话）
func (s *Service) SessionOwner(sessionID string) (int64, bool) {
	var uid int64
	err := s.db.QueryRow(`SELECT user_id FROM user_sessions WHERE session_id = ?`, sessionID).Scan(&uid)
	if err != nil {
		return 0, false
	}
	return uid, true
}

// ---- 登录失败锁定 ----

func (s *Service) checkLocked(norm string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.failures[norm]
	if !ok {
		return nil
	}
	if !rec.until.IsZero() && time.Now().Before(rec.until) {
		return ErrLocked
	}
	if !rec.until.IsZero() && time.Now().After(rec.until) {
		delete(s.failures, norm) // 锁定到期，重新计
	}
	return nil
}

func (s *Service) noteFailure(norm string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.failures) > 10_000 { // 防御性上限：极端场景下丢弃最早记录而非吃光内存
		s.failures = map[string]*failRecord{}
	}
	rec := s.failures[norm]
	if rec == nil {
		rec = &failRecord{}
		s.failures[norm] = rec
	}
	rec.count++
	if rec.count >= lockThreshold {
		rec.until = time.Now().Add(lockDuration)
	}
}

func (s *Service) clearFailure(norm string) {
	s.mu.Lock()
	delete(s.failures, norm)
	s.mu.Unlock()
}
