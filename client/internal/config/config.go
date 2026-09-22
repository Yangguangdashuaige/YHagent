// internal/config/config.go
//
// 瘦客户端配置：只包含"去哪里连服务器、用什么令牌"和窗口外观。
// 本模块（yhagent-client）不包含任何业务密钥：豆包 Key、三角洲 Key 只存在于服务端。
package config

import (
	"fmt"
	"os"
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

// Load 从环境变量（本地开发可放 client/.env）加载配置，必填项缺失时直接报错
func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		RemoteBaseURL: strings.TrimRight(strings.TrimSpace(os.Getenv("REMOTE_BASE_URL")), "/"),
		RemoteToken:   strings.TrimSpace(os.Getenv("REMOTE_TOKEN")),

		DefaultWidth:  420,
		DefaultHeight: 650,
	}

	if cfg.RemoteBaseURL == "" {
		return nil, fmt.Errorf("REMOTE_BASE_URL 未配置，请在 client/.env 或系统环境变量中指定远端服务地址")
	}
	if cfg.RemoteToken == "" {
		return nil, fmt.Errorf("REMOTE_TOKEN 未配置，请在 client/.env 或系统环境变量中指定访问令牌")
	}
	return cfg, nil
}
