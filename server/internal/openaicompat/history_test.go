// internal/openaicompat/history_test.go
//
// 历史转换单测：以 JSON 往返形态断言方舟 SDK 消息结构
// （与 internal/ai/history.go 的 MarshalJSON→map 解析路线同款，不拆 Sum 类型分支）。
package openaicompat

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/volcengine/ark-runtime-go/arkruntime/model/chat"
)

// asObj 消息序列化为宽松 map（字段存在性/取值断言用）。
func asObj(t *testing.T, m chat.ChatCompletionRequestMessage) map[string]any {
	t.Helper()
	b, err := m.MarshalJSON()
	if err != nil {
		t.Fatalf("消息序列化失败: %v", err)
	}
	var obj map[string]any
	if err := json.Unmarshal(b, &obj); err != nil {
		t.Fatalf("消息 JSON 非法: %v\n%s", err, b)
	}
	return obj
}

// mustParse 把 JSON 字符串解析为 wire 消息序列（构造用例的标准入口）。
func mustParse(t *testing.T, in string) []WireMessage {
	t.Helper()
	var msgs []WireMessage
	if err := json.Unmarshal([]byte(in), &msgs); err != nil {
		t.Fatalf("用例 JSON 解析失败: %v\n%s", err, in)
	}
	return msgs
}

func TestParseHistorySystemExtraction(t *testing.T) {
	msgs := mustParse(t, `[
		{"role":"system","content":"你是桌宠 Coo"},
		{"role":"developer","content":"补充规则"},
		{"role":"user","content":"你好"}
	]`)
	h, err := ParseHistory(msgs)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if h.SystemText != "你是桌宠 Coo\n\n补充规则" {
		t.Fatalf("SystemText = %q", h.SystemText)
	}
	if len(h.Messages) != 1 {
		t.Fatalf("非 system 消息数 = %d", len(h.Messages))
	}
	obj := asObj(t, h.Messages[0])
	if obj["role"] != "user" || obj["content"] != "你好" {
		t.Fatalf("user 消息形态: %v", obj)
	}
}

func TestParseHistoryUserString(t *testing.T) {
	h, err := ParseHistory(mustParse(t, `[
		{"role":"user","content":"M14 改枪方案"}
	]`))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	obj := asObj(t, h.Messages[0])
	if obj["role"] != "user" || obj["content"] != "M14 改枪方案" {
		t.Fatalf("纯文本 user 消息: %v", obj)
	}
}

func TestParseHistoryUserMultimodalParts(t *testing.T) {
	h, err := ParseHistory(mustParse(t, `[
		{"role":"user","content":[
			{"type":"text","text":"识别这张图"},
			{"type":"image_url","image_url":{"url":"data:image/png;base64,QUJD"}},
			{"type":"image_url","image_url":{"url":"data:image/jpeg;base64,REVG","detail":"high"}}
		]}
	]`))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	obj := asObj(t, h.Messages[0])
	parts, ok := obj["content"].([]any)
	if !ok || len(parts) != 3 {
		t.Fatalf("多模态分片数异常: %v", obj["content"])
	}
	p0, _ := parts[0].(map[string]any)
	p1, _ := parts[1].(map[string]any)
	if p0["type"] != "text" || p0["text"] != "识别这张图" {
		t.Fatalf("文本分片: %v", p0)
	}
	if p1["type"] != "image_url" {
		t.Fatalf("图片分片: %v", p1)
	}
	iu, _ := p1["image_url"].(map[string]any)
	if iu["url"] != "data:image/png;base64,QUJD" {
		t.Fatalf("图片 URL: %v", iu)
	}
}

func TestParseHistoryUserImageLimit(t *testing.T) {
	var parts strings.Builder
	parts.WriteString(`[{"type":"text","text":"x"}`)
	for i := 0; i <= maxImagesPerUserMessage; i++ {
		parts.WriteString(`,{"type":"image_url","image_url":{"url":"data:image/png;base64,Q"}}`)
	}
	parts.WriteString(`]`)
	_, err := ParseHistory(mustParse(t, `[{"role":"user","content":`+parts.String()+`}]`))
	if err == nil || !strings.Contains(err.Error(), "超上限") {
		t.Fatalf("图片超限未报错: %v", err)
	}
}

func TestParseHistoryAssistantVariants(t *testing.T) {
	h, err := ParseHistory(mustParse(t, `[
		{"role":"assistant","content":"你好呀","reasoning_content":"思考片段"},
		{"role":"assistant","content":"","tool_calls":[
			{"id":"call_1","type":"function","function":{"name":"pet_say","arguments":"{\"script\":\"hi\"}"}}
		]},
		{"role":"assistant","content":"先说一句","tool_calls":[
			{"id":"call_2","type":"function","function":{"name":"pet_move_to","arguments":"{\"to\":\"center\"}"}},
			{"id":"call_3","type":"function","function":{"name":"web_search","arguments":"{}"}}
		]}
	]`))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(h.Messages) != 3 {
		t.Fatalf("消息数 = %d", len(h.Messages))
	}
	// 纯文本：reasoning_content 必须被剥离
	m0 := asObj(t, h.Messages[0])
	if _, ok := m0["reasoning_content"]; ok {
		t.Fatalf("reasoning_content 未剥离: %v", m0)
	}
	if m0["content"] != "你好呀" {
		t.Fatalf("正文: %v", m0)
	}
	// 纯工具调用
	m1 := asObj(t, h.Messages[1])
	calls1, _ := m1["tool_calls"].([]any)
	if len(calls1) != 1 {
		t.Fatalf("tool_calls 数: %v", m1)
	}
	// 正文+工具调用混合
	m2 := asObj(t, h.Messages[2])
	if m2["content"] != "先说一句" {
		t.Fatalf("混合消息正文: %v", m2)
	}
	calls2, _ := m2["tool_calls"].([]any)
	if len(calls2) != 2 {
		t.Fatalf("混合 tool_calls 数: %v", m2)
	}
	c0, _ := calls2[0].(map[string]any)
	fn, _ := c0["function"].(map[string]any)
	if c0["id"] != "call_2" || fn["name"] != "pet_move_to" || fn["arguments"] != `{"to":"center"}` {
		t.Fatalf("工具调用形态: %v", c0)
	}
}

func TestParseHistoryToolMessage(t *testing.T) {
	h, err := ParseHistory(mustParse(t, `[
		{"role":"tool","tool_call_id":"call_9","name":"pet_say","content":"{\"shown\":true}"}
	]`))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	obj := asObj(t, h.Messages[0])
	if obj["role"] != "tool" || obj["tool_call_id"] != "call_9" || obj["name"] != "pet_say" {
		t.Fatalf("tool 消息形态: %v", obj)
	}
	if obj["content"] != `{"shown":true}` {
		t.Fatalf("tool 结果: %v", obj["content"])
	}
}

func TestParseHistoryErrors(t *testing.T) {
	cases := []struct {
		name, in string
		want     string
	}{
		{"tool 缺 call_id", `[{"role":"tool","content":"x"}]`, "tool_call_id"},
		{"非法角色", `[{"role":"wizard","content":"x"}]`, "角色非法"},
		{"user content 非法数组", `[{"role":"user","content":[1,2,3]}]`, "解析失败"},
	}
	for _, c := range cases {
		_, err := ParseHistory(mustParse(t, c.in))
		if err == nil {
			t.Fatalf("%s: 未报错", c.name)
		}
		if c.want != "" && !strings.Contains(err.Error(), c.want) {
			t.Fatalf("%s: 错误 %q 不含 %q", c.name, err.Error(), c.want)
		}
	}
}

func TestParseHistoryEmptyAndNilContent(t *testing.T) {
	h, err := ParseHistory(nil)
	if err != nil || len(h.Messages) != 0 || h.SystemText != "" {
		t.Fatalf("空输入: %v %d %q", err, len(h.Messages), h.SystemText)
	}
	// user 空 content（Cortico 可能发 content:null 的消息）
	h2, err := ParseHistory(mustParse(t, `[{"role":"assistant","content":null}]`))
	if err != nil {
		t.Fatalf("null content 解析失败: %v", err)
	}
	m := asObj(t, h2.Messages[0])
	if m["role"] != "assistant" {
		t.Fatalf("assistant 消息: %v", m)
	}
}

func TestParseHistoryOrderPreserved(t *testing.T) {
	// 顺序是配对完整性的生命线：assistant(tool_calls) 必须紧跟对应 tool 结果
	h, err := ParseHistory(mustParse(t, `[
		{"role":"user","content":"q"},
		{"role":"assistant","tool_calls":[{"id":"a","type":"function","function":{"name":"pet_act","arguments":"{}"}}]},
		{"role":"tool","tool_call_id":"a","content":"ok"},
		{"role":"assistant","content":"done"}
	]`))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	roles := make([]string, 0, 4)
	for _, m := range h.Messages {
		roles = append(roles, asObj(t, m)["role"].(string))
	}
	want := []string{"user", "assistant", "tool", "assistant"}
	for i := range want {
		if roles[i] != want[i] {
			t.Fatalf("顺序 = %v", roles)
		}
	}
}
