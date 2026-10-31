// internal/server/guard.go
//
// 服务端入口保护：
//   - chatGate：全局并发闸门（带界排队）。给 /v1/chat 的在途请求设硬上限，
//     防止异常流量打爆上游 RPM 限额与本机内存；满员后新请求不再立刻 503，
//     而是先到先得排队等待（上限 waitLimit），把瞬时尖峰削成平滑曲线；
//   - ipLimiter：按来源 IP 的每分钟固定窗口计数（含会话签发/重置接口），
//     抬高频刷接口与批量撞库 sessionID 的成本。
//
// 两者都是 fail-closed 的廉价本地实现（标准库、无外部依赖，建于单二进制约束时期——
// 该约束已于 2026-10-30 解除）；多实例部署时需平滑替换为 Redis 令牌桶（本地计数在
// 多实例间不共享，限流会按实例数放大）。
package server

import (
	"context"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// defaultMaxConcurrentChat 未配置时的全局对话并发上限。
	// 每路在途对话 = 1 个 goroutine + 1 条 SSE 流，本机开销极小，真正的约束是
	// 上游 LLM 的 RPM/并发配额。128 路可吸收数百人同时提问的尖峰（超出部分排队），
	// 上游配额紧张时用 CHAT_MAX_CONCURRENT 下调。
	defaultMaxConcurrentChat = 128
	// defaultChatRPMPerIP 单 IP 每分钟最多请求次数（chat / session/new / reset 共用窗口）。
	defaultChatRPMPerIP = 30
	// defaultChatQueueWait 满员后排队等待上限，0 = 不限时（默认）。
	// 排队时长交给用户决定：网页端「停止排队」随时退出（断连即让位）；
	// 需要服务端强制兜底时用 CHAT_QUEUE_TIMEOUT 配置秒数上限。
	defaultChatQueueWait = 0
)

// chatGate 全局并发闸门：带缓冲通道即信号量。
// 名额充足时 tryAcquire 零等待直接放行；满员后 acquire 排队等待
// （Go channel 对阻塞发送方天然 FIFO，先到先得），等待期内客户端断连
// 立即让位，超过 waitLimit 返回 false 由调用方拒绝。
type chatGate struct {
	sem       chan struct{}
	waitLimit time.Duration
	waiting   atomic.Int64 // 当前排队中的请求数（含自己），用于推送排队位置
}

func newChatGate(maxConcurrent int, queueWait time.Duration) *chatGate {
	if maxConcurrent <= 0 {
		maxConcurrent = defaultMaxConcurrentChat
	}
	if queueWait < 0 {
		queueWait = defaultChatQueueWait
	}
	return &chatGate{
		sem:       make(chan struct{}, maxConcurrent),
		waitLimit: queueWait,
	}
}

// tryAcquire 非阻塞抢占一个对话名额，满了返回 false（快路径）
func (g *chatGate) tryAcquire() bool {
	select {
	case g.sem <- struct{}{}:
		return true
	default:
		return false
	}
}

// acquire 排队等待名额（慢路径）：waitLimit>0 时最多等该时长（nil 定时器通道
// 表示不限时等待）；入队时与之后每 5s 各回调一次 onWait（参数为当前排队人数，
// 含自己）供调用方推送排队位置；ctx 取消（客户端点「停止排队」断连）立即返回
// false。返回 true 表示拿到名额。
func (g *chatGate) acquire(ctx context.Context, onWait func(waiting int64)) bool {
	g.waiting.Add(1)
	defer g.waiting.Add(-1)
	if onWait != nil {
		onWait(g.waiting.Load()) // 入队即推一次位置，前端不用等首个 tick
	}
	var timeout <-chan time.Time
	if g.waitLimit > 0 {
		timer := time.NewTimer(g.waitLimit)
		defer timer.Stop()
		timeout = timer.C
	}
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	for {
		select {
		case g.sem <- struct{}{}:
			return true
		case <-timeout:
			return false
		case <-ctx.Done():
			return false
		case <-tick.C:
			if onWait != nil {
				onWait(g.waiting.Load())
			}
		}
	}
}

// release 归还名额（与 tryAcquire/acquire 严格配对，建议 defer）
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
