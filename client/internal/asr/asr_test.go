// asr_test.go 听写协议解析单测（纯函数；SAPI 进程链路属真机冒烟范围）
package asr

import "testing"

func TestParseLine(t *testing.T) {
	cases := []struct {
		name    string
		line    string
		want    Event
		wantErr bool
	}{
		{"ready", `{"type":"ready"}`, Event{Type: "ready"}, false},
		{"partial", `{"type":"partial","text":"今天金"}`, Event{Type: "partial", Text: "今天金"}, false},
		{"final", `{"type":"final","text":"今天金价多少","confidence":0.87}`,
			Event{Type: "final", Text: "今天金价多少", Confidence: 0.87}, false},
		{"done完成", `{"type":"done","reason":"completed"}`, Event{Type: "done", Reason: "completed"}, false},
		{"done错误", `{"type":"done","reason":"error","detail":"audio device"}`,
			Event{Type: "done", Reason: "error", Detail: "audio device"}, false},
		{"非JSON", `not json`, Event{}, true},
		{"未知类型", `{"type":"foo"}`, Event{}, true},
		{"空行", ``, Event{}, true},
	}
	for _, c := range cases {
		got, err := ParseLine(c.line)
		if c.wantErr {
			if err == nil {
				t.Errorf("%s：应返回错误", c.name)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s：意外错误 %v", c.name, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s：got %+v want %+v", c.name, got, c.want)
		}
	}
}

func TestParseLineUnicodeEscape(t *testing.T) {
	// PowerShell 5.1 ConvertTo-Json 把中文转义为 \uXXXX
	e, err := ParseLine(`{"type":"final","text":"\u4eca\u5929","confidence":0.9}`)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if e.Text != "今天" {
		t.Fatalf("Unicode 转义还原失败：%q", e.Text)
	}
}
