// internal/remote/client_test.go
//
// parseSSE 是断线续传对齐的核心：trace 捕获 turn_id、内容事件（chunk/thought）
// 计数、interrupted 事件转 *InterruptedError。这里用内存 SSE 流覆盖关键行为，
// 不依赖网络。
package remote

import (
	"errors"
	"strings"
	"testing"
)

// sseBlock 把一个事件打包成 W3C SSE 格式（event + data + 空行）
func sseBlock(event, data string) string {
	if data == "" {
		return "event: " + event + "\n\n"
	}
	return "event: " + event + "\ndata: " + data + "\n\n"
}

func TestParseSSE_ChunkCountsAndCallsBack(t *testing.T) {
	var chunks []string
	content := 0
	stream := sseBlock("chunk", `"你好"`) + sseBlock("chunk", `"世界"`) + sseBlock("done", "")

	err := parseSSE(strings.NewReader(stream),
		func(s string) { chunks = append(chunks, s) },
		nil, nil, func() { content++ })

	if err != nil {
		t.Fatalf("期望 nil，得到 %v", err)
	}
	if len(chunks) != 2 || chunks[0] != "你好" || chunks[1] != "世界" {
		t.Fatalf("chunk 回调异常: %v", chunks)
	}
	// 每个 chunk 事件都计为一条内容（与服务端 WAL 内容计数对齐）
	if content != 2 {
		t.Fatalf("期望内容计数 2，得到 %d", content)
	}
}

func TestParseSSE_ThoughtCountsButNoChunk(t *testing.T) {
	var chunks []string
	content := 0
	// 先 chunk 再 thought：思考模型推理流只计数、不进回答正文
	stream := sseBlock("chunk", `"答"`) + sseBlock("thought", `{"text":"推理中"}`) + sseBlock("done", "")

	err := parseSSE(strings.NewReader(stream),
		func(s string) { chunks = append(chunks, s) },
		nil, nil, func() { content++ })

	if err != nil {
		t.Fatalf("期望 nil，得到 %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("期望 1 个 chunk 回调，得到 %d", len(chunks))
	}
	if content != 2 {
		t.Fatalf("期望内容计数 2（chunk+thought），得到 %d", content)
	}
}

func TestParseSSE_TraceCapturesTurnID(t *testing.T) {
	var traceID string
	stream := sseBlock("trace", `{"trace_id":"t_abc123"}`) + sseBlock("done", "")

	err := parseSSE(strings.NewReader(stream), nil, nil,
		func(id string) { traceID = id }, nil)

	if err != nil {
		t.Fatalf("期望 nil，得到 %v", err)
	}
	if traceID != "t_abc123" {
		t.Fatalf("期望 turn_id t_abc123，得到 %q", traceID)
	}
}

func TestParseSSE_InterruptedReturnsTypedError(t *testing.T) {
	stream := sseBlock("chunk", `"半截"`) +
		sseBlock("interrupted", `{"reason":"服务重启","turn_id":"t_deadbeef"}`)

	err := parseSSE(strings.NewReader(stream), nil, nil, nil, nil)

	var ie *InterruptedError
	if !errors.As(err, &ie) {
		t.Fatalf("期望 *InterruptedError，得到 %T: %v", err, err)
	}
	if ie.Reason != "服务重启" || ie.TurnID != "t_deadbeef" {
		t.Fatalf("InterruptedError 字段异常: reason=%q turn_id=%q", ie.Reason, ie.TurnID)
	}
}

func TestParseSSE_ErrorEventReturnsMessage(t *testing.T) {
	stream := sseBlock("error", `"上游挂了"`)
	err := parseSSE(strings.NewReader(stream), nil, nil, nil, nil)
	if err == nil || err.Error() != "上游挂了" {
		t.Fatalf("期望错误「上游挂了」，得到 %v", err)
	}
}

func TestInterruptedError_NilSafe(t *testing.T) {
	var ie *InterruptedError
	// nil 接收者不应 panic
	if msg := ie.Error(); msg != "回答中断" {
		t.Fatalf("nil 接收者期望「回答中断」，得到 %q", msg)
	}
}
