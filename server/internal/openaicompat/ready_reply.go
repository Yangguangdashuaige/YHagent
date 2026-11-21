package openaicompat

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"yhagent-server/internal/data/deltaapi"
)

var (
	voiceQuestion   = regexp.MustCompile(`^\[voice\][^:：\r\n]*[:：]\s*`)
	danmakuQuestion = regexp.MustCompile(`^\[弹幕\|[^\]\r\n]*\]\s*`)
	// Restrict automatic delivery to one uncomplicated request. Preserve the model
	// path for comparisons, constraints, and batches containing other questions.
	directReadyQuestion = regexp.MustCompile(`(?i)^(?:请|你好[，, ]*)?那?(?:如果)?我?(?:如果)?(?:想|要)?(?:帮我)?(?:该?怎么)?(?:卡|凑|配|推荐|查一下)?到?(?:[1一]个|一套)?\s*([0-9]+(?:\.[0-9]+)?)\s*(万|w)?\s*的?战备[，,。\s]*(?:该?怎么(?:配装?|赔养|配养|搭配|弄)|可以吗)?(?:呢|呀|吧)?[？?!！。\s]*$`)
	petScriptText       = strings.NewReplacer("【", "〔", "】", "〕", "<", "〈", ">", "〉", "＜", "〈", "＞", "〉")
	spokenSlots         = strings.NewReplacer("枪械1", "一号枪", "枪械2", "二号枪", "枪1", "一号枪", "枪2", "二号枪", "-", "的")
)

func directReadyTarget(question string) int {
	question = strings.TrimSpace(question)
	if strings.ContainsAny(question, "\r\n") {
		return 0
	}
	question = voiceQuestion.ReplaceAllString(question, "")
	question = danmakuQuestion.ReplaceAllString(question, "")
	parts := directReadyQuestion.FindStringSubmatch(question)
	if parts == nil {
		return 0
	}
	value, err := strconv.ParseFloat(parts[1], 64)
	if parts[2] != "" {
		value *= 10000
	}
	if err != nil || value < 1500 || value > 780000 || math.Abs(value-math.Round(value)) > 1e-6 {
		return 0
	}
	return int(math.Round(value))
}

func readyArguments(arguments string, plan *deltaapi.ReadyPlan, target int, withDetails bool) string {
	var fields map[string]json.RawMessage
	if json.Unmarshal([]byte(arguments), &fields) != nil || fields == nil {
		fields = make(map[string]json.RawMessage)
	}
	var items []string
	var details strings.Builder
	fmt.Fprintf(&details, "目标战备 %d · 方案战备 %d · 预计花费 %d 哈夫币\n", target, plan.Value, plan.Price)
	for _, item := range plan.Items {
		slot := strings.Join(strings.Fields(item.Slot), " ")
		name := strings.Join(strings.Fields(item.Name), " ")
		spokenSlot := spokenSlots.Replace(slot)
		line := spokenSlot + "用" + name
		exchange := strings.TrimSpace(item.Exchange) != ""
		if exchange {
			line = spokenSlot + "兑换" + name
		}
		items = append(items, line)
		fmt.Fprintf(&details, "\n%s · %s", slot, name)
		if exchange {
			details.WriteString("（兑换）")
		}
		if item.Price > 0 {
			fmt.Fprintf(&details, " · %d 哈夫币", item.Price)
		}
	}
	details.WriteString("\n\n来源：实时战备配装工具。实际挂单价格可能变动。")
	script := fmt.Sprintf("这样配：%s。战备 %d，预计花 %d 哈夫币。", strings.Join(items, "，"), plan.Value, plan.Price)
	fields["script"], _ = json.Marshal(petScriptText.Replace(script))
	if withDetails {
		fields["details"], _ = json.Marshal(details.String())
	} else {
		delete(fields, "details")
	}
	encoded, _ := json.Marshal(fields)
	return string(encoded)
}

func petReplyDelivered(receipt string) bool {
	for _, prefix := range []string{"已开始显示", "已排队,前面还有约 ", "Showing now;", "Queued behind about ", "Saved answer details in the Chat page."} {
		if strings.HasPrefix(receipt, prefix) {
			return true
		}
	}
	return false
}
