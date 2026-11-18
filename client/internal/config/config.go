// internal/config/config.go
//
// 桌宠瘦客户端配置：只包含"去哪里连服务器、用什么令牌"和桌宠窗口/语音参数。
// 本模块（yhagent-client）不包含任何业务密钥：模型 Key、三角洲 Key 只存在于服务端。
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// Config 桌宠运行所需的全部配置（全量 env 可配，缺省值开箱即用）
type Config struct {
	// RemoteBaseURL 远端 Agent 服务地址，如 https://ai.example.com
	RemoteBaseURL string
	// RemoteToken 访问令牌，必须与服务端 SERVER_TOKEN 一致
	RemoteToken string

	// PetBandHeight 带状窗高度（物理像素）：宠物活动区 + 气泡空间
	PetBandHeight int
	// PetBottomGap 窗口底边离工作区底边间隙（物理像素，Coopanion AUTOHIDE_TASKBAR_GAP=3）
	PetBottomGap int

	// PTTEnabled 按键说话开关（GetAsyncKeyState 轮询左 Alt）
	PTTEnabled bool
	// PTTKeyVK PTT 虚拟键码（默认 0xA4 左 Alt；十进制 env 配置）
	PTTKeyVK int
	// ASRAutoSend 听写结束自动发送（Coopanion 行为：说完即问）；关闭则填入输入框待确认
	ASRAutoSend bool
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

		PetBandHeight: envInt("PET_BAND_HEIGHT", 260),
		PetBottomGap:  envInt("PET_BOTTOM_GAP", 3),

		PTTEnabled:   envBool("PTT_ENABLED", true),
		PTTKeyVK:      envInt("PTT_KEY_VK", 0xA4),
		ASRAutoSend:   envBool("ASR_AUTO_SEND", true),
	}

	if cfg.RemoteBaseURL == "" {
		return nil, fmt.Errorf("REMOTE_BASE_URL 未配置，请在 client/.env 或系统环境变量中指定远端服务地址")
	}
	if cfg.RemoteToken == "" {
		return nil, fmt.Errorf("REMOTE_TOKEN 未配置，请在 exe 同目录 .env 或系统环境变量中指定访问令牌")
	}
	if cfg.PetBandHeight < 120 {
		cfg.PetBandHeight = 120 // 兜底：太矮放不下宠物+气泡
	}
	if cfg.PetBottomGap < 0 {
		cfg.PetBottomGap = 0
	}
	if cfg.PTTKeyVK <= 0 {
		cfg.PTTKeyVK = 0xA4
	}
	return cfg, nil
}

// envInt 读整型 env（非法或缺失回退默认；可观测性失败不阻断启动）
func envInt(key string, def int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

// envBool 读布尔 env（1/true 开，其余关）
func envBool(key string, def bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	return strings.EqualFold(v, "1") || strings.EqualFold(v, "true")
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
