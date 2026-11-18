// Package asr Windows SAPI 流式听写（Coopanion 行为规范：子进程 + stdout 流式事件）。
//
// 每次 Start 拉起一个 PowerShell（System.Speech 听写语法）进程：ready →
// partial*（实时假设）→ final（一句定稿）→ done（进程自然退出）。
// 引擎冷启动约 0.3~0.8s（v1 已知延迟，前端以"正在听"气泡缓解；长驻进程列为后续优化）。
package asr

import (
	"bufio"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

//go:embed sapi.ps1
var sapiScript embed.FS

// Event 听写事件（与 sapi.ps1 的 stdout JSON 行一一对应）
type Event struct {
	Type       string  `json:"type"` // ready / partial / final / done
	Text       string  `json:"text,omitempty"`
	Confidence float64 `json:"confidence,omitempty"`
	Reason     string  `json:"reason,omitempty"` // done 的细分：completed/timeout/rejected/cancelled/error
	Detail     string  `json:"detail,omitempty"` // error 时的异常信息
}

// ParseLine 解析一行 stdout JSON 为事件（纯函数，单测覆盖）。
// 空行或非 JSON 行返回错误（调用方按可观测性失败跳过，不阻断流）。
func ParseLine(line string) (Event, error) {
	var e Event
	if err := json.Unmarshal([]byte(line), &e); err != nil {
		return Event{}, fmt.Errorf("听写事件解析失败: %w", err)
	}
	switch e.Type {
	case "ready", "partial", "final", "done":
		return e, nil
	default:
		return Event{}, errors.New("未知听写事件类型: " + e.Type)
	}
}

// Manager SAPI 听写管理器（同一时刻至多一个识别进程）
type Manager struct {
	mu         sync.Mutex
	scriptPath string // 脚本落盘路径（首次 Start 时提取）
	cmd        *exec.Cmd
	// onEvent 事件回调（识别 goroutine 中调用；上层做轻量转发）
	onEvent func(Event)
	// logf 可观测性注入（nil 安全）
	logf func(format string, args ...any)
}

// NewManager 创建听写管理器
func NewManager() *Manager { return &Manager{} }

// OnEvent 注册事件回调（ready/partial/final/done 全量透传）
func (m *Manager) OnEvent(fn func(Event)) { m.onEvent = fn }

// SetLogger 注入日志函数
func (m *Manager) SetLogger(fn func(format string, args ...any)) { m.logf = fn }

func (m *Manager) printf(format string, args ...any) {
	if m.logf != nil {
		m.logf(format, args...)
	}
}

// Start 拉起一次听写进程；上一进程仍在时先终止（PTT 重复触发兜底）。
// 返回错误仅代表"进程没拉起来"，识别过程中的失败经 done 事件上报。
func (m *Manager) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.ensureScript(); err != nil {
		return err
	}
	m.killLocked(0)

	cmd := exec.Command("powershell", "-NoProfile", "-STA", "-ExecutionPolicy", "Bypass",
		"-File", m.scriptPath)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("听写进程管道创建失败: %w", err)
	}
	// stderr 丢弃：SAPI 的非致命告警会污染协议流之外的输出
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("听写进程启动失败（缺少 PowerShell 或 System.Speech）: %w", err)
	}
	m.cmd = cmd
	m.printf("[asr] 听写进程已启动 pid=%d", cmd.Process.Pid)

	go m.readLoop(stdout)
	return nil
}

// Stop 结束当前听写：先给 600ms 自然收尾（句尾定稿已 done 的进程自己退出），
// 仍在说话则强杀（丢弃未定稿的 partial）。
func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.killLocked(600 * time.Millisecond)
}

// killLocked 终止进程（调用方持锁）。grace>0 时先等自然退出再杀。
func (m *Manager) killLocked(grace time.Duration) {
	if m.cmd == nil || m.cmd.Process == nil {
		return
	}
	cmd := m.cmd
	m.cmd = nil
	if grace > 0 {
		done := make(chan struct{})
		go func() { _ = cmd.Wait(); close(done) }()
		select {
		case <-done:
			return
		case <-time.After(grace):
		}
	}
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
}

// readLoop 消费听写进程的 stdout JSON 行（识别 goroutine）
func (m *Manager) readLoop(stdout io.Reader) {
	sc := bufio.NewScanner(stdout)
	for sc.Scan() {
		e, err := ParseLine(sc.Text())
		if err != nil {
			m.printf("[asr] %v", err)
			continue
		}
		if m.onEvent != nil {
			m.onEvent(e)
		}
	}
}

// ensureScript 把 embed 的脚本落盘到临时目录（一次即可；调用方持锁）
func (m *Manager) ensureScript() error {
	if m.scriptPath != "" {
		if _, err := os.Stat(m.scriptPath); err == nil {
			return nil
		}
	}
	data, err := sapiScript.ReadFile("sapi.ps1")
	if err != nil {
		return fmt.Errorf("内嵌听写脚本读取失败: %w", err)
	}
	dir := os.TempDir()
	p := filepath.Join(dir, "yhagent-sapi.ps1")
	if err := os.WriteFile(p, data, 0o644); err != nil {
		return fmt.Errorf("听写脚本落盘失败: %w", err)
	}
	m.scriptPath = p
	return nil
}
