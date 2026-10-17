// cmd/itemsync/render.go
//
// 把全物品基础信息渲染成"一件物品一篇 Markdown"：
// 每篇控制在约一个切片（600字内），静态属性中文化；
// 枪械适配弹药通过 objectID 在全量物品中反查名称；
// 配件槽位 ID（改枪配件方案）等无语义字段不入库——那是在线改枪工具的领域。
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// gradeNames 品质等级 -> 颜色俗称（三角洲行动：白绿蓝紫金红）
var gradeNames = map[int]string{
	0: "无等级",
	1: "白品",
	2: "绿品",
	3: "蓝品",
	4: "紫品",
	5: "金品",
	6: "红品",
}

var primaryClassCN = map[string]string{
	"acc":      "武器配件",
	"ammo":     "弹药",
	"gun":      "枪械",
	"protect":  "防护装备",
	"props":    "道具物资",
	"exchange": "兑换/收藏品",
}

// fieldLabels 各一级分类下 detail 字段的中文名（未覆盖的字段原样输出）
var fieldLabels = map[string]map[string]string{
	"ammo": {
		"penetrationLevel": "穿透等级",
		"harmRatio":        "基础伤害倍率",
		"armorHarmLevel":   "对护甲伤害能力",
	},
	"gun": {
		"caliber":        "口径",
		"capacity":       "弹匣容量",
		"fireMode":       "开火模式",
		"fireSpeed":      "射速",
		"armorHarm":      "装甲伤害",
		"meatHarm":       "肉体伤害",
		"muzzleVelocity": "枪口初速",
		"shootDistance":  "有效射击距离",
		"soundDistance":  "枪声传播距离",
		"hipShot":        "腰射稳定性",
		"control":        "操控性",
		"recoil":         "后坐控制力",
		"stable":         "据枪稳定性",
	},
	"acc": {
		"bombCapacity":        "弹匣容量加成",
		"controlSpeed":        "操控速度修正",
		"controlStable":       "据枪稳定性修正",
		"hipShot":             "腰射稳定修正",
		"recoil":              "后坐控制修正",
		"shotDistancePercent": "射程修正",
		"quickSeparate":       "快速分离",
	},
	"protect": {
		"durability":       "耐久度",
		"durableLoss":      "耐久损耗速度",
		"protectLevel":     "防护等级",
		"protectArea":      "防护区域",
		"repairEfficiency": "装备特性",
		"capacity":         "容量",
		"aimSpeed":         "瞄准速度影响",
		"moveSpeed":        "移动速度影响",
		"sprintCost":       "冲刺体力消耗影响",
		"soundEffect":      "听觉影响",
		"faceMask":         "面罩特性",
	},
	"props": {
		"type":             "类型",
		"activeTime":       "生效时间",
		"availableCount":   "可用次数",
		"durability":       "耐久度",
		"useMap":           "使用地图",
		"usePlace":         "使用地点",
		"propsSource":      "获取来源",
		"keyCardType":      "钥匙卡类型",
		"replyEffect":      "回复效果",
		"repairArea":       "修复部位",
		"repairEfficiency": "修复效果",
		"repairPoints":     "修复点数",
		"bearEnhance":      "负重增益",
		"bodyCapacity":     "体力容量增益",
		"hearEnhance":      "听力增益",
	},
}

// renderItems 清掉旧的物品文档后全量重生成，返回写入篇数
func renderItems(outDir string, items []itemInfo) (int, error) {
	// 重新生成前清理上一版物品文档（已下架物品不应残留）
	old, _ := filepath.Glob(filepath.Join(outDir, "物品-*.md"))
	for _, f := range old {
		if err := os.Remove(f); err != nil {
			return 0, fmt.Errorf("清理旧物品文档 %s: %w", f, err)
		}
	}

	// objectID -> 名称（枪械 detail.ammo 反查用）
	nameByObjectID := make(map[int64]string, len(items))
	for _, it := range items {
		if it.ObjectID != 0 {
			nameByObjectID[it.ObjectID] = it.ObjectName
		}
	}

	used := map[string]int{} // 文件名去重计数
	n := 0
	for _, it := range items {
		if strings.TrimSpace(it.ObjectName) == "" {
			continue
		}
		content := renderOne(it, nameByObjectID)
		name := safeFileName(it.ObjectName, used)
		if err := os.WriteFile(filepath.Join(outDir, name), []byte(content), 0o644); err != nil {
			return n, fmt.Errorf("写入 %s: %w", name, err)
		}
		n++
	}
	return n, nil
}

func renderOne(it itemInfo, nameByObjectID map[int64]string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", it.ObjectName)

	primary := primaryClassCN[it.PrimaryClass]
	if primary == "" {
		primary = it.PrimaryClass
	}
	sub := it.SecondClass // 实测配件/枪械/防护/道具为中文，弹药为口径
	if sub == "" || strings.HasPrefix(strings.ToLower(sub), "ammo") {
		sub = firstNonEmpty(it.SecondClass, it.SecondClassCN)
	}
	grade := fmt.Sprintf("%d级（%s）", it.Grade, gradeNames[it.Grade])
	trade := "不可交易"
	if it.IsGet == 1 {
		trade = "可在交易行交易"
	}
	fmt.Fprintf(&b, "> 物品图鉴 | 分类：%s/%s | 品质：%s | %s | 仓库占地：%d×%d格\n\n",
		primary, sub, grade, trade, it.Width, it.Length)

	if desc := strings.TrimSpace(it.Desc); desc != "" {
		fmt.Fprintf(&b, "%s\n\n", desc)
	}

	detail := parseDetail(it.Detail)
	var lines []string
	if len(detail) > 0 {
		lines = append(lines, renderDetail(it.PrimaryClass, detail, nameByObjectID)...)
	}
	if len(lines) > 0 {
		b.WriteString("**核心属性：**\n")
		for _, l := range lines {
			b.WriteString("- " + l + "\n")
		}
	}
	return b.String()
}

// renderDetail 把 detail map 按类别渲染成中文 "标签：值" 行
func renderDetail(primary string, m map[string]any, nameByObjectID map[int64]string) []string {
	labels := fieldLabels[primary]

	// 枪械：先反查适配弹药名称（高价值语义），槽位类数组跳过
	if primary == "gun" {
		var out []string
		if names := resolveAmmoNames(m["ammo"], nameByObjectID); len(names) > 0 {
			out = append(out, "适配弹药："+strings.Join(names, "、"))
		}
		order := []string{"caliber", "fireMode", "fireSpeed", "capacity", "armorHarm", "meatHarm",
			"penetrationPower", "muzzleVelocity", "shootDistance", "soundDistance",
			"hipShot", "control", "recoil", "stable"}
		for _, k := range order {
			if v, ok := m[k]; ok {
				out = append(out, formatKV(labels, k, v))
			}
		}
		return out
	}

	// 配件：增益/减益效果取 effectList 里的中文描述，数值修正取标量
	if primary == "acc" {
		var out []string
		if advs := effectTexts(m["advantage"]); len(advs) > 0 {
			out = append(out, "增益效果："+strings.Join(advs, "；"))
		}
		if dis := effectTexts(m["disadvantage"]); len(dis) > 0 {
			out = append(out, "减益效果："+strings.Join(dis, "；"))
		}
		for _, k := range []string{"bombCapacity", "controlSpeed", "controlStable", "hipShot", "recoil", "shotDistancePercent", "quickSeparate"} {
			if v, ok := m[k]; ok {
				if k == "quickSeparate" {
					if f, _ := v.(float64); f == 0 {
						continue // 0=不支持，没有写入价值
					}
					out = append(out, "快速分离：支持")
					continue
				}
				out = append(out, formatKV(labels, k, v))
			}
		}
		return out
	}

	// 防护装备：buff/debuff 结构（percent 或中文 value）
	if primary == "protect" {
		var out []string
		for _, k := range []string{"protectLevel", "protectArea", "durability", "durableLoss", "capacity", "repairEfficiency"} {
			if v, ok := m[k]; ok {
				out = append(out, formatKV(labels, k, v))
			}
		}
		for _, k := range []string{"aimSpeed", "moveSpeed", "sprintCost", "soundEffect", "faceMask"} {
			if v, ok := m[k]; ok {
				if txt := buffText(k, v, labels); txt != "" {
					out = append(out, txt)
				}
			}
		}
		return out
	}

	// 弹药：穿透/倍率/护甲伤害固定顺序
	if primary == "ammo" {
		var ammoOut []string
		for _, k := range []string{"penetrationLevel", "harmRatio", "armorHarmLevel"} {
			if v, ok := m[k]; ok {
				ammoOut = append(ammoOut, formatKV(labels, k, v))
			}
		}
		return ammoOut
	}

	// 其余类别（props）及兜底：按 key 字典序稳定输出
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		if v := formatKV(labels, k, m[k]); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// resolveAmmoNames 从 gun.ammo: [{"objectID":123}, ...] 反查弹药名称
func resolveAmmoNames(v any, names map[int64]string) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, e := range arr {
		obj, ok := e.(map[string]any)
		if !ok {
			continue
		}
		f, ok := obj["objectID"].(float64)
		if !ok {
			continue
		}
		name := names[int64(f)]
		if name != "" && !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// effectTexts 提取 advantage/disadvantage.effectList[].value 中文描述
func effectTexts(v any) []string {
	obj, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	list, ok := obj["effectList"].([]any)
	if !ok {
		return nil
	}
	var out []string
	for _, e := range list {
		if em, ok := e.(map[string]any); ok {
			if s, ok := em["value"].(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, s)
			}
		}
	}
	return out
}

// buffText 渲染防护装备的属性修正结构：{"percent":-4} 或 {"value":"降噪能力：中"}
func buffText(key string, v any, labels map[string]string) string {
	obj, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	label := labels[key]
	if label == "" {
		label = key
	}
	if p, ok := obj["percent"].(float64); ok {
		return fmt.Sprintf("%s：%+.0f%%", label, p)
	}
	if s, ok := obj["value"].(string); ok && s != "" {
		return fmt.Sprintf("%s：%s", label, s)
	}
	return ""
}

// formatKV 标量字段 -> "中文标签：值"；带单位的数值补单位；未知字段原样输出
func formatKV(labels map[string]string, key string, v any) string {
	if v == nil {
		return ""
	}
	label := labels[key]
	if label == "" {
		label = key
	}
	switch val := v.(type) {
	case string:
		if strings.TrimSpace(val) == "" {
			return ""
		}
		if key == "caliber" {
			val = strings.TrimPrefix(val, "ammo") // ammo7.62x39 -> 7.62x39
		}
		val = strings.ReplaceAll(val, ",", "、") // 中文语境用顿号
		return fmt.Sprintf("%s：%s", label, val)
	case float64:
		return fmt.Sprintf("%s：%s", label, scalarWithUnit(key, val))
	case bool:
		if val && key == "quickSeparate" {
			return label + "：支持"
		}
		return ""
	default:
		b, _ := json.Marshal(v)
		return fmt.Sprintf("%s：%s", label, string(b))
	}
}

// scalarWithUnit 给带明确量纲的字段补中文单位
func scalarWithUnit(key string, f float64) string {
	s := formatNum(f)
	switch key {
	case "fireSpeed":
		return s + " 发/分钟"
	case "muzzleVelocity":
		return s + " 米/秒"
	case "soundDistance":
		return s + " 米"
	case "capacity":
		return s + " 发"
	case "harmRatio":
		return s + "%"
	case "shotDistancePercent":
		return s + "%"
	}
	return s
}

func formatNum(f float64) string {
	if f == float64(int64(f)) {
		return fmt.Sprintf("%d", int64(f))
	}
	return fmt.Sprintf("%g", f)
}

// safeFileName 生成"物品-名.md"，非法字符替换、超长截断、重名加序号
func safeFileName(name string, used map[string]int) string {
	bad := strings.NewReplacer(
		`\`, "_", "/", "_", ":", "_", "*", "_", "?", "_",
		`"`, "_", "<", "_", ">", "_", "|", "_",
	)
	base := strings.TrimSpace(bad.Replace(name))
	base = trimRunes(base, 50)
	final := "物品-" + base
	if n := used[final]; n > 0 {
		used[final] = n + 1
		final = fmt.Sprintf("物品-%s-%d", base, n+1)
	} else {
		used[final] = 1
	}
	return final + ".md"
}

func trimRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}
