// internal/ai/service.go
package ai

import "context"

// ChatService 是 UI 层依赖的对话服务抽象（当前的 Wails 窗口直接用，未来的瘦客户端网络层也只认它）。
//
// 演进路线：
//   - 现在：本地 Agent（*Agent）天然实现本接口，所有能力在客户端进程内完成；
//   - 将来：新增一个基于 HTTPS/SSE 的远端实现（如 RemoteChatService），在启动时注入即可，
//     Wails 绑定层、前端界面与事件协议一行都不用改，RAG/向量/缓存全部收敛到服务器。
type ChatService interface {
	// ChatWithSession 在指定会话中跑完一整轮 Function Call 流式对话，
	// 模型每产生一段回答文本，就通过 onChunk 推给调用方。
	ChatWithSession(ctx context.Context, sessionID, userText string, onChunk func(content string)) error

	// ChatWithSessionImages 与 ChatWithSession 相同，但允许随消息携带图片（截图识别）。
	// images 为 data URL（data:image/jpeg;base64,...），无图片时传 nil。
	ChatWithSessionImages(ctx context.Context, sessionID, userText string, images []string, onChunk func(content string)) error

	// Reset 清空指定会话的历史上下文。
	Reset(sessionID string)
}
