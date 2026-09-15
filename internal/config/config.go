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

func Load() error {
	// 加载.env文件，和你现有逻辑一致
	_ = godotenv.Load()

	AppConfig = &Config{
		// 豆包配置从环境变量读
		DoubaoAPIKey:   os.Getenv("DOUBAO_API_KEY"),
		DoubaoModel:    os.Getenv("DOUBAO_MODEL"),
		DoubaoEndpoint: os.Getenv("DOUBAO_ENDPOINT"),

		// 三角洲API配置
		DeltaAPIKey:  os.Getenv("DELTA_API_KEY"),
		DeltaAPIBase: os.Getenv("DELTA_API_BASE"),

		// 默认UI配置
		DefaultOpacity: 0.8,
		DefaultWidth:   420,
		DefaultHeight:  650,
		Hotkey:         "Ctrl+`",
	}
	return nil
}
