// internal/ai/compact.go
//
// 会话上下文的 token 预算裁剪 + 滚动摘要压缩。
//
// 动机：此前会话只按"固定保留最近 20 条"管理（snapshot），条数≠token——
// 一条 32KB 的工具结果和一句"多少钱"占的资源差几十倍，且第 21 条会把
// 玩家刚说过的关键偏好（主玩枪械/预算）直接挤出窗口，模型"失忆"。
//
// 方案：每次请求开始时估算会话历史（不含 system）的 token 总量，
// 超过 SessionTokenBudget 就把旧消息交给 LLM 压缩成一条滚动摘要
// （旧摘要参与合并，信息不逐层丢失），近期消息原文保留。
// 摘要压缩失败不影响主流程（下次请求再试）；预算未配置（<=0）时整体禁用。
package ai

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/volcengine/ark-runtime-go/arkruntime/model/chat"

	"yhagent-server/internal/trace"
)

// SessionTokenBudget 单会话历史消息（不含 system prompt）的 token 预算。
// cmd/server/main.go 从 SESSION_TOKEN_BUDGET 注入；<=0 表示禁用压缩。
var SessionTokenBudget = 0

const (
	// compactKeepRatio 压缩后保留的近期消息 token 目标（占预算的比例）
	compactKeepRatio = 0.5
	// minPrefixTokens 待压缩前缀低于该 token 数不值得调一次 LLM
	minPrefixTokens = 800
	// transcriptMsgRunes 渲染给压缩器的单条消息上限（控制摘要调用本身的成本）
	transcriptMsgRunes = 1200
	// imageTokenEstimate 每张历史图片的固定 token 估算
	// （图片走 URL 引用时不按 base64 长度计，那样会虚高几十倍）
	imageTokenEstimate = 1000
	// summaryMarker 摘要消息的识别前缀（同时是给模型的提示）
	summaryMarker = "[历史对话摘要]"
	// compactLLMTimeout 摘要调用一次 LLM 的超时。主对话端点是思考模型，
	// 大入参时首 token 可达 1 分钟量级（实测 ~2.6k token 转录耗时 54-70s）
	compactLLMTimeout = 120 * time.Second
	// compactBackoff 摘要失败后的退避时长：期间跳过压缩，避免思考模型
	// 状态不佳时每次请求都白等一次超时
	compactBackoff = 10 * time.Minute
)

const compactSystemPrompt = `你是游戏助手系统的对话历史压缩器。把提供的历史对话压缩成一份摘要，供后续对话参考。
要求：
1. 必须保留：玩家的个人偏好（主玩干员/枪械/预算/习惯等）、已确认的关键结论与数值（价格/方案/密码等）、未完成或待跟进的事项；
2. 丢弃寒暄、重复提问、失败尝试的细节；
3. 若提供了旧摘要，将其与新对话合并为一份更新后的摘要，不要丢掉旧摘要里仍有效的信息；
4. 用要点式中文输出，不超过400字，不要输出摘要以外的任何内容。`

// ---------- token 估算 ----------

// estimateMessageTokens 估算一条消息的 token 数。口径偏保守（宁可高估）：
// CJK 字符≈1 token，ASCII≈1/4 token，每条固定开销 8，图片按固定值计。
func estimateMessageTokens(msg chat.ChatCompletionRequestMessage) int {
	_, text, images := messageText(msg)
	return 8 + estString(text) + images*imageTokenEstimate
}

func estString(s string) int {
	if s == "" {
		return 0
	}
	cjk, ascii := 0, 0
	for _, r := range s {
		if r < 128 {
			ascii++
		} else {
			cjk++
		}
	}
	return cjk + ascii/4
}

// messageText 提取消息的角色、纯文本和图片数（token 估算与摘要渲染共用）。
func messageText(msg chat.ChatCompletionRequestMessage) (role, text string, images int) {
	sum := msg.OneOf
	switch {
	case sum.IsChatCompletionRequestUserMessage():
		role = "user"
		m := sum.ChatCompletionRequestUserMessage
		if m.Content.Type == chat.StringChatCompletionMessageContent {
			text = m.Content.String
		} else {
			var b strings.Builder
			for _, p := range m.Content.ChatCompletionContentPartArray {
				ps := p.OneOf
				if ps.IsChatCompletionContentPartText() {
					b.WriteString(ps.ChatCompletionContentPartText.Text)
				} else {
					images++
				}
			}
			text = b.String()
		}
	case sum.IsChatCompletionRequestAssistantMessage():
		role = "assistant"
		m := sum.ChatCompletionRequestAssistantMessage
		var b strings.Builder
		if c, ok := m.Content.Get(); ok && c.Type == chat.StringChatCompletionMessageContent {
			b.WriteString(c.String)
		}
		for _, c := range m.ToolCalls {
			if b.Len() > 0 {
				b.WriteString(" ")
			}
			b.WriteString(c.Function.Name + " " + c.Function.Arguments)
		}
		text = b.String()
	case sum.IsChatCompletionRequestToolMessage():
		role = "tool"
		m := sum.ChatCompletionRequestToolMessage
		if m.Content.Type == chat.StringChatCompletionMessageContent {
			text = m.Content.String
		}
	case sum.IsChatCompletionRequestSystemMessage():
		role = "system"
		m := sum.ChatCompletionRequestSystemMessage
		if m.Content.Type == chat.StringChatCompletionMessageContent {
			text = m.Content.String
		}
	}
	return role, text, images
}

func isUserMessage(msg chat.ChatCompletionRequestMessage) bool {
	r, _, _ := messageText(msg)
	return r == "user"
}

// isSummaryMessage 判断是否为滚动摘要消息（以固定前缀开头的用户消息）
func isSummaryMessage(msg chat.ChatCompletionRequestMessage) bool {
	r, text, _ := messageText(msg)
	return r == "user" && strings.HasPrefix(text, summaryMarker)
}

// ---------- 切点选择（纯函数，单测覆盖） ----------

// compactCut 计算压缩切点：返回 k 表示 msgs[:k] 将被压缩为摘要、msgs[k:] 原样保留；
// 无需压缩或无可行的切点时返回 -1。
//
// 切点约束：
//   - 只允许落在用户消息上——assistant 的 tool_calls 与其 tool 结果消息
//     必须同生共死，拆开发给模型会直接 400（tool 消息找不到发起者）；
//   - 待压缩前缀不足 minPrefixTokens 时返回 -1（不值得为几百 token 调一次 LLM）。
func compactCut(msgs []chat.ChatCompletionRequestMessage, budget int) int {
	total := 0
	for _, m := range msgs {
		total += estimateMessageTokens(m)
	}
	if total <= budget {
		return -1
	}
	// 从尾部向前保留近期消息，直到占满保留配额
	keepTokens := int(float64(budget) * compactKeepRatio)
	acc, k := 0, len(msgs)
	for k > 0 {
		acc += estimateMessageTokens(msgs[k-1])
		if acc >= keepTokens {
			break
		}
		k--
	}
	// 推进切点到最近的用户消息边界（tool 消息不能作为保留段起点）
	for k < len(msgs) && !isUserMessage(msgs[k]) {
		k++
	}
	// 至少保留最后一条（正常情况下尾部就是刚 append 的用户消息）
	if k >= len(msgs) {
		return -1
	}
	prefix := 0
	for i := 0; i < k; i++ {
		prefix += estimateMessageTokens(msgs[i])
	}
	if k < 2 || prefix < minPrefixTokens {
		return -1
	}
	return k
}

// ---------- 压缩执行 ----------

// compactIfNeeded 在请求开始时检查会话 token 预算，超限则压缩旧消息为滚动摘要。
// 任何失败只记日志不影响主流程；预算未配置时为 no-op。
func (a *Agent) compactIfNeeded(ctx context.Context, s *Session, span *trace.Span) {
	budget := SessionTokenBudget
	if budget <= 0 {
		return
	}
	if s.compactBlocked() {
		return
	}
	msgs := s.historyAfterSystem()
	if len(msgs) == 0 {
		return
	}
	k := compactCut(msgs, budget)
	if k < 0 {
		return
	}
	before := 0
	for _, m := range msgs {
		before += estimateMessageTokens(m)
	}

	start := time.Now()
	summary, err := a.summarizeHistory(ctx, msgs[:k])
	if err != nil {
		s.noteCompactResult(false)
		log.Printf("[session] 会话 %s 滚动摘要压缩失败（跳过，%s 内不重试）: %v",
			s.ID(), compactBackoff, err)
		return
	}
	s.noteCompactResult(true)
	ms := time.Since(start).Milliseconds()

	s.replacePrefix(k, NewUserMessage(summaryMarker+"\n"+summary))
	after := 0
	for _, m := range s.historyAfterSystem() {
		after += estimateMessageTokens(m)
	}
	span.NoteCompact(k, before, after, ms)
	log.Printf("[session] 会话 %s 上下文压缩：%d 条消息 → 1 条摘要，估算 token %d → %d（耗时 %dms）",
		s.ID(), k, before, after, ms)
}

// summarizeHistory 把一段历史消息（可能首条已是旧摘要）压缩成一份新摘要。
func (a *Agent) summarizeHistory(ctx context.Context, prefix []chat.ChatCompletionRequestMessage) (string, error) {
	var b strings.Builder
	rest := prefix
	if len(rest) > 0 && isSummaryMessage(rest[0]) {
		_, old, _ := messageText(rest[0])
		b.WriteString("【旧摘要（合并进新摘要，仍有效的信息不要丢）】\n")
		b.WriteString(strings.TrimPrefix(old, summaryMarker))
		b.WriteString("\n\n【新对话记录】\n")
		rest = rest[1:]
	}
	for i, m := range rest {
		role, text, _ := messageText(m)
		b.WriteString(roleLabel(role) + "：" + truncateRunes(text, transcriptMsgRunes))
		if i < len(rest)-1 {
			b.WriteString("\n")
		}
	}

	cctx, cancel := context.WithTimeout(ctx, compactLLMTimeout)
	defer cancel()
	out, err := a.client.SimpleChat(cctx, compactSystemPrompt, b.String())
	if err != nil {
		return "", err
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return "", errEmptySummary
	}
	return out, nil
}

var errEmptySummary = errors.New("摘要结果为空")

func roleLabel(role string) string {
	switch role {
	case "user":
		return "玩家"
	case "assistant":
		return "助手"
	case "tool":
		return "工具结果"
	}
	return role
}
