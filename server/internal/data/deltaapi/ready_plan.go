package deltaapi

import (
	"encoding/json"
	"strings"
)

// ReadyPlan is the common plan shape returned by the live and DIY loadout APIs.
type ReadyPlan struct {
	Name  string      `json:"name,omitempty"`
	Value int         `json:"jz"`
	Price int         `json:"price"`
	Items []ReadyItem `json:"data"`
}

type ReadyItem struct {
	Slot          string          `json:"type"`
	Name          string          `json:"name"`
	Price         int             `json:"price"`
	Exchange      string          `json:"exchange,omitempty"`
	ExchangeRules json.RawMessage `json:"exchange_plus,omitempty"`
}

type readyGroup struct {
	Name    string      `json:"name"`
	Minimum int         `json:"zb"`
	Updated string      `json:"dateT,omitempty"`
	Plans   []ReadyPlan `json:"list"`
}

// CompactReadyV4 keeps the leading complete plans from each strategy for model
// comparisons when the upstream result exceeds the chat tool's context limit.
func CompactReadyV4(raw json.RawMessage) json.RawMessage {
	var groups []readyGroup
	if json.Unmarshal(raw, &groups) != nil {
		return raw
	}
	selected := make([]readyGroup, 0, len(groups))
	for _, group := range groups {
		plans := group.Plans[:0]
		for _, plan := range group.Plans {
			if plan.Value > 0 && plan.Price > 0 && plan.hasEveryItem(true) {
				plans = append(plans, plan)
				if len(plans) == 2 {
					break
				}
			}
		}
		if len(plans) > 0 {
			group.Plans = plans
			selected = append(selected, group)
		}
	}
	if len(selected) == 0 {
		return raw
	}
	compact, _ := json.Marshal(struct {
		Groups []readyGroup `json:"groups"`
		Note   string       `json:"note"`
	}{selected, "保留每种配装策略最多两套，明细按方案列出。"})
	return compact
}

func (plan ReadyPlan) hasEveryItem(allowExchange bool) bool {
	for _, item := range plan.Items {
		if strings.TrimSpace(item.Name) == "" || strings.TrimSpace(item.Slot) == "" || (!allowExchange && strings.TrimSpace(item.Exchange) != "") {
			return false
		}
	}
	return len(plan.Items) > 0
}

// SelectReadyPlan returns a complete qualifying plan. For a live market query,
// select by cash cost across groups and retain each gun's own attachment slots.
func SelectReadyPlan(tool string, args, payload json.RawMessage, target int) *ReadyPlan {
	var best *ReadyPlan
	consider := func(plan *ReadyPlan, minimum int, exchange bool) {
		if plan.Value < minimum || plan.Price <= 0 || (best != nil && plan.Price >= best.Price) || !plan.hasEveryItem(exchange) {
			return
		}
		best = plan
	}
	if target < 1500 || target > 780000 {
		return nil
	}
	switch tool {
	case "delta_diy_ready_build":
		var req DIYReadyBuildParams
		var plan ReadyPlan
		if json.Unmarshal(args, &req) == nil && req.Zb == target && json.Unmarshal(payload, &plan) == nil {
			consider(&plan, target, req.Exchange == 1)
		}
	case "delta_ready_build_v4", "delta_ready_build_v3", "delta_ready_build_v3_plus":
		var req struct {
			Lv *int `json:"lv"`
		}
		if json.Unmarshal(args, &req) != nil || req.Lv == nil {
			return nil
		}
		switch *req.Lv {
		case 0, 1, 2, 3, 5:
		default:
			return nil
		}
		nominal := [...]int{110000, 180000, 550000, 600000, 0, 780000}[*req.Lv]
		if tool == "delta_ready_build_v4" {
			var groups []readyGroup
			if json.Unmarshal(payload, &groups) != nil {
				return nil
			}
			for _, group := range groups {
				if (target != nominal && target != group.Minimum) || strings.Contains(group.Name, "兑换") {
					continue
				}
				for i := range group.Plans {
					consider(&group.Plans[i], max(target, group.Minimum), false)
				}
			}
		} else if target == nominal {
			var result struct {
				Plans []ReadyPlan `json:"data"`
			}
			if json.Unmarshal(payload, &result) == nil {
				for i := range result.Plans {
					consider(&result.Plans[i], target, tool == "delta_ready_build_v3_plus")
				}
			}
		}
	}
	return best
}
