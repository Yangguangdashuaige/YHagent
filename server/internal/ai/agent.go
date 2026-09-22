// internal/ai/agent.go
package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/volcengine/ark-runtime-go/arkruntime/model/chat"
)

const (
	// defaultSessionID 未显式指定会话时使用的默认会话（当前无UI阶段也能直接跑）
	defaultSessionID = "default"
	// maxToolRounds 单次提问内允许的最大工具调用轮数，防止模型陷入死循环把上下文撑爆
	maxToolRounds = 5
	// toolExecuteTimeout 单个工具的最长执行时间，三角洲接口再慢也必须在此时间内降级返回
	toolExecuteTimeout = 25 * time.Second
	// maxToolResultBytes 工具结果回传给模型的硬上限，超出直接降级为提示信息，避免 token 爆炸
	maxToolResultBytes = 32 * 1024
)

// defaultSystemPrompt Agent 的基础人设与工具使用规则
const defaultSystemPrompt = `你是“三角洲行动”游戏助手小小轩，通过悬浮窗为玩家提供实时数据查询服务。
工作规则：
1. 需要实时游戏数据（物品价格、战备配装、钥匙卡、子弹、密码、任务等）时，必须调用对应工具获取，不要凭空编造数据。
2. 调用工具前可以根据用户意图补齐合理的默认参数；工具返回异常时，用通俗中文向用户说明情况并给出替代建议。
3. 回答一律使用中文，简洁直接，适当使用列表和emoji，不要输出工具原始JSON。
4. 价格、战备值等数字以工具返回的最新数据为准，并说明数据仅供参考。`

// Agent Agent 核心：持有大模型客户端、工具注册中心和会话管理器，
// 负责跑完“提问→模型决策→工具执行→结果回传→流式回答”的完整多轮循环。
type Agent struct {
	client       *SeedClient
	tools        *Registry
	sessions     *Manager
	systemPrompt string
}

// Option Agent 可选配置
type Option func(*Agent)

// WithSystemPrompt 自定义系统提示词
func WithSystemPrompt(prompt string) Option {
	return func(a *Agent) {
		if prompt != "" {
			a.systemPrompt = prompt
		}
	}
}

// WithSessionManager 注入外部会话管理器（未来接入持久化存储时使用）
func WithSessionManager(m *Manager) Option {
	return func(a *Agent) {
		if m != nil {
			a.sessions = m
		}
	}
}

// NewAgent 创建 Agent 核心实例，默认自带空工具注册中心和内存会话管理器
func NewAgent(client *SeedClient, opts ...Option) *Agent {
	a := &Agent{
		client:       client,
		tools:        NewRegistry(),
		sessions:     NewSessionManager(),
		systemPrompt: defaultSystemPrompt,
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// Tools 暴露工具注册中心，外部模块（如三角洲API封装）通过它注册工具，无需改动 Agent 核心
func (a *Agent) Tools() *Registry {
	return a.tools
}

// Reset 清空指定会话的历史上下文
func (a *Agent) Reset(sessionID string) {
	if sessionID == "" {
		sessionID = defaultSessionID
	}
	a.sessions.GetOrCreate(sessionID, a.systemPrompt).reset(a.systemPrompt)
}

// Chat 在默认会话中发起一轮带工具调用能力的流式对话
func (a *Agent) Chat(ctx context.Context, userText string, onChunk func(content string)) error {
	return a.ChatWithSession(ctx, defaultSessionID, userText, onChunk)
}

// ChatWithSession 在指定会话中发起一轮流式对话，完整多轮 Function Call 循环：
// 用户提问 → 带上下文和工具定义请求豆包 → 模型要求调工具则执行并把结果回传 → 直到模型输出最终回答并流式推给前端。
func (a *Agent) ChatWithSession(ctx context.Context, sessionID, userText string, onChunk func(content string)) error {
	if strings.TrimSpace(userText) == "" {
		return errors.New("提问内容不能为空")
	}
	if sessionID == "" {
		sessionID = defaultSessionID
	}
	session := a.sessions.GetOrCreate(sessionID, a.systemPrompt)
	session.append(NewUserMessage(userText))
	log.Printf("[agent] 会话 %s 收到提问: %s", sessionID, previewArgs(userText))

	for round := 0; round < maxToolRounds; round++ {
		// pendingCalls 收集本轮模型要求执行的工具调用；answerBuilder 累积本轮流式文本
		var pendingCalls []chat.ChatCompletionMessageToolCall
		var answerBuilder strings.Builder

		err := a.client.ChatStream(
			ctx,
			session.snapshot(),
			a.tools.Definitions(),
			func(chunk string) {
				answerBuilder.WriteString(chunk)
				if onChunk != nil {
					onChunk(chunk)
				}
			},
			func(calls []chat.ChatCompletionMessageToolCall) error {
				pendingCalls = calls
				return nil
			},
		)
		if err != nil {
			// 豆包API超时/失败：回滚本轮用户消息，返回友好提示，绝不崩溃
			session.popLast()
			log.Printf("[agent] 豆包API调用失败: %v", err)
			return friendlyUpstreamError(err)
		}

		// 没有工具调用 = 模型已给出最终回答，落盘历史后结束
		if len(pendingCalls) == 0 {
			session.append(NewAssistantTextMessage(answerBuilder.String()))
			return nil
		}

		// 模型要求调工具：先把“助手发起的工具调用”写进上下文
		session.append(NewAssistantToolCallMessage(pendingCalls))

		// 逐个执行工具并把结果回传；单个工具失败只影响该工具，错误以消息形式回传由模型降级解释
		for idx, call := range pendingCalls {
			callID := call.ID
			if callID == "" {
				// 极少数情况下模型不下发ID，本地补一个，保证 tool 消息能与 tool_calls 对应
				callID = fmt.Sprintf("call_%d_%d", round, idx)
			}
			log.Printf("[agent] 会话 %s 第%d轮 调用工具 #%d: %s 参数=%s",
				sessionID, round+1, idx+1, call.Function.Name, previewArgs(call.Function.Arguments))
			result := a.executeToolSafe(ctx, call)
			session.append(NewToolResultMessage(callID, call.Function.Name, result))
		}
		// 继续下一轮，让模型基于工具结果决定继续调工具还是输出最终回答
	}

	// 达到工具轮数上限仍未收敛：给出友好兜底，并保持对话历史结构完整
	tip := "抱歉，这个问题需要查询的数据有点多，我暂时没能整理完，建议缩小范围后再问一次。"
	if onChunk != nil {
		onChunk(tip)
	}
	session.append(NewAssistantTextMessage(tip))
	log.Printf("[agent] 会话 %s 达到最大工具轮数 %d，已兜底结束", sessionID, maxToolRounds)
	return nil
}

// executeToolSafe 执行单个工具，统一做超时控制、panic 恢复和错误降级，
// 无论成功失败都返回合法 JSON 字符串，保证主循环不会被任何工具拖垮。
func (a *Agent) executeToolSafe(parent context.Context, call chat.ChatCompletionMessageToolCall) (resultJSON string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[agent] 工具 %s 执行panic: %v", call.Function.Name, r)
			resultJSON = toolErrorJSON("工具内部异常，请换个问法或稍后再试")
		}
	}()

	tool, ok := a.tools.Get(call.Function.Name)
	if !ok {
		log.Printf("[agent] 模型调用了未注册工具: %s", call.Function.Name)
		return toolErrorJSON(fmt.Sprintf("工具 %s 不存在，请从可用工具列表中选择", call.Function.Name))
	}

	// 模型不下发参数时按空对象处理
	rawArgs := json.RawMessage(call.Function.Arguments)
	if len(strings.TrimSpace(string(rawArgs))) == 0 {
		rawArgs = json.RawMessage(`{}`)
	}

	ctx, cancel := context.WithTimeout(parent, toolExecuteTimeout)
	defer cancel()

	start := time.Now()
	output, err := tool.Execute(ctx, rawArgs)
	cost := time.Since(start)
	if err != nil {
		log.Printf("[agent] 工具 %s 执行失败 耗时=%dms: %v", call.Function.Name, cost.Milliseconds(), err)
		return toolErrorJSON(friendlyToolError(err))
	}

	payload, err := json.Marshal(output)
	if err != nil {
		log.Printf("[agent] 工具 %s 结果序列化失败: %v", call.Function.Name, err)
		return toolErrorJSON("工具返回数据格式异常，请稍后再试")
	}
	log.Printf("[agent] 工具 %s 执行成功 耗时=%dms 返回=%d字节", call.Function.Name, cost.Milliseconds(), len(payload))
	if len(payload) > maxToolResultBytes {
		log.Printf("[agent] 工具 %s 返回过大(%d字节)，已截断降级", call.Function.Name, len(payload))
		return toolErrorJSON("工具返回数据量过大，请增加筛选条件（如名称、分类、分页）缩小范围后重试")
	}
	return string(payload)
}

// previewArgs 把工具调用参数压缩成适合写日志的短字符串：
// 去掉空白并限制长度，避免大分页参数把日志撑爆。
func previewArgs(raw string) string {
	s := strings.Join(strings.Fields(raw), "")
	if s == "" {
		return "{}"
	}
	const max = 300
	if len(s) > max {
		return s[:max] + "...(截断)"
	}
	return s
}

// toolErrorJSON 构造回传给模型的统一错误结构
func toolErrorJSON(message string) string {
	b, _ := json.Marshal(map[string]string{"error": message})
	return string(b)
}

// friendlyToolError 把工具层错误转换成适合模型转述给玩家的中文提示
func friendlyToolError(err error) string {
	if errors.Is(err, context.DeadlineExceeded) || isTimeoutText(err.Error()) {
		return "数据查询超时，请稍后再试"
	}
	return "数据查询失败：" + err.Error()
}

// friendlyUpstreamError 把豆包侧错误转换成面向用户的中文友好提示，原始错误只写日志
func friendlyUpstreamError(err error) error {
	if errors.Is(err, context.Canceled) {
		return errors.New("请求已取消")
	}
	if errors.Is(err, context.DeadlineExceeded) || isTimeoutText(err.Error()) {
		return errors.New("豆包AI响应超时，请检查网络后稍后再试")
	}
	return errors.New("AI服务暂时不可用，请稍后再试")
}

// isTimeoutText 兜底识别SDK/网络库包装后的超时错误
func isTimeoutText(msg string) bool {
	return strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "deadline") ||
		strings.Contains(msg, "超时")
}
