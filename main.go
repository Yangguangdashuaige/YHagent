package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/volcengine/volcengine-go-sdk/service/arkruntime"
	"github.com/volcengine/volcengine-go-sdk/service/arkruntime/model"
	"github.com/volcengine/volcengine-go-sdk/volcengine"

	"YHagent/config"
)

// ==============================================
// 以下为原封不动保留的你原有代码（配置、Prompt、Mock工具）
// ==============================================
var client *arkruntime.Client
var cfg *config.Config

var (
	// 统计当前打开的聊天页面数量，支持多标签页场景（开多个标签页不会误退出）
	activePageCount int64 = 0
	// 最后一次收到前端心跳的时间
	lastHeartbeatTime = time.Now()
	// WebSocket升级器，因为是本地服务，不做跨域校验
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
)

// 启动心跳检测协程，在main函数启动服务之前调用即可
func startHeartbeatCheck(server *http.Server) {
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			// 规则：
			// 1. 如果没有任何打开的页面，直接退出
			// 2. 如果有页面，但连续30秒没收到心跳（说明页面关了/断网了），且没有活跃页面，退出
			if atomic.LoadInt64(&activePageCount) == 0 && time.Since(lastHeartbeatTime) > 30*time.Second {
				log.Println("检测到所有聊天页面已关闭，30秒无活动，程序自动退出")
				// 优雅关闭HTTP服务
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				_ = server.Shutdown(ctx)
				// 正常退出进程
				os.Exit(0)
			}
		}
	}()
}

// WebSocket心跳接口，挂到Gin路由上
func wsHeartbeat(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	// 新页面打开，活跃计数+1
	atomic.AddInt64(&activePageCount, 1)
	log.Printf("检测到聊天页面打开，当前活跃页面数:%d", atomic.LoadInt64(&activePageCount))
	// 页面关闭，计数-1
	defer atomic.AddInt64(&activePageCount, -1)

	// 循环读前端发的心跳消息
	for {
		var msg struct {
			Type string `json:"type"`
		}
		err := conn.ReadJSON(&msg)
		if err != nil {
			// 连接断开（页面关了/刷新了）直接退出循环
			return
		}
		if msg.Type == "ping" {
			// 更新最后心跳时间，给前端回pong
			lastHeartbeatTime = time.Now()
			_ = conn.WriteJSON(map[string]string{"type": "pong"})
		}
	}
}

// SystemPrompt 客服专属System Prompt
const SystemPrompt = `
你是XX淘宝店的智能客服小豆，需要严格遵守以下规则回答用户问题：
1. 身份：你是店铺的专属客服，语气亲切自然，符合淘宝客服的说话习惯，可以适当用😊这类表情，不要太生硬
2. 准确性：所有回答必须基于知识库检索结果、工具查询返回的真实数据，绝对不能编造信息，如果信息不足请直接说"非常抱歉这个问题我暂时无法解答，马上为您转接人工坐席~"
3. 工具使用：涉及订单、物流、商品库存、优惠金额的问题，必须调用对应的工具查询，绝对不能凭记忆回答
4. 风险控制：绝对不可以引导用户去微信、其他平台交易，不可以承诺超出店铺规则的权益（比如超出规则的退款、补偿）
5. 操作规则：涉及退款、改地址等修改类操作，必须先和用户确认信息，得到用户明确同意后再执行，严禁私自操作
6. 转人工：用户明确要求找人工、情绪愤怒、问题超出你能力范围时，直接调用转人工工具
`

// 工具1：查询物流信息（Mock，后续对接淘宝时替换成真实物流查询API）
func getLogisticsInfo(orderId string) string {
	return fmt.Sprintf("订单%s的物流信息：顺丰快递，单号SF123456789，目前已到达杭州转运中心，预计明天送达", orderId)
}

// 工具2：转人工（Mock，后续对接淘宝时替换成真实转人工逻辑）
func transferToHuman(reason string) string {
	return fmt.Sprintf("已为您转接人工坐席，转接原因：%s，请稍等~", reason)
}

// 工具3：查询订单详情（Mock，和淘宝接口参数对齐，后续直接替换）
func getOrderInfo(orderId string) string {
	return fmt.Sprintf("订单%s的详情：下单时间2024-06-01，实付金额99元，商品为白色纯棉M码T恤，当前状态为已发货", orderId)
}

// 工具4：查询商品详情（Mock，和淘宝接口参数对齐，后续直接替换）
func getProductInfo(itemId string) string {
	return fmt.Sprintf("商品%s的详情：夏季纯棉短袖T恤，售价99元，目前M码有货，支持7天无理由退换，发货地浙江杭州，默认发顺丰快递", itemId)
}

// ==============================================
// 以下为新加的Agent核心逻辑（多轮记忆、自动工具调用循环）
// ==============================================
// 全局会话上下文：本地测试单用户场景用内存存储即可，后续多用户/接淘宝时按用户ID存在Redis即可
var chatHistory []*model.ChatCompletionMessage

const maxHistoryRound = 20 // 最多保留20轮对话，避免Token超限

// 注册给豆包的工具列表（所有工具和后续淘宝接口参数完全一致）
var tools = []*model.Tool{
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
	{
		Type: model.ToolTypeFunction,
		Function: &model.FunctionDefinition{
			Name:        "get_order_info",
			Description: "当用户需要查询订单状态、金额、商品信息、收货信息时调用此工具",
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
			Name:        "get_product_info",
			Description: "当用户需要咨询商品参数、库存、价格、尺码、发货规则时调用此工具",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"item_id": map[string]interface{}{
						"type":        "string",
						"description": "用户要咨询的商品ID",
					},
				},
				"required": []string{"item_id"},
			},
		},
	},
}

// 通用Chat函数：接收用户输入，自动处理工具调用，返回最终客服回复
// 👉 后续对接淘宝时直接调用这个函数即可，不用改任何内部逻辑
func Chat(ctx context.Context, userInput string) (string, error) {
	// 1. 把用户新消息追加到历史上下文
	chatHistory = append(chatHistory, &model.ChatCompletionMessage{
		Role:    model.ChatMessageRoleUser,
		Content: &model.ChatCompletionMessageContent{StringValue: volcengine.String(userInput)},
	})

	// 2. 循环处理大模型请求/工具调用（支持连续多次工具调用）
	for {
		// 控制上下文长度，超过最大轮数就截断最早的对话（保留SystemPrompt）
		if len(chatHistory) > maxHistoryRound+1 {
			chatHistory = append([]*model.ChatCompletionMessage{chatHistory[0]}, chatHistory[len(chatHistory)-maxHistoryRound:]...)
		}

		req := model.ChatCompletionRequest{
			Model:    cfg.ModelEndPoint,
			Messages: chatHistory,
			Tools:    tools,
		}

		resp, err := client.CreateChatCompletion(ctx, req)
		if err != nil {
			log.Printf("大模型调用失败: %v", err)
			return "非常抱歉，客服暂时有点小问题，请稍后再试哦~", nil
		}

		choice := resp.Choices[0]
		// 如果模型返回的是最终文本回复，直接返回结果
		if choice.FinishReason == "stop" {
			replyContent := *choice.Message.Content.StringValue
			// 把助手回复追加到历史记录
			chatHistory = append(chatHistory, &choice.Message)
			return replyContent, nil
		}

		// 如果模型要求调用工具，执行工具并把结果追加到上下文，继续下一轮循环
		if choice.FinishReason == "tool_calls" {
			chatHistory = append(chatHistory, &choice.Message)
			log.Printf("[Agent调用工具] 共%d个工具需要执行", len(choice.Message.ToolCalls))

			for _, toolCall := range choice.Message.ToolCalls {
				funcName := toolCall.Function.Name
				var args map[string]interface{}
				_ = json.Unmarshal([]byte(toolCall.Function.Arguments), &args)

				var toolResult string
				// ==============================================
				// 👉 后续对接淘宝API时，只需要修改下面这个switch里的实现
				// 把Mock返回换成真实淘宝接口调用即可，其他逻辑不用动
				// ==============================================
				switch funcName {
				case "get_logistics_info":
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
				case "get_order_info":
					orderId, ok := args["order_id"].(string)
					if !ok {
						toolResult = "请提供需要查询的订单号哦~"
					} else {
						toolResult = getOrderInfo(orderId)
					}
				case "get_product_info":
					itemId, ok := args["item_id"].(string)
					if !ok {
						toolResult = "请提供需要咨询的商品ID哦~"
					} else {
						toolResult = getProductInfo(itemId)
					}
				default:
					toolResult = "工具不存在"
				}

				log.Printf("[工具执行] 函数:%s, 参数:%v, 结果:%s", funcName, args, toolResult)
				// 把工具结果追加到上下文
				chatHistory = append(chatHistory, &model.ChatCompletionMessage{
					Role:       model.ChatMessageRoleTool,
					ToolCallID: toolCall.ID,
					Content:    &model.ChatCompletionMessageContent{StringValue: volcengine.String(toolResult)},
				})
			}
			// 执行完工具后继续下一轮请求大模型
			continue
		}

		// 其他异常情况兜底
		return "非常抱歉，我暂时无法理解您的问题，马上为您转接人工~", nil
	}
}

// 自动打开浏览器的工具函数
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	default:
		return
	}
	_ = cmd.Start()
}

func getAvailablePort() (int, error) {
	for port := 8080; port < 8100; port++ {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err == nil {
			_ = ln.Close()
			return port, nil
		}
	}
	return 0, fmt.Errorf("8080-8100端口全部被占用，请关闭占用端口的程序后重试")
}

func main() {
	logFile, err := os.OpenFile("agent.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err == nil {
		log.SetOutput(io.MultiWriter(os.Stdout, logFile)) // 控制台+文件双写
	}
	log.Println("🚀 智能客服程序启动中...")
	// 1. 加载配置（原有逻辑保留）
	cfg, err = config.Load()
	if err != nil {
		log.Fatalf("配置初始化失败: %v", err)
	}

	// 2. 初始化豆包客户端（原有逻辑保留）
	client = arkruntime.NewClientWithApiKey(
		cfg.ArkAPIKey,
		arkruntime.WithBaseUrl(cfg.ArkBaseUrl),
	)

	// 3. 初始化会话上下文，第一步放入System Prompt
	chatHistory = append(chatHistory, &model.ChatCompletionMessage{
		Role:    model.ChatMessageRoleSystem,
		Content: &model.ChatCompletionMessageContent{StringValue: volcengine.String(SystemPrompt)},
	})
	log.Println("✅ 豆包Agent核心逻辑初始化成功，所有工具已加载")

	// 4. 启动本地Web服务
	r := gin.Default()
	// 本地测试关闭Gin冗余Debug日志，控制台更清爽
	gin.SetMode(gin.ReleaseMode)

	r.StaticFile("/", "./static/index.html")

	// 路由2：聊天接口，接收前端消息，调用Agent返回结果
	r.POST("/api/chat", func(c *gin.Context) {
		// 单请求超时30秒，避免大模型调用异常导致请求无限挂死
		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()

		var req struct {
			Message string `json:"message" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "消息格式不正确"})
			return
		}

		// 直接调用通用Chat方法，核心Agent逻辑和入口完全解耦
		reply, err := Chat(ctx, req.Message)
		if err != nil {
			log.Printf("Agent响应失败: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"reply": "非常抱歉，客服暂时有点小问题，请稍后再试哦~"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"reply": reply})
	})

	// 5. 启动本地服务，仅监听127.0.0.1，避免公网暴露测试服务
	port, err := getAvailablePort()
	if err != nil {
		log.Fatalf("启动失败: %v", err)
	}
	r.GET("/ws/heartbeat", wsHeartbeat)

	listenAddr := fmt.Sprintf("127.0.0.1:%d", port)
	// 注意：因为要手动优雅关闭服务，不要直接用r.Run，改成手动创建http.Server
	server := &http.Server{
		Addr:    listenAddr,
		Handler: r,
	}

	// 启动心跳自动退出检测
	startHeartbeatCheck(server)

	// 后面自动打开浏览器的逻辑保持不变
	go func() {
		time.Sleep(1 * time.Second)
		openBrowser(fmt.Sprintf("http://%s", listenAddr))
	}()

	// 启动服务
	log.Printf("服务监听地址: %s", listenAddr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("服务启动失败: %v", err)
	}

	go func() {
		// 等待服务启动1秒后自动打开浏览器
		time.Sleep(1 * time.Second)
		localUrl := fmt.Sprintf("http://%s", listenAddr)
		log.Printf("🚀 本地测试服务已启动，访问地址：%s", localUrl)
		log.Println("正在自动打开浏览器聊天窗口... 如果没有弹出请手动复制上面的地址访问")
		openBrowser(localUrl)
	}()

	// 启动HTTP服务
	if err := r.Run(listenAddr); err != nil {
		log.Fatalf("服务启动失败，可能是端口8080被占用：%v", err)
	}
}
