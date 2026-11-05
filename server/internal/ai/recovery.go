// internal/ai/recovery.go
//
// 崩溃恢复的"最后一公里"：把 WAL 回放得到的半截回答写回会话持久化历史，
// 让用户刷新页面（纯历史加载、无活动 SSE）时也能看到已生成的内容，
// 并明确标记为中断，避免被误当成完整回答。
//
// 幂等性保证在上游：TurnStore.RecoverIncomplete 扫描后会给对应 WAL 追加
// interrupted 终止标记，再次扫描不会重复返回，因此本函数不会重复追加。
package ai

import (
	"log"
	"strings"
)

// AppendRecoveredAnswer 把恢复得到的半截回答追加到会话持久化历史。
// store 为 nil（纯内存模式）时直接返回——纯内存模式下重启即失忆，
// 客户端只能凭 WAL 回放当前进程内的中断回合，无法跨进程恢复历史。
func AppendRecoveredAnswer(store SessionStore, sessionID, answer string) {
	if store == nil || sessionID == "" {
		return
	}
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return // 没有已生成内容（如崩溃发生在工具执行阶段）：会话已在轮首持久化了用户问题
	}
	msgs, ok, err := store.Load(sessionID)
	if err != nil || !ok {
		return
	}
	marked := "（回答中断，以下为服务重启前已生成的内容）\n" + answer
	msg := NewAssistantTextMessage(marked)
	b, err := msg.MarshalJSON()
	if err != nil {
		return
	}
	if err := store.Save(sessionID, append(msgs, b)); err != nil {
		log.Printf("[recovery] 会话 %s 恢复回答落盘失败: %v", sessionID, err)
	}
}
