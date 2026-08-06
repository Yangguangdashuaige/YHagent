package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/volcengine/volcengine-go-sdk/service/arkruntime"
	"github.com/volcengine/volcengine-go-sdk/service/arkruntime/model"
	"github.com/volcengine/volcengine-go-sdk/volcengine"

	"YHagent/config"
)

// 全局声明豆包客户端，不再在init中提前初始化（避免配置未加载时用空密钥初始化）
var client *arkruntime.Client

// SystemPrompt 客服专属System Prompt （非敏感内容保持不变）
const SystemPrompt = `
你是XX淘宝店的智能客服小豆，需要严格遵守以下规则回答用户问题：
1. 身份：你是店铺的专属客服，语气亲切自然，符合淘宝客服的说话习惯，可以适当用😊这类表情，不要太生硬
2. 准确性：所有回答必须基于知识库检索结果、工具查询返回的真实数据，绝对不能编造信息，如果信息不足请直接说"非常抱歉这个问题我暂时无法解答，马上为您转接人工坐席~"
3. 工具使用：涉及订单、物流、商品库存、优惠金额的问题，必须调用对应的工具查询，绝对不能凭记忆回答
4. 风险控制：绝对不可以引导用户去微信、其他平台交易，不可以承诺超出店铺规则的权益（比如超出规则的退款、补偿）
5. 操作规则：涉及退款、改地址等修改类操作，必须先和用户确认信息，得到用户明确同意后再执行，严禁私自操作
6. 转人工：用户明确要求找人工、情绪愤怒、问题超出你能力范围时，直接调用转人工工具
`

// 工具1：查询物流信息 （原有逻辑保持不变）
func getLogisticsInfo(orderId string) string {
	return fmt.Sprintf("订单%s的物流信息：顺丰快递，单号SF123456789，目前已到达杭州转运中心，预计明天送达", orderId)
}

// 工具2：转人工（原有逻辑保持不变）
func transferToHuman(reason string) string {
	return fmt.Sprintf("已为您转接人工坐席，转接原因：%s，请稍等~", reason)
}

func main() {
	// ========== 改造点：程序启动第一步加载配置/密钥，校验失败直接退出 ==========
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("配置初始化失败: %v", err)
	}

	// ========== 改造点：配置加载成功后再初始化客户端，使用从配置中读取的密钥 ==========
	client = arkruntime.NewClientWithApiKey(
		cfg.ArkAPIKey,
		arkruntime.WithBaseUrl(cfg.ArkBaseUrl),
	)

	ctx := context.Background()
	// 注册给大模型的工具列表 （原有逻辑保持不变）
	tools := []*model.Tool{
		{
			Type: model.ToolTypeFunction,
			Function: &model.FunctionDefinition{
				Name:        "get_logistics_info",
				Description: "当用户需要查询订单物流状态、快递进度、预计送达时间时调用此工具",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"order_id": map[string]interface{}{
							"type":        "string",
							"description": "用户要查询的订单号",
						},
					},
					"required": []string{"order_id"},
				},
			},
		},
		{
			Type: model.ToolTypeFunction,
			Function: &model.FunctionDefinition{
				Name:        "transfer_to_human",
				Description: "当需要转人工坐席时调用此工具",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"reason": map[string]interface{}{
							"type":        "string",
							"description": "转人工的原因",
						},
					},
					"required": []string{"reason"},
				},
			},
		},
	}

	// 示例：模拟用户提问（原有逻辑保持不变）
	userQuery := "你好，我订单123456什么时候能到啊？"
	fmt.Printf("用户: %s\n", userQuery)

	// 第一次请求大模型 ========== 改造点：Model参数从配置读取 ==========
	req := model.ChatCompletionRequest{
		Model: cfg.ModelEndPoint,
		Messages: []*model.ChatCompletionMessage{
			{Role: model.ChatMessageRoleSystem, Content: &model.ChatCompletionMessageContent{StringValue: volcengine.String(SystemPrompt)}},
			{Role: model.ChatMessageRoleUser, Content: &model.ChatCompletionMessageContent{StringValue: volcengine.String(userQuery)}},
		},
		Tools: tools,
	}

	resp, err := client.CreateChatCompletion(ctx, req)
	if err != nil {
		panic(fmt.Sprintf("大模型调用失败: %v", err))
	}

	choice := resp.Choices[0]
	// 判断大模型是否要调用工具（原有逻辑保持不变）
	if choice.FinishReason == "tool_calls" {
		req.Messages = append(req.Messages, &choice.Message)

		for _, toolCall := range choice.Message.ToolCalls {
			funcName := toolCall.Function.Name
			var args map[string]interface{}
			_ = json.Unmarshal([]byte(toolCall.Function.Arguments), &args)

			var toolResult string
			switch funcName {
			case "get_logistics_info":
				// 增加安全判断，避免大模型返回参数异常导致panic
				orderId, ok := args["order_id"].(string)
				if !ok {
					toolResult = "请提供需要查询的订单号哦~"
				} else {
					toolResult = getLogisticsInfo(orderId)
				}
			case "transfer_to_human":
				reason, ok := args["reason"].(string)
				if !ok {
					reason = "用户需求超出自动客服能力范围"
				}
				toolResult = transferToHuman(reason)
			default:
				toolResult = "工具不存在"
			}

			fmt.Printf("[系统] 调用工具%s, 参数:%v, 返回结果:%s\n", funcName, args, toolResult)
			req.Messages = append(req.Messages, &model.ChatCompletionMessage{
				Role:       model.ChatMessageRoleTool,
				ToolCallId: toolCall.Id,
				Content:    &model.ChatCompletionMessageContent{StringValue: volcengine.String(toolResult)},
			})
		}

		// 生成最终回复（原有逻辑保持不变）
		resp2, err := client.CreateChatCompletion(ctx, req)
		if err != nil {
			panic(fmt.Sprintf("生成最终回复失败: %v", err))
		}
		fmt.Printf("客服: %s\n", *resp2.Choices[0].Message.Content.StringValue)
	} else {
		fmt.Printf("客服: %s\n", *choice.Message.Content.StringValue)
	}
}
