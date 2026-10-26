// internal/gunparts/tools.go
//
// gun_combo_solve 工具封装：把组合枚举求解器暴露给 Agent。
// 模型负责"识别组合问题 + 翻译约束参数"，工具负责"完备枚举不漏解"。
package gunparts

import (
	"context"
	"encoding/json"
	"fmt"

	"yhagent-server/internal/ai"
)

// solveParams gun_combo_solve 的 Function Call 参数
type solveParams struct {
	Weapon string `json:"weapon"`
	Attr   string `json:"attr"`
	Op     string `json:"op"`
	Value  *int   `json:"value"`
	Mode   string `json:"mode"`
	TopN   int    `json:"top_n"`
}

var validAttrs = map[string]bool{
	"stability": true, "recoilControl": true, "handling": true,
	"waistAccuracy": true, "effectiveRange": true,
}

// opSymbol 约束方向的显示符号
var opSymbol = map[Op]string{OpGte: "≥", OpLte: "≤", OpEq: "="}

// RegisterTools 注册 gun_combo_solve 工具（数据文件缺失时不注册，调用方保证 Store 非 nil）
func RegisterTools(reg *ai.Registry, st *Store) {
	reg.Register(ai.NewFunctionTool(
		"gun_combo_solve",
		"改枪组合枚举求解器（基于完整配件兼容数据）。枚举指定枪械的全部合法装配组合，返回满足属性约束的所有方案统计。"+
			"当用户问『某枪满足某条件的所有改法/能到多高/有哪些组合』（如『M14据枪+8的所有改枪方式』→ weapon=M14, attr=stability, value=8）时必须使用本工具，"+
			"不要只罗列社区改枪方案。返回：满足总数、最优Top-N方案（每槽配件名）、每槽可行配件分布。"+
			"约束基于配件修正值总和（游戏改枪面板显示的 +/- 数值），非武器最终面板值；百分比修正不计入。",
		map[string]any{
			"weapon": ai.StrProp("武器名称或ID（如 M14、K416），支持部分匹配；命中多个时会返回候选列表"),
			"attr":   ai.EnumProp("面板属性：stability=据枪稳定性 recoilControl=后坐力控制 handling=操控速度 waistAccuracy=腰射精度 effectiveRange=有效射程", "stability", "recoilControl", "handling", "waistAccuracy", "effectiveRange"),
			"op":     ai.EnumProp("约束方向：gte=大于等于(默认) lte=小于等于 eq=等于", "gte", "lte", "eq"),
			"value":  ai.IntProp("约束目标值（据枪+8 → 8；负修正用负数，如 ≤-2 → value=-2, op=lte）"),
			"mode":   ai.EnumProp("数值模式（三模式配件修正基本相同，默认 sol）", "sol", "mp", "bd"),
			"top_n":  ai.IntProp("返回最优方案数量 1~20，默认 10"),
		},
		[]string{"weapon", "attr", "value"},
		func(ctx context.Context, raw json.RawMessage) (any, error) {
			var p solveParams
			if err := json.Unmarshal(raw, &p); err != nil {
				return nil, fmt.Errorf("参数解析失败: %w", err)
			}
			if p.Value == nil {
				return nil, fmt.Errorf("缺少约束目标值 value（如据枪+8 → value=8）")
			}
			if !validAttrs[p.Attr] {
				return nil, fmt.Errorf("未知属性 %q（支持 stability/recoilControl/handling/waistAccuracy/effectiveRange）", p.Attr)
			}
			if p.Mode == "" {
				p.Mode = "sol"
			}

			// 武器定位：0 命中报错；多命中返回候选列表引导模型重试
			w, candidates := st.FindWeapon(p.Weapon)
			if w == nil {
				if len(candidates) > 0 {
					return map[string]any{
						"ambiguous": true,
						"note":      "匹配到多把武器，请用完整名称重试",
						"candidates": candidates,
					}, nil
				}
				return nil, fmt.Errorf("未找到武器 %q，请确认名称（如 M14、K416、M7）", p.Weapon)
			}

			slots, cond := st.BuildSpace(w, p.Mode, p.Attr)
			op := Op(p.Op)
			res, err := Solve(slots, cond, op, float64(*p.Value), p.TopN)
			if err != nil {
				return nil, err
			}

			attrName := st.AttrName(p.Attr)
			sym := opSymbol[op]
			if sym == "" {
				sym = "≥"
			}
			category := w.CategoryName
			if category == "" {
				category = w.Category
			}
			return map[string]any{
				"weapon": map[string]any{"id": w.Id, "name": w.Name, "category": category},
				"mode":    p.Mode,
				"constraint": map[string]any{
					"attr": p.Attr, "attr_name": attrName,
					"op": string(op), "symbol": sym, "value": *p.Value,
					"basis": "配件修正值总和（改枪面板显示的 +/- 值），百分比修正不计入；含未安装槽位",
				},
				"stats": map[string]any{
					"matches":   res.Matches,
					"evaluated": res.Evaluated,
					"pruned":    res.Pruned,
					"elapsed_ms": res.ElapsedMs,
					"data_version": st.Version(),
				},
				"top_solutions": res.Top,
				"slot_insight":  res.Slots,
				"note":          fmt.Sprintf("共 %d 种组合满足『%s%s%d』（已枚举 %d 种组合，剪枝 %d 次）。top_solutions 为最优方案（每槽配件名）；slot_insight.feasible 为该槽在满足约束组合中出现过的选项数。",
					res.Matches, attrName, sym, *p.Value, res.Evaluated, res.Pruned),
			}, nil
		},
	))
}
