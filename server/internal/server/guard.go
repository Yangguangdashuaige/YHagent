// internal/server/guard.go
//
// 服务端入口保护：
//   - chatGate：全局限流并发闸门。LLM 调用按并发线性烧钱，单实例必须给
//     /v1/chat 的在途请求设硬上限，防止异常流量打爆上游 RPM 限额与本机内存；
//   - ipLimiter：按来源 IP 的每分钟固定窗口计数（含会话签发/重置接口），
//     抬高频刷接口与批量撞库 sessionID 的成本。
//
// 两者都是 fail-closed 的廉价本地实现（标准库、无外部依赖，建于单二进制约束时期——
// 该约束已于 2026-10-30 解除）；多实例部署时需平滑替换为 Redis 令牌桶（本地计数在
// 多实例间不共享，限流会按实例数放大）。
package server

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	// defaultMaxConcurrentChat 未配置时的全局对话并发上限。
	// 单轮对话平均占用 LLM 连接数秒到数十秒，16 并发对万级日活的问答类
	// 流量足够（请求排队在用户侧表现为短暂等待），同时把成本上限钉死。
	defaultMaxConcurrentChat = 16
	// defaultChatRPMPerIP 单 IP 每分钟最多请求次数（chat / session/new / reset 共用窗口）。
	defaultChatRPMPerIP = 30
)

// chatGate 全局并发闸门：带缓冲通道即信号量，取空位非阻塞、满了立刻 503
type chatGate struct {
	sem chan struct{}
}

func newChatGate(maxConcurrent int) *chatGate {
	if maxConcurrent <= 0 {
		maxConcurrent = defaultMaxConcurrentChat
	}
	return &chatGate{sem: make(chan struct{}, maxConcurrent)}
}

// tryAcquire 非阻塞抢占一个对话名额，满了返回 false
func (g *chatGate) tryAcquire() bool {
	select {
	case g.sem <- struct{}{}:
		return true
	default:
		return false
	}
}

// release 归还名额（与 tryAcquire 严格配对，建议 defer）
func (g *chatGate) release() {
	<-g.sem
}

// ipLimiter 按 IP 的每分钟固定窗口限流器。
// 固定窗口的边界突刺对本场景无害（窗口仅 60s、阈值宽松），换来零清扫
// goroutine：跨分钟时整张计数表惰性丢弃，旧窗口的 IP 条目随之 GC。
type ipLimiter struct {
	mu     sync.Mutex
	rpm    int
	minute string // 当前窗口标识（yyyyMMddHHmm），变化即重置 counts
	counts map[string]int
}

// newIPLimiter 构造限流器；rpm=0 用默认阈值。调用方在 rpm<0 时不应构造（表示显式关闭）
func newIPLimiter(rpm int) *ipLimiter {
	if rpm == 0 {
		rpm = defaultChatRPMPerIP
	}
	return &ipLimiter{
		rpm:    rpm,
		minute: time.Now().Truncate(time.Minute).Format("200601021504"),
		counts: make(map[string]int),
	}
}

// allow 报告该 IP 本次请求是否放行（本次计数也计入窗口）
func (l *ipLimiter) allow(ip string) bool {
	cur := time.Now().Truncate(time.Minute).Format("200601021504")
	l.mu.Lock()
	defer l.mu.Unlock()
	if cur != l.minute {
		// 进入新的一分钟：整张表丢弃，旧表随 GC 回收，无需逐项过期
		l.minute = cur
		l.counts = make(map[string]int)
	}
	l.counts[ip]++
	return l.counts[ip] <= l.rpm
}

// clientIP 提取请求来源 IP。服务部署在 Nginx 反代之后（见 X-Accel-Buffering
// 相关配置），优先取 X-Forwarded-For 的第一个地址（最初客户端）；
// 直连请求没有该头，回退到 RemoteAddr。注意：直连 :8080 的客户端可伪造
// XFF，故生产环境必须用防火墙保证只能经 Nginx 访问本机端口。
func clientIP(r *http.Request) string {
	if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
		if i := strings.IndexByte(xff, ','); i >= 0 {
			xff = xff[:i] // XFF: client, proxy1, proxy2 —— 第一个才是最初来源
		}
		if ip := strings.TrimSpace(xff); ip != "" {
			return ip
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
