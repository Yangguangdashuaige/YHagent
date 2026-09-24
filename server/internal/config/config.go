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

	// 知识库向量化模型接入点（静态知识库 RAG 用）：
	// 火山方舟 doubao-embedding-vision 接入点 ID（ep- 开头），文本/图片共用同一向量空间
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

// DefaultEmbeddingModel 向量化接入点的兜底值。
// 推荐在 .env 中用 ARK_EMBEDDING_MODEL 配置自己的 doubao-embedding-vision
// 在线推理接入点 ID（ep- 开头），建库与线上查询必须走同一接入点。
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

		// 向量化模型接入点：ARK_EMBEDDING_MODEL 指定 doubao-embedding-vision 的 ep- 接入点
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
