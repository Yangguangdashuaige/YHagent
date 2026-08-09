package config

import (
	"errors"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

// Config 全局配置/密钥结构体，所有敏感配置统一在这里管理
type Config struct {
	ArkAPIKey     string // 火山方舟API密钥
	ModelEndPoint string // 模型接入点ID
	ArkBaseUrl    string // 方舟服务地址
}

// Global 全局配置实例，加载后供全项目调用
var Global *Config

// Load 加载配置：
// - 本地开发：自动读取项目根目录的.env文件
// - 生产环境：自动跳过.env，直接读取系统环境变量
func Load() (*Config, error) {
	// 尝试加载本地.env，文件不存在不报错（适配生产环境）
	_ = godotenv.Load()

	cfg := &Config{
		ArkAPIKey:     strings.TrimSpace(os.Getenv("ARK_API_KEY")),
		ModelEndPoint: strings.TrimSpace(os.Getenv("ARK_MODEL_ENDPOINT")),
		ArkBaseUrl:    strings.TrimSpace(os.Getenv("ARK_BASE_URL")),
	}

	// 给BaseUrl设置默认值，用户不需要改就能用
	if cfg.ArkBaseUrl == "" {
		cfg.ArkBaseUrl = "https://ark.cn-beijing.volces.com/api/v3"
	}

	// 启动时强制校验必填密钥，避免调用API才发现没配置
	if cfg.ArkAPIKey == "" || strings.HasPrefix(cfg.ArkAPIKey, "请替换") {
		return nil, errors.New("缺失ARK_API_KEY：请在.env中填写真实方舟API Key，或在系统环境变量中设置")
	}
	if cfg.ModelEndPoint == "" || strings.HasPrefix(cfg.ModelEndPoint, "请替换") {
		return nil, errors.New("缺失ARK_MODEL_ENDPOINT：请填写真实模型接入点ID")
	}

	Global = cfg
	return cfg, nil
}
