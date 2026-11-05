// internal/eventlog/alerter.go
//
// 异常模式检测器：订阅 eventlog 的事件流，识别两类需要人工介入的信号，
// 产出 alert.* 事件（默认只落事件日志；配置 ALERT_WEBHOOK 后同步外推）：
//
//  1. 重启抖动（alert.restart_flapping）：短时间窗口内 service.restart_detected
//     达到阈值——通常意味着服务陷入崩溃循环（坏部署/上游全挂/OOM），需立即排查。
//  2. 上游连续失败（alert.upstream_failing）：连续 upstream.error 达到阈值——
//     豆包侧或网络大概率异常，期间用户体验断崖式下跌。
//
// 设计要点：
//   - 告警判定在 Emit 调用栈内同步完成（轻量计数），但 alert.* 事件的落盘与
//     webhook 外推走低优先级路径，绝不拖慢业务埋点；
//   - webhook 失败只记 stderr，不重试（避免告警风暴把事件流堵死）；
//   - 阈值/窗口均可由环境变量覆盖，默认值偏保守（宁可不报也不误报刷屏）。
package eventlog

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

// 事件类型常量（Agent / main 埋点与告警器观测共用，避免字符串漂移）
const (
	EvtServiceStart          = "service.start"
	EvtServiceShutdown       = "service.shutdown"
	EvtServiceRestartDetect  = "service.restart_detected"
	EvtAgentTurnBegin        = "agent.turn_begin"
	EvtAgentTurnEnd          = "agent.turn_end"
	EvtAgentTurnCrashed      = "agent.turn_crashed"
	EvtAgentTurnInterrupted  = "agent.turn_interrupted"
	EvtUpstreamError         = "upstream.error"
	EvtUpstreamStale         = "upstream.stale"
	EvtRecoveryResumed       = "recovery.resumed"
	EvtRecoveryCompleted     = "recovery.completed"
	EvtAlertRestartFlapping  = "alert.restart_flapping"
	EvtAlertUpstreamFailing  = "alert.upstream_failing"
)

// 默认告警阈值（环境变量可覆盖）
const (
	defaultRestartWindow    = 5 * time.Minute
	defaultRestartThreshold = 3
	defaultUpstreamThreshold = 5
	webhookTimeout          = 5 * time.Second
)

// alerter 观测事件流并产出 alert.* 事件。方法线程安全。
type alerter struct {
	rec *Recorder

	mu                  sync.Mutex
	restartWindow       time.Duration
	restartThreshold    int
	upstreamThreshold   int
	restarts            []time.Time // 窗口内的异常重启时间点
	consecutiveUpstream int         // 当前连续 upstream.error 计数
	webhook             string      // 外推 URL，空 = 仅落日志
	alertCooldown       map[string]time.Time // 同类告警的冷却截止，防刷屏
}

// newAlerter 从环境变量构造告警器（main 装配时调用）
func newAlerter(rec *Recorder) *alerter {
	a := &alerter{
		rec:               rec,
		restartWindow:     defaultRestartWindow,
		restartThreshold:  defaultRestartThreshold,
		upstreamThreshold: defaultUpstreamThreshold,
		webhook:           os.Getenv("ALERT_WEBHOOK"),
		alertCooldown:     make(map[string]time.Time),
	}
	if v := os.Getenv("ALERT_RESTART_WINDOW"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			a.restartWindow = d
		}
	}
	if v := os.Getenv("ALERT_RESTART_THRESHOLD"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			a.restartThreshold = n
		}
	}
	if v := os.Getenv("ALERT_UPSTREAM_THRESHOLD"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			a.upstreamThreshold = n
		}
	}
	return a
}

// EnableAlerts 为记录器挂上异常模式检测（main 启动时调用一次）
func (r *Recorder) EnableAlerts() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.alerter == nil {
		r.alerter = newAlerter(r)
	}
}

// observe 处理一条事件（由 Recorder.EmitWith 在落盘后调用，不持 Recorder 锁）
func (a *alerter) observe(e Event) {
	if a == nil {
		return
	}
	now := time.Now()
	switch e.Event {
	case EvtServiceRestartDetect:
		a.mu.Lock()
		a.restarts = append(a.restarts, now)
		// 只保留窗口内的时间点
		cutoff := now.Add(-a.restartWindow)
		idx := 0
		for idx < len(a.restarts) && a.restarts[idx].Before(cutoff) {
			idx++
		}
		a.restarts = a.restarts[idx:]
		count := len(a.restarts)
		thr := a.restartThreshold
		a.mu.Unlock()
		if count >= thr {
			a.fire(now, EvtAlertRestartFlapping, map[string]any{
				"count":   count,
				"window":  a.restartWindow.String(),
				"message": "短时间内异常重启次数达到阈值，服务可能陷入崩溃循环",
			})
		}
	case EvtUpstreamError:
		a.mu.Lock()
		a.consecutiveUpstream++
		count := a.consecutiveUpstream
		thr := a.upstreamThreshold
		a.mu.Unlock()
		if count >= thr {
			a.fire(now, EvtAlertUpstreamFailing, map[string]any{
				"count":   count,
				"message": "上游连续失败次数达到阈值，豆包服务或网络可能异常",
			})
		}
	case EvtAgentTurnEnd:
		// 一轮成功结束 → 上游连续失败计数清零（ok=true 才算恢复）
		if ok, _ := e.Fields["ok"].(bool); ok {
			a.mu.Lock()
			a.consecutiveUpstream = 0
			a.mu.Unlock()
		}
	}
}

// fire 产出一条 alert.* 事件：落盘 + 冷却内不重复 + webhook 外推（异步）
func (a *alerter) fire(now time.Time, event string, fields map[string]any) {
	a.mu.Lock()
	if until, ok := a.alertCooldown[event]; ok && now.Before(until) {
		a.mu.Unlock()
		return // 冷却期内，同类告警只报一次
	}
	a.alertCooldown[event] = now.Add(10 * time.Minute) // 同类告警 10 分钟冷却
	wh := a.webhook
	a.mu.Unlock()

	if a.rec != nil {
		a.rec.writeAlert(event, fields)
	}
	if wh != "" {
		go a.postWebhook(wh, event, fields, now)
	}
	log.Printf("[eventlog] 告警触发: %s %v", event, fields)
}

// postWebhook 把告警外推到配置的 webhook（best-effort，失败不重试）
func (a *alerter) postWebhook(url, event string, fields map[string]any, now time.Time) {
	payload := map[string]any{
		"ts":     now.Format(time.RFC3339),
		"event":  event,
		"fields": fields,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return
	}
	client := &http.Client{Timeout: webhookTimeout}
	resp, err := client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		log.Printf("[eventlog] 告警 webhook 推送失败: %v", err)
		return
	}
	_ = resp.Body.Close()
}
