// internal/config/config.go
//
// 瘦客户端配置：只包含"去哪里连服务器、用什么令牌"和窗口外观。
// 本模块（yhagent-client）不包含任何业务密钥：豆包 Key、三角洲 Key 只存在于服务端。
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
)

// Config 桌面端运行所需的全部配置
type Config struct {
	// RemoteBaseURL 远端 Agent 服务地址，如 https://ai.example.com
	RemoteBaseURL string
	// RemoteToken 访问令牌，必须与服务端 SERVER_TOKEN 一致
	RemoteToken string

	// 悬浮窗默认尺寸
	DefaultWidth  int
	DefaultHeight int
}

// Load 加载配置：按优先级依次尝试 exe 同目录、当前工作目录的 .env
// （双击运行时工作目录是 exe 所在目录，但用户可能从快捷方式/其他目录启动，
// 两处都找才能保证单文件发布开箱即用），必填项缺失时直接报错
func Load() (*Config, error) {
	for _, p := range envPaths() {
		_ = godotenv.Load(p)
	}

	cfg := &Config{
		RemoteBaseURL: strings.TrimRight(strings.TrimSpace(os.Getenv("REMOTE_BASE_URL")), "/"),
		RemoteToken:   strings.TrimSpace(os.Getenv("REMOTE_TOKEN")),

		// 悬浮窗默认尺寸：稍放大提升阅读体验（可在前端自由缩放，Min 360x480 兜底）
		DefaultWidth:  460,
		DefaultHeight: 700,
	}

	if cfg.RemoteBaseURL == "" {
		return nil, fmt.Errorf("REMOTE_BASE_URL 未配置，请在 client/.env 或系统环境变量中指定远端服务地址")
	}
	if cfg.RemoteToken == "" {
		return nil, fmt.Errorf("REMOTE_TOKEN 未配置，请在 exe 同目录 .env 或系统环境变量中指定访问令牌")
	}
	return cfg, nil
}

// envPaths .env 候选路径：exe 同目录优先（双击/快捷方式启动的工作目录不确定），
// 其次当前工作目录（go run / wails dev 开发态）
func envPaths() []string {
	var paths []string
	if exe, err := os.Executable(); err == nil {
		paths = append(paths, filepath.Join(filepath.Dir(exe), ".env"))
	}
	paths = append(paths, ".env")
	return paths
}
