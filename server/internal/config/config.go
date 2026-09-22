// internal/config/config.go
package config

import (
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	// 你现有的豆包配置
	DoubaoAPIKey   string
	DoubaoModel    string
	DoubaoEndpoint string
	DoubaoBaseURL  string

	// 知识库向量化模型（静态知识库 RAG 用），默认当前方舟唯一在线的多模态向量化模型
	DoubaoEmbeddingModel string

	// 新增三角洲开放平台配置
	DeltaAPIKey  string
	DeltaAPIBase string

	// 悬浮窗配置
	DefaultOpacity float64
	DefaultWidth   int
	DefaultHeight  int
	Hotkey         string
}

var AppConfig *Config

// DefaultDoubaoBaseURL 火山方舟北京区默认 API 地址，ARK_BASE_URL 未配置时兜底
const DefaultDoubaoBaseURL = "https://ark.cn-beijing.volces.com/api/v3"

// DefaultEmbeddingModel 知识库向量化模型：doubao-embedding-vision-251215
// 同时支持纯文本输入（2048维），是 2026 年方舟唯一在线的向量化模型，需在控制台开通。
const DefaultEmbeddingModel = "doubao-embedding-vision-251215"

// getenvOr 依次读取多个环境变量名，返回第一个非空值（兼容不同的.env命名习惯，不改变Config结构）
func getenvOr(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}

func Load() error {
	// 加载.env文件，和你现有逻辑一致
	_ = godotenv.Load()

	AppConfig = &Config{
		// 豆包配置：优先 DOUBAO_*，兼容 .env 中实际使用的 ARK_* 命名
		DoubaoAPIKey:   getenvOr("DOUBAO_API_KEY", "ARK_API_KEY"),
		DoubaoModel:    getenvOr("DOUBAO_MODEL", "ARK_MODEL_ENDPOINT"),
		DoubaoEndpoint: getenvOr("DOUBAO_ENDPOINT", "ARK_MODEL_ENDPOINT"),
		// API 基础地址：读 ARK_BASE_URL / DOUBAO_BASE_URL，留空则用北京区默认地址
		DoubaoBaseURL: getenvOr("DOUBAO_BASE_URL", "ARK_BASE_URL"),

		// 向量化模型：可通过 ARK_EMBEDDING_MODEL 覆盖，默认 doubao-embedding-vision-251215
		DoubaoEmbeddingModel: getenvOr("ARK_EMBEDDING_MODEL", "DOUBAO_EMBEDDING_MODEL"),

		// 三角洲API配置
		DeltaAPIKey:  os.Getenv("DELTA_API_KEY"),
		DeltaAPIBase: os.Getenv("DELTA_API_BASE"),

		// 默认UI配置
		DefaultOpacity: 0.8,
		DefaultWidth:   420,
		DefaultHeight:  650,
		Hotkey:         "Ctrl+`",
	}

	// ARK_BASE_URL 未配置时使用北京区默认地址，保证与改造前行为一致
	if AppConfig.DoubaoBaseURL == "" {
		AppConfig.DoubaoBaseURL = DefaultDoubaoBaseURL
	}
	if AppConfig.DoubaoEmbeddingModel == "" {
		AppConfig.DoubaoEmbeddingModel = DefaultEmbeddingModel
	}
	return nil
}
