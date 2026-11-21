package openaicompat

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"yhagent-server/internal/data/deltaapi"
)

var eventFrameHeader = regexp.MustCompile(`^\[\d+ new events?\]\r?\n`)

// Cortico sends external text as an evf_ tool receipt and the batch clock as a user message.
func turnInput(messages []WireMessage, continuing bool) string {
	for i := len(messages) - 1; i >= 0; i-- {
		m := messages[i]
		text := strings.TrimSpace(contentString(m.Content))
		switch m.Role {
		case "user":
			if text != "" && !strings.HasPrefix(text, "[system]") {
				return text
			}
		case "tool":
			if (m.Name == "external_event_frame" || strings.HasPrefix(m.ToolCallID, "evf_")) && eventFrameHeader.MatchString(text) {
				return eventFrameHeader.ReplaceAllString(text, "")
			}
		case "assistant":
			if !continuing {
				return ""
			}
		}
	}
	return ""
}

type petReply struct {
	Script   string `json:"script"`
	Details  string `json:"details"`
	Question string `json:"question"`
}

func clientHasDetails(req ChatRequest) bool {
	for _, tool := range req.Tools {
		if tool.Function != nil && tool.Function.Name == "pet_say" {
			var schema struct {
				Properties map[string]json.RawMessage `json:"properties"`
			}
			if json.Unmarshal(tool.Function.Parameters, &schema) == nil && len(schema.Properties["details"]) > 0 {
				return true
			}
		}
	}
	return false
}

func dailyPriceReply(maps []deltaapi.DailyKeyPrices, question string) (string, string) {
	var details strings.Builder
	var samples []string
	for _, table := range maps {
		var named []deltaapi.DailyKeyPrice
		for _, card := range table.Cards {
			if strings.Contains(question, card.Name) {
				named = append(named, card)
			}
		}
		cards := table.Cards
		if len(named) > 0 {
			cards = named
		}
		if len(cards) == 0 {
			continue
		}
		if details.Len() > 0 {
			details.WriteString("\n\n")
		}
		fmt.Fprintf(&details, "%s · 每日预测低价（哈夫币）", table.Map)
		for i, card := range cards {
			fmt.Fprintf(&details, "\n%s　%d　%02d:00", card.Name, card.Price, card.Hour)
			if i < 2 && len(samples) < 3 {
				samples = append(samples, fmt.Sprintf("%s%s，%d 哈夫币，%d 点", table.Map, card.Name, card.Price, card.Hour))
			}
		}
	}
	if len(samples) == 0 {
		return "", ""
	}
	return "预测低价：" + strings.Join(samples, "；") + "。", details.String()
}

func withDailyDetails(arguments, script, details string) string {
	var fields map[string]json.RawMessage
	if json.Unmarshal([]byte(arguments), &fields) != nil || fields == nil {
		fields = make(map[string]json.RawMessage)
	}
	var current string
	_ = json.Unmarshal(fields["script"], &current)
	// Keep a specific spoken reply. A generic announcement still needs actual prices for listeners.
	mentionsCard := false
	for _, line := range strings.Split(details, "\n") {
		if name, _, row := strings.Cut(line, "　"); row && strings.Contains(current, name) {
			mentionsCard = true
			break
		}
	}
	if !mentionsCard || !strings.ContainsAny(current, "0123456789０１２３４５６７８９") {
		fields["script"], _ = json.Marshal(script)
	}
	fields["details"], _ = json.Marshal(details)
	encoded, _ := json.Marshal(fields)
	return string(encoded)
}
