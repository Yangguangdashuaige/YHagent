// internal/openaicompat/turn.go
//
// 门面回合核心：把一次（或续轮多次）OpenAI 兼容请求跑完 YHagent 的 FC 循环。
// 与 internal/ai/agent.go 的会话态循环刻意分离——两套语义本质不同：
//   - Agent 循环：服务端全权管理会话（持久化/WAL/压缩/反馈）；
//   - 门面循环：  无状态语义，上下文以每次请求携带的 messages 为准（Coopanion
//     自管历史），WAL/压缩/反馈闭环均不挂接（可观测性走 trace/eventlog）。
//
// 客户端工具中继（本文件的核心机制）：
//
//	模型的工具调用按"调用方"分拣——注册在服务端 Registry 的就地执行（客户端
//	无感知），请求 tools 里带来的客户端工具（pet_* 等）以 tool_calls 流回
//	客户端执行。一轮混调时：服务端工具先执行并入影子上下文（保 Ark API 的
//	tool_calls/tool 配对完整），客户端工具调用则重写 id（yhx_<turnID>_<seq>）
//	后作为本次 HTTP 响应输出（finish_reason=tool_calls）。客户端执行完携结果
//	发起下一次请求，门面按 id 前缀唤醒影子回合，合并结果继续循环。
package openaicompat

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/go-faster/jx"
	"github.com/volcengine/ark-runtime-go/arkruntime/model/chat"

	"yhagent-server/internal/ai"
	"yhagent-server/internal/data/deltaapi"
	"yhagent-server/internal/eventlog"
	"yhagent-server/internal/trace"
)

// relayCallIDPrefix 门面生成的客户端工具调用 id 前缀（yhx_<turnID>_<seq>），
// 续轮请求凭此识别"这是我们的影子回合"。
const relayCallIDPrefix = "yhx_"

const (
	// defaultMaxToolRounds 门面循环默认轮数上限（Coopanion 侧还有一层 Cortico
	// 循环兜底，这里的上限防单请求内死循环）
	defaultMaxToolRounds = 8
	// defaultToolExecuteTimeout 服务端工具单次执行超时，与 Agent 循环对齐
	defaultToolExecuteTimeout = 45 * time.Second
	// defaultHeartbeatInterval 工具执行期间的心跳间隔（客户端 SSE 空闲阈值内的双倍余量）
	defaultHeartbeatInterval = 15 * time.Second
	// defaultPendingTTL 影子回合成活期：超时未收到客户端回执即淘汰
	defaultPendingTTL = 10 * time.Minute
	// pendingCleanupInterval 影子回合后台扫描间隔
	pendingCleanupInterval = time.Minute
	// maxToolResultBytes 工具结果回传模型硬上限（与 Agent 循环一致）
	maxToolResultBytes = 32 * 1024
)

// LLMStreamer 模型流式调用抽象（*ai.SeedClient 天然满足此签名；测试注入 fake）。
type LLMStreamer interface {
	ChatStream(
		ctx context.Context,
		history []chat.ChatCompletionRequestMessage,
		tools []chat.ChatCompletionTool,
		onChunk func(content string),
		onToolCall func(calls []chat.ChatCompletionMessageToolCall) error,
		onUsage func(u chat.CompletionUsage),
		onReasoning func(reasoning string),
	) error
}

// Config 门面配置（全部字段有默认值，env 装配层覆盖）。
type Config struct {
	// ModelName 对外暴露的模型名（/models 响应与 chunk.model 字段）
	ModelName string
	// MaxToolRounds 循环轮数上限（<=0 用默认 8）
	MaxToolRounds int
	// ToolExecuteTimeout 服务端工具执行超时（<=0 用默认 45s）
	ToolExecuteTimeout time.Duration
	// HeartbeatInterval 工具执行期间的心跳间隔（<=0 用默认 15s）
	HeartbeatInterval time.Duration
	// PendingTTL 影子回合成活期（<=0 用默认 10 分钟）
	PendingTTL time.Duration
	// PassthroughSystem true=客户端 system 原样透传不融合（调试模式）
	PassthroughSystem bool
	// BaseSystemPrompt 融合基底（空=ai.DefaultSystemPrompt()）
	BaseSystemPrompt string
}

func (c Config) withDefaults() Config {
	if c.ModelName == "" {
		c.ModelName = "yhagent-delta"
	}
	if c.MaxToolRounds <= 0 {
		c.MaxToolRounds = defaultMaxToolRounds
	}
	if c.ToolExecuteTimeout <= 0 {
		c.ToolExecuteTimeout = defaultToolExecuteTimeout
	}
	if c.HeartbeatInterval <= 0 {
		c.HeartbeatInterval = defaultHeartbeatInterval
	}
	if c.PendingTTL <= 0 {
		c.PendingTTL = defaultPendingTTL
	}
	if c.BaseSystemPrompt == "" {
		c.BaseSystemPrompt = ai.DefaultSystemPrompt()
	}
	return c
}

// Facade 门面核心：持有模型客户端、服务端工具注册中心与影子回合存储。
type Facade struct {
	llm    LLMStreamer
	tools  *ai.Registry
	cfg    Config
	relays *relayStore
}

// NewFacade 创建门面实例（llm 通常为 *ai.SeedClient；tools 为装配完成的
// Agent 工具注册中心，门面按名字分拣——客户端工具名优先于服务端注册名）。
func NewFacade(llm LLMStreamer, tools *ai.Registry, cfg Config) *Facade {
	return &Facade{
		llm:    llm,
		tools:  tools,
		cfg:    cfg.withDefaults(),
		relays: newRelayStore(cfg.PendingTTL),
	}
}

// TurnEvents 门面循环向 HTTP 层的实时输出回调（流式分块已由 HTTP 层写出）。
type TurnEvents struct {
	// OnReasoning 思考流分片（V4.1 Flash 推理过程透传给客户端"在想"动画）
	OnReasoning func(string)
	// OnContent 正文文本分片（打字机气泡）
	OnContent func(string)
	// OnHeartbeat 工具执行期间的空 delta 心跳（HTTP 层实现为写 chunk）
	OnHeartbeat func()
}

// TurnResult 一次门面回合的终态。
type TurnResult struct {
	// FinishReason stop=最终回答 / tool_calls=等待客户端工具回执 / length=截断
	FinishReason string
	// Text 本回合最终文本（流式期间已实时回调，此处供非流式响应聚合）
	Text string
	// ClientCalls finish=tool_calls 时流回客户端的调用（id 已重写为 yhx_ 前缀）
	ClientCalls []WireToolCall
	// Usage 本回合 token 用量（多轮 LLM 调用取末轮——含累计语义由上游决定）
	Usage WireUsage
}

// ErrRelayBusy 影子回合已有续轮在执行（客户端不应并发续轮，属协议异常）
var ErrRelayBusy = errors.New("该回合已在处理中，请勿并发续轮")

// RunTurn 跑完一次门面回合。req 为解析后的 Chat Completions 请求；
// 返回终态由 HTTP 层写出（finish chunk + usage chunk + [DONE]）。
func (f *Facade) RunTurn(ctx context.Context, req ChatRequest, ev TurnEvents) (ret TurnResult, retErr error) {
	history, err := ParseHistory(req.Messages)
	if err != nil {
		return ret, err
	}

	// 续轮识别：末尾 tool 消息带 yhx_ 前缀 = 客户端工具回执 → 唤醒影子回合；
	// 否则（含 pending 丢失的降级路径）按客户端历史原样重建开新回合。
	parentID := relayReceiptTurnID(history.Messages)
	question := turnInput(req.Messages, parentID != "")
	span := trace.Start(sessionIDForFacade, question, 0)
	span.ContinueFrom(parentID)
	span.NoteMode("coopanion")
	ctx = trace.WithSpan(ctx, span)
	turnID := span.ID()
	if turnID == "" {
		turnID = newFacadeTurnID()
	}

	msgs, isResume, daily := f.resolveContext(history)
	withDetails := clientHasDetails(req)
	tools := f.toolsetFor(req)

	eventlog.EmitWith(eventlog.LevelInfo, eventlog.EvtAgentTurnBegin, turnID, sessionIDForFacade, map[string]any{
		"resume": isResume,
		"rounds": f.cfg.MaxToolRounds,
	})
	started := time.Now()
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[openaicompat] 回合 panic: %v\n%s", r, debug.Stack())
			eventlog.EmitWith(eventlog.LevelError, eventlog.EvtAgentTurnCrashed, turnID, sessionIDForFacade, map[string]any{
				"panic": fmt.Sprint(r),
			})
			retErr = errors.New("服务内部异常，请稍后重试")
		}
		eventlog.EmitWith(eventlog.LevelInfo, eventlog.EvtAgentTurnEnd, turnID, sessionIDForFacade, map[string]any{
			"ok": retErr == nil,
			"ms": time.Since(started).Milliseconds(),
		})
		span.End(errText(retErr))
	}()

	var usage chat.CompletionUsage
	for round := 0; round < f.cfg.MaxToolRounds; round++ {
		var pendingCalls []chat.ChatCompletionMessageToolCall
		var answer strings.Builder
		llmStart := time.Now()

		err := f.llm.ChatStream(
			ctx,
			msgs,
			tools,
			func(chunk string) {
				answer.WriteString(chunk)
				span.FirstToken()
				if ev.OnContent != nil {
					ev.OnContent(chunk)
				}
			},
			func(calls []chat.ChatCompletionMessageToolCall) error {
				pendingCalls = calls
				return nil
			},
			func(u chat.CompletionUsage) { usage = u },
			func(r string) {
				if ev.OnReasoning != nil {
					ev.OnReasoning(r)
				}
			},
		)
		if err != nil {
			level, evt := eventlog.LevelError, eventlog.EvtUpstreamError
			if errors.Is(err, context.Canceled) {
				level, evt = eventlog.LevelInfo, eventlog.EvtAgentTurnInterrupted
			}
			eventlog.EmitWith(level, evt, turnID, sessionIDForFacade, map[string]any{"error": err.Error()})
			return ret, fmt.Errorf("上游模型调用失败: %w", err)
		}
		span.LLMRound(time.Since(llmStart).Milliseconds(),
			usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens, 0)
		span.AddDialogue("assistant", answer.String())
		if len(pendingCalls) == 0 && withDetails {
			if script, details := dailyPriceReply(daily, question); details != "" {
				pendingCalls = []chat.ChatCompletionMessageToolCall{{
					ID: newFacadeTurnID(), Type: chat.ToolTypeFunction,
					Function: chat.ChatCompletionMessageToolCallFunction{Name: "pet_say", Arguments: withDailyDetails("{}", script, details)},
				}}
			}
		}

		// 无工具调用 = 最终回答
		if len(pendingCalls) == 0 {
			span.Answer(answer.String())
			return TurnResult{
				FinishReason: FinishStop,
				Text:         answer.String(),
				Usage:        wireUsage(usage),
			}, nil
		}

		// 分拣：客户端工具（请求 tools 名单内）vs 服务端工具（Registry）
		clientNames := clientToolNames(req)
		var serverCalls, clientCalls []chat.ChatCompletionMessageToolCall
		for _, c := range pendingCalls {
			if clientNames[c.Function.Name] {
				clientCalls = append(clientCalls, c)
			} else {
				serverCalls = append(serverCalls, c)
			}
		}

		// 完整 tool_calls 信封先入上下文（配对完整性的生命线：
		// assistant.tool_calls 与后续 tool 结果必须一一对应）
		assistantAt := len(msgs)
		msgs = append(msgs, ai.NewAssistantMessageWithToolCalls(answer.String(), pendingCalls))

		// 服务端工具就地执行（客户端不可见），期间发心跳防断流
		if len(serverCalls) > 0 {
			results := f.executeServerTools(ctx, round+1, serverCalls, ev)
			for i, c := range serverCalls {
				msgs = append(msgs, ai.NewToolResultMessage(callIDOf(round, i, c), c.Function.Name, results[i]))
				if c.Function.Name == "delta_keys_day_price" {
					var table deltaapi.DailyKeyPrices
					if json.Unmarshal([]byte(results[i]), &table) == nil && len(table.Cards) > 0 {
						daily = append(daily, table)
					}
				}
			}
		}

		// 客户端工具：重写 id、存影子回合、以 tool_calls 结束本次 HTTP 响应
		if len(clientCalls) > 0 {
			wireCalls := make([]WireToolCall, 0, len(clientCalls))
			relayCalls := make([]relayCall, 0, len(clientCalls))
			var spoken []string
			for seq, c := range clientCalls {
				if c.Function.Name == "pet_say" && withDetails && len(daily) > 0 {
					if script, details := dailyPriceReply(daily, question); details != "" {
						c.Function.Arguments = withDailyDetails(c.Function.Arguments, script, details)
						for i := range pendingCalls {
							if pendingCalls[i].ID == c.ID {
								pendingCalls[i] = c
								break
							}
						}
						daily = nil
					}
				}
				if c.Function.Name == "pet_say" || c.Function.Name == "pet_ask" {
					var reply petReply
					if json.Unmarshal([]byte(c.Function.Arguments), &reply) == nil {
						span.AddDialogue("pet", reply.Script)
						span.AddDialogue("pet", reply.Question)
						span.AddDialogue("details", reply.Details)
						spoken = append(spoken, reply.Script, reply.Question, reply.Details)
					}
				}
				wireID := fmt.Sprintf("%s%s_%d", relayCallIDPrefix, turnID, seq)
				wireCalls = append(wireCalls, WireToolCall{
					ID:   wireID,
					Type: "function",
					Function: &WireToolCallFn{
						Name:      c.Function.Name,
						Arguments: c.Function.Arguments,
					},
				})
				relayCalls = append(relayCalls, relayCall{wireID: wireID, origID: c.ID, name: c.Function.Name})
			}
			// 影子上下文快照：含本轮全部 tool_calls 信封与服务端工具结果，
			// 但不含客户端回执（下一请求到达时补齐）
			snapshot := make([]chat.ChatCompletionRequestMessage, len(msgs))
			copy(snapshot, msgs)
			snapshot[assistantAt] = ai.NewAssistantMessageWithToolCalls(answer.String(), pendingCalls)
			f.relays.save(turnID, snapshot, relayCalls, daily)
			span.Answer(strings.TrimSpace(answer.String() + "\n" + strings.Join(spoken, "\n")))
			log.Printf("[openaicompat] 回合 %s 中继 %d 个客户端工具（服务端工具 %d 个已就地执行）",
				turnID, len(clientCalls), len(serverCalls))
			return TurnResult{
				FinishReason: FinishToolCalls,
				Text:         answer.String(),
				ClientCalls:  wireCalls,
				Usage:        wireUsage(usage),
			}, nil
		}
		// 全部是服务端工具：继续循环让模型消化结果
	}

	// 轮数上限兜底（与 Agent 循环同款文案风格）
	tip := "抱歉，这个问题需要查询的数据有点多，我暂时没能整理完，建议缩小范围后再问一次。"
	if ev.OnContent != nil {
		ev.OnContent(tip)
	}
	span.Answer(tip)
	span.AddDialogue("assistant", tip)
	return TurnResult{FinishReason: FinishStop, Text: tip, Usage: wireUsage(usage)}, nil
}

// sessionIDForFacade 门面回合在 trace/eventlog 中的会话标识（固定值，
// 观测页按 mode=coopanion 区分来源）。
const sessionIDForFacade = "coopanion"

// resolveContext 组装本轮起跑上下文。续轮命中影子回合时返回其快照并按
// wire→原生 id 映射合并客户端回执；否则（新回合 / pending 丢失降级）
// 按客户端历史原样重建。
func (f *Facade) resolveContext(history ParsedHistory) ([]chat.ChatCompletionRequestMessage, bool, []deltaapi.DailyKeyPrices) {
	system := f.fusedSystemPrompt(history.SystemText)
	base := append([]chat.ChatCompletionRequestMessage{ai.NewSystemMessage(system)}, history.Messages...)

	// 续轮识别：消息末尾是 tool 回执（yhx_ 前缀）
	relayTurnID := relayReceiptTurnID(history.Messages)
	if relayTurnID == "" {
		return base, false, nil
	}
	stored, ok := f.relays.take(relayTurnID)
	if !ok {
		// 影子回合丢失（服务重启 / TTL 过期）：降级按客户端历史重建——
		// 历史里的 assistant.tool_calls 与 tool 回执（重写 id）天然配对完整，
		// 模型只是看不到服务端工具痕迹，可能重查一次工具，可接受
		eventlog.EmitWith(eventlog.LevelWarn, eventlog.EvtAgentTurnInterrupted, relayTurnID, sessionIDForFacade, map[string]any{
			"reason": "影子回合丢失，降级重建上下文",
		})
		return base, false, nil
	}
	// 合并客户端回执：按重写 id（wire）映射回模型原生 id（orig）后追加，
	// 保证与影子上下文里 assistant.tool_calls 的配对完整；客户端漏回执的
	// 调用补错误占位（协议要求一次全回执，防御异构客户端）。
	received := make(map[string]string, len(stored.relays))
	for _, m := range trailingToolReceipts(history.Messages) {
		received[toolCallIDOf(m)] = messageContentText(m)
	}
	msgs := append([]chat.ChatCompletionRequestMessage{}, stored.messages...)
	merged := 0
	for _, rc := range stored.relays {
		if content, ok := received[rc.wireID]; ok {
			msgs = append(msgs, ai.NewToolResultMessage(rc.origID, rc.name, content))
			merged++
		} else {
			msgs = append(msgs, ai.NewToolResultMessage(rc.origID, rc.name, toolErrorJSON("客户端未回执该工具调用")))
		}
	}
	log.Printf("[openaicompat] 影子回合 %s 唤醒（回执 %d/%d）", relayTurnID, merged, len(stored.relays))
	return msgs, true, stored.daily
}

// fusedSystemPrompt 融合系统提示词（详见 prompt.go）。
func (f *Facade) fusedSystemPrompt(clientSystem string) string {
	return fuseSystemPrompt(f.cfg, clientSystem)
}

// toolsetFor 组装本轮下发给模型的工具集：服务端工具（剔除 ask_user——
// 澄清场景由客户端的 pet_ask 天然覆盖，且 Coopanion 无法执行 ask_user）
// + 请求携带的客户端工具 schema。
func (f *Facade) toolsetFor(req ChatRequest) []chat.ChatCompletionTool {
	var defs []chat.ChatCompletionTool
	for _, def := range f.tools.Definitions() {
		if def.Function.Name == "ask_user" {
			continue
		}
		defs = append(defs, def)
	}
	return append(defs, wireToolsToArk(req.Tools)...)
}

// executeServerTools 并行执行一组服务端工具（复刻 Agent.executeToolSafe 的
// 安全网：超时/panic 恢复/结果截断/trace 埋点），等待期间按间隔发心跳。
func (f *Facade) executeServerTools(ctx context.Context, round int, calls []chat.ChatCompletionMessageToolCall, ev TurnEvents) []string {
	results := make([]string, len(calls))
	var wg sync.WaitGroup
	for i, c := range calls {
		wg.Add(1)
		go func(i int, c chat.ChatCompletionMessageToolCall) {
			defer wg.Done()
			results[i] = f.executeServerTool(ctx, round, c)
		}(i, c)
	}
	if ev.OnHeartbeat != nil {
		done := make(chan struct{})
		go func() { wg.Wait(); close(done) }()
		ticker := time.NewTicker(f.cfg.HeartbeatInterval)
		defer ticker.Stop()
	waitLoop:
		for {
			select {
			case <-done:
				break waitLoop
			case <-ticker.C:
				ev.OnHeartbeat()
			case <-ctx.Done():
				break waitLoop
			}
		}
	}
	wg.Wait()
	return results
}

// executeServerTool 执行单个服务端工具，无论成功失败都返回合法 JSON 字符串。
func (f *Facade) executeServerTool(parent context.Context, round int, call chat.ChatCompletionMessageToolCall) (resultJSON string) {
	span := trace.FromContext(parent)
	start := time.Now()
	ok := true
	errMsg := ""
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[openaicompat] 工具 %s 执行panic: %v", call.Function.Name, r)
			resultJSON = toolErrorJSON("工具内部异常，请换个问法或稍后再试")
			ok = false
			errMsg = fmt.Sprintf("panic: %v", r)
		}
		span.ToolCall(round, call.Function.Name, call.Function.Arguments,
			time.Since(start).Milliseconds(), len(resultJSON), ok, errMsg)
	}()

	tool, exists := f.tools.Get(call.Function.Name)
	if !exists {
		ok = false
		errMsg = "未注册工具"
		return toolErrorJSON(fmt.Sprintf("工具 %s 不存在，请从可用工具列表中选择", call.Function.Name))
	}
	rawArgs := json.RawMessage(call.Function.Arguments)
	if len(strings.TrimSpace(string(rawArgs))) == 0 {
		rawArgs = json.RawMessage(`{}`)
	}
	ctx, cancel := context.WithTimeout(parent, f.cfg.ToolExecuteTimeout)
	defer cancel()

	output, err := tool.Execute(ctx, rawArgs)
	if err != nil {
		ok = false
		errMsg = err.Error()
		return toolErrorJSON(friendlyToolError(err))
	}
	payload, err := json.Marshal(output)
	if err != nil {
		ok = false
		errMsg = "结果序列化失败"
		return toolErrorJSON("工具返回数据格式异常，请稍后再试")
	}
	if len(payload) > maxToolResultBytes {
		ok = false
		errMsg = "返回超限截断"
		return toolErrorJSON("工具返回数据量过大，请增加筛选条件（如名称、分类、分页）缩小范围后重试")
	}
	log.Printf("[openaicompat] 工具 %s 执行成功 耗时=%dms 返回=%d字节",
		call.Function.Name, time.Since(start).Milliseconds(), len(payload))
	return string(payload)
}

// ---- 影子回合存储 ----

// relayCall 一次待回执的客户端工具调用（双 id：wire 面向客户端，orig 面向模型）。
type relayCall struct {
	wireID string
	origID string
	name   string
}

// relayTurn 一个挂起的影子回合。
type relayTurn struct {
	daily    []deltaapi.DailyKeyPrices
	messages []chat.ChatCompletionRequestMessage // 影子上下文（不含客户端回执）
	relays   []relayCall                         // 待回执清单（wire→orig 映射）
	expires  time.Time
}

// relayStore 影子回合存储：内存 map + TTL 后台清理（复刻会话管理器的模式）。
type relayStore struct {
	mu     sync.Mutex
	turns  map[string]*relayTurn
	ttl    time.Duration
	closed chan struct{}
}

func newRelayStore(ttl time.Duration) *relayStore {
	s := &relayStore{
		turns:  make(map[string]*relayTurn),
		ttl:    ttl,
		closed: make(chan struct{}),
	}
	go s.cleanupLoop()
	return s
}

func (s *relayStore) save(turnID string, messages []chat.ChatCompletionRequestMessage, relays []relayCall, daily []deltaapi.DailyKeyPrices) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.turns[turnID] = &relayTurn{messages: messages, relays: relays, daily: daily, expires: time.Now().Add(s.ttl)}
}

// take 取出并删除影子回合（消费即删：继续跑出新的客户端工具调用会存新回合）。
func (s *relayStore) take(turnID string) (*relayTurn, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.turns[turnID]
	if ok {
		delete(s.turns, turnID)
	}
	return t, ok
}

func (s *relayStore) cleanupLoop() {
	ticker := time.NewTicker(pendingCleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.closed:
			return
		case <-ticker.C:
			now := time.Now()
			s.mu.Lock()
			for id, t := range s.turns {
				if now.After(t.expires) {
					delete(s.turns, id)
				}
			}
			s.mu.Unlock()
		}
	}
}

// trailingToolReceipts 提取历史末尾连续的 tool 消息（原顺序）。
func trailingToolReceipts(messages []chat.ChatCompletionRequestMessage) []chat.ChatCompletionRequestMessage {
	n := 0
	for i := len(messages) - 1; i >= 0; i-- {
		if !isToolMessage(messages[i]) {
			break
		}
		n++
	}
	return messages[len(messages)-n:]
}

// relayReceiptTurnID 从末尾回执组解析影子回合标识：
// 回执全部为 yhx_<turnID>_<seq> 前缀时返回 turnID，否则（空回执 / 非 yhx_
// 前缀的异构形态）返回空串走新回合路径。turnID 自身可能含下划线（trace ID
// 形如 t_<hex>），因此按最后一个下划线从尾部剥离序号，不做"首个下划线"切分。
func relayReceiptTurnID(messages []chat.ChatCompletionRequestMessage) string {
	receipts := trailingToolReceipts(messages)
	if len(receipts) == 0 {
		return ""
	}
	turnID := ""
	for _, m := range receipts {
		id := toolCallIDOf(m)
		if !strings.HasPrefix(id, relayCallIDPrefix) {
			return ""
		}
		if turnID == "" {
			rest := strings.TrimPrefix(id, relayCallIDPrefix)
			if u := strings.LastIndexByte(rest, '_'); u > 0 {
				turnID = rest[:u]
			}
		}
	}
	return turnID
}

// messageContentText 提取消息 content 文本（tool 回执为字符串形态）。
func messageContentText(m chat.ChatCompletionRequestMessage) string {
	b, err := m.MarshalJSON()
	if err != nil {
		return ""
	}
	var obj struct {
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(b, &obj); err != nil {
		return ""
	}
	return contentString(obj.Content)
}

// ---- 辅助 ----

// clientToolNames 请求携带的客户端工具名集合（分拣依据：客户端名单优先）。
func clientToolNames(req ChatRequest) map[string]bool {
	names := make(map[string]bool, len(req.Tools))
	for _, t := range req.Tools {
		if t.Function != nil && t.Function.Name != "" {
			names[t.Function.Name] = true
		}
	}
	return names
}

// wireToolsToArk wire 工具定义转方舟 SDK 形态（parameters 为完整 JSON Schema
// 对象，拆成 map[string]jx.Raw 逐字段搬运——与 ai.toFunctionParameters 同路线）。
func wireToolsToArk(tools []WireTool) []chat.ChatCompletionTool {
	out := make([]chat.ChatCompletionTool, 0, len(tools))
	for _, t := range tools {
		if t.Function == nil || t.Function.Name == "" {
			continue
		}
		params := chat.FunctionObjectParameters{}
		if len(t.Function.Parameters) > 0 {
			var schema map[string]json.RawMessage
			if err := json.Unmarshal(t.Function.Parameters, &schema); err == nil {
				for k, v := range schema {
					params[k] = jx.Raw(v)
				}
			}
		}
		if _, ok := params["type"]; !ok {
			params["type"] = jx.Raw(`"object"`)
		}
		def := chat.ChatCompletionTool{
			Type: chat.ToolTypeFunction,
			Function: chat.FunctionObject{
				Name:        t.Function.Name,
				Description: chat.NewOptString(t.Function.Description),
				Parameters:  chat.NewOptFunctionObjectParameters(params),
			},
		}
		out = append(out, def)
	}
	return out
}

// callIDOf 模型未下发 id 时本地补一个（与 Agent 循环同语义）。
func callIDOf(round, idx int, c chat.ChatCompletionMessageToolCall) string {
	if c.ID != "" {
		return c.ID
	}
	return fmt.Sprintf("call_%d_%d", round, idx)
}

// toolCallIDOf 提取一条消息的 tool_call_id（非 tool 消息返回空）。
func toolCallIDOf(m chat.ChatCompletionRequestMessage) string {
	b, err := m.MarshalJSON()
	if err != nil {
		return ""
	}
	var obj struct {
		ToolCallID string `json:"tool_call_id"`
	}
	if err := json.Unmarshal(b, &obj); err != nil {
		return ""
	}
	return obj.ToolCallID
}

// isToolMessage 与 ai 包的窗口边界判断同语义（经 JSON 判 role，不拆 Sum 类型）。
func isToolMessage(m chat.ChatCompletionRequestMessage) bool {
	b, err := m.MarshalJSON()
	if err != nil {
		return false
	}
	var obj struct {
		Role string `json:"role"`
	}
	if err := json.Unmarshal(b, &obj); err != nil {
		return false
	}
	return obj.Role == "tool"
}

// toolErrorJSON 构造回传给模型的统一错误结构（与 Agent 循环一致）。
func toolErrorJSON(message string) string {
	b, _ := json.Marshal(map[string]string{"error": message})
	return string(b)
}

// friendlyToolError 工具层错误转模型可转述的中文提示（与 Agent 循环一致）。
func friendlyToolError(err error) string {
	if errors.Is(err, context.DeadlineExceeded) ||
		strings.Contains(err.Error(), "timeout") ||
		strings.Contains(err.Error(), "超时") {
		return "数据查询超时，请稍后再试"
	}
	return "数据查询失败：" + err.Error()
}

// wireUsage SDK usage → wire usage（SDK 侧为 int32）。
func wireUsage(u chat.CompletionUsage) WireUsage {
	return WireUsage{
		PromptTokens:     int(u.PromptTokens),
		CompletionTokens: int(u.CompletionTokens),
		TotalTokens:      int(u.TotalTokens),
	}
}

// newFacadeTurnID 生成本地回合标识（trace 未启用时使用）。
func newFacadeTurnID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("oc%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
