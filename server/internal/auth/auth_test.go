package auth

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "accounts.db"), time.Hour, true)
	if err != nil {
		t.Fatalf("打开测试库: %v", err)
	}
	t.Cleanup(func() { s.db.Close() })
	return s
}

func TestRegisterLoginFlow(t *testing.T) {
	s := newTestService(t)

	// 注册即登录：拿到令牌与过期时间
	u, token, exp, err := s.Register("Alice_01", "password123")
	if err != nil {
		t.Fatalf("注册失败: %v", err)
	}
	if u.ID == 0 || u.Username != "Alice_01" || token == "" || !exp.After(time.Now()) {
		t.Errorf("注册返回不完整: %+v", u)
	}

	// 令牌可验证且归属正确
	got, ok := s.VerifyToken(token)
	if !ok || got.ID != u.ID || got.Username != "Alice_01" {
		t.Errorf("VerifyToken 应命中注册令牌: ok=%v got=%+v", ok, got)
	}

	// 登录：正确密码发新令牌
	u2, token2, _, err := s.Login("alice_01", "password123") // 用户名大小写不敏感
	if err != nil || u2.ID != u.ID {
		t.Fatalf("登录失败: %v", err)
	}
	if _, ok := s.VerifyToken(token2); !ok {
		t.Errorf("登录令牌应有效")
	}

	// 错误密码：模糊错误（不暴露用户是否存在）
	if _, _, _, err := s.Login("alice_01", "wrong-password"); err != ErrBadCredentials {
		t.Errorf("错误密码应返回 ErrBadCredentials，得到 %v", err)
	}
	// 不存在的用户：同一错误
	if _, _, _, err := s.Login("nobody", "password123"); err != ErrBadCredentials {
		t.Errorf("不存在用户应返回 ErrBadCredentials（模糊提示），得到 %v", err)
	}

	// Logout 后令牌失效
	s.Logout(token)
	if _, ok := s.VerifyToken(token); ok {
		t.Errorf("Logout 后令牌应失效")
	}
	if _, ok := s.VerifyToken(token2); !ok {
		t.Errorf("另一令牌不应受影响")
	}
}

func TestRegisterValidation(t *testing.T) {
	s := newTestService(t)
	if _, _, _, err := s.Register("ab", "password123"); err != ErrBadUsername { // 2 字符太短
		t.Errorf("3字符以下应拒绝: %v", err)
	}
	if _, _, _, err := s.Register("有 空格", "password123"); err != ErrBadUsername {
		t.Errorf("含空格应拒绝: %v", err)
	}
	if _, _, _, err := s.Register("正常名", "1234567"); err != ErrWeakPassword { // 密码 7 字节
		t.Errorf("短密码应拒绝: %v", err)
	}
	if _, _, _, err := s.Register("正常名", strings.Repeat("0", 73)); err != ErrWeakPassword {
		t.Errorf("超 72 字节密码应拒绝（bcrypt 截断）: %v", err)
	}
	// 中文用户名合法
	if _, _, _, err := s.Register("大黑鹰", "password123"); err != nil {
		t.Errorf("中文用户名应允许: %v", err)
	}
	// 先注册，再试大小写变体 → 归一后重名拒绝
	if _, _, _, err := s.Register("Alice_01", "password123"); err != nil {
		t.Fatalf("首次注册应成功: %v", err)
	}
	if _, _, _, err := s.Register("alice_01", "password456"); err != ErrUserExists {
		t.Errorf("大小写归一后重名应拒绝: %v", err)
	}
}

func TestRegisterClosed(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "a.db"), 0, false) // TTL 默认 + 注册关闭
	if err != nil {
		t.Fatal(err)
	}
	defer s.db.Close()
	if _, _, _, err := s.Register("someone", "password123"); err != ErrRegisterClosed {
		t.Errorf("注册关闭时应拒绝: %v", err)
	}
}

func TestLoginLockout(t *testing.T) {
	s := newTestService(t)
	if _, _, _, err := s.Register("victim", "password123"); err != nil {
		t.Fatal(err)
	}
	// 连续 5 次错误密码触发锁定
	for i := 0; i < lockThreshold; i++ {
		if _, _, _, err := s.Login("victim", "bad-pass-x"+time.Now().Format("150405")); err != ErrBadCredentials {
			t.Fatalf("第 %d 次应返回凭据错误: %v", i+1, err)
		}
	}
	if _, _, _, err := s.Login("victim", "password123"); err != ErrLocked {
		t.Errorf("锁定期内正确密码也应返回 ErrLocked，得到 %v", err)
	}
}

func TestTokenExpiry(t *testing.T) {
	// TTL=-1s：签发即过期（库内时间为 Unix 秒）
	s, err := Open(filepath.Join(t.TempDir(), "a.db"), -time.Second, true)
	if err != nil {
		t.Fatal(err)
	}
	defer s.db.Close()
	_, token, _, err := s.Register("shorty", "password123")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.VerifyToken(token); ok {
		t.Errorf("过期令牌不应通过验证")
	}
}

func TestSessionOwnership(t *testing.T) {
	s := newTestService(t)
	ua, _, _, _ := s.Register("userA", "password123")
	ub, _, _, _ := s.Register("userB", "password123")

	// 无归属 → ok=false
	if _, ok := s.SessionOwner("sess_none"); ok {
		t.Errorf("未绑定会话应无归属")
	}
	// A 绑定后归属 A；B 抢注失败（先到先得）
	if err := s.BindSession("sess_x", ua.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.BindSession("sess_x", ub.ID); err != nil {
		t.Fatal(err)
	}
	owner, ok := s.SessionOwner("sess_x")
	if !ok || owner != ua.ID {
		t.Errorf("归属应保持先绑定的 A，得到 owner=%d ok=%v", owner, ok)
	}
}
