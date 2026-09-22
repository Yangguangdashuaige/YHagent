// internal/data/deltaapi/gun_advisor.go
//
// 智能改枪顾问（一期）：上游 /gun_gqm 只能读社区已发布方案、且要求传入完整枪名。
// 本文件在不突破该硬边界的前提下做三层增强：
//  1. 枪名规范化：模糊名/简称 → 自动补全分类后缀，必要时扫描分类页反查完整枪名；
//  2. 多页聚合：一次拉取最多 advisorMaxPages 页社区方案（走 6h 长缓存，点数成本极低）；
//  3. 意图排序：解析 propertiesDetail 八维属性，按压枪/腰射/稳定/射程/均衡/省钱/热度排序取 Top5。
package deltaapi

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

const advisorMaxPages = 8 // 每页 12 条，最多分析 96 个社区方案

// flexInt 上游 likeNum/applyNum/price 等字段数字与空字符串混用，统一容错解析
type flexInt int

func (f *flexInt) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(strings.Trim(string(b), `"`))
	if s == "" {
		*f = 0
		return nil
	}
	n, err := strconv.Atoi(strings.ReplaceAll(s, ",", ""))
	if err != nil {
		*f = 0
		return nil
	}
	*f = flexInt(n)
	return nil
}

// flexStat propertiesDetail 里的属性值可能是 "55"/"+25"/"-35"/68 等形态
type flexStat int

func (f *flexStat) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(strings.Trim(string(b), `"`))
	if s == "" {
		*f = 0
		return nil
	}
	s = strings.TrimPrefix(s, "+")
	n, err := strconv.Atoi(s)
	if err != nil {
		*f = 0
		return nil
	}
	*f = flexStat(n)
	return nil
}

// nestedJSON 兼容上游把 JSON 对象写成转义字符串的字段（propertiesDetail 实测为 JSON 字符串）
type nestedJSON []byte

func (n *nestedJSON) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		*n = nestedJSON(s)
		return nil
	}
	cp := append(nestedJSON(nil), b...)
	*n = cp
	return nil
}

// gunListPage /gun_gqm 的 data 实测直接是方案数组（count 在外层 envelope，已被 doGet 剥离），
// 同时兼容个别接口 data 为 {"data":[...]} 的嵌套形态。
type gunListPage struct {
	Data []gunBuildItem `json:"data"`
}

// gunBuildItem 单条社区改枪方案
type gunBuildItem struct {
	ObjectName       string     `json:"objectName"`
	Name             string     `json:"name"`
	SolutionCode     string     `json:"solutionCode"`
	Pic              string     `json:"pic"`
	Mtype            string     `json:"m_type"`
	Author           string     `json:"authorNickname"`
	LikeNum          flexInt    `json:"likeNum"`
	ApplyNum         flexInt    `json:"applyNum"`
	Price            flexInt    `json:"price"`
	PropertiesDetail nestedJSON `json:"propertiesDetail"`
}

// buildProps 八维属性（数值越高代表该维度表现越好，change 为相对裸枪的增减）
type buildProps struct {
	MeatHarm      flexStat `json:"meatHarm"`      // 肉伤
	ShootDistance flexStat `json:"shootDistance"` // 射程
	Recoil        flexStat `json:"recoil"`        // 后座控制
	Control       flexStat `json:"control"`       // 操控
	Stable        flexStat `json:"stable"`        // 稳定
	HipShot       flexStat `json:"hipShot"`       // 腰射
	ArmorHarm     flexStat `json:"armorHarm"`     // 甲伤
	FireSpeed     flexStat `json:"fireSpeed"`     // 射速
}

type buildPropsPair struct {
	Origin buildProps `json:"origin"`
	Change buildProps `json:"change"`
}

// GunAdvisorParams 智能改枪顾问入参
type GunAdvisorParams struct {
	Gun    string `json:"gun"`    // 枪械名，支持简称，如 K437 / M700
	Mtype  string `json:"mtype"`  // 可选：枪械分类，提供后枪名补全更准
	Intent string `json:"intent"` // recoil/hipshot/stable/control/range/balanced/budget/popular
	Budget int    `json:"budget"` // 可选：预算上限（哈夫币），超过的已知价格方案被过滤
}

// scoredBuild 聚合+打分后的内部结构
type scoredBuild struct {
	item  gunBuildItem
	props buildPropsPair
	score float64
}

// mtypeAlias 把模型可能传入的分类说法归一到接口枚举
func mtypeAlias(raw string) string {
	s := strings.TrimSpace(raw)
	switch {
	case strings.Contains(s, "冲锋"):
		return "冲锋枪"
	case strings.Contains(s, "精确"), strings.Contains(s, "射手"):
		return "精确射手步枪"
	case strings.Contains(s, "狙击"):
		return "狙击步枪"
	case strings.Contains(s, "霰弹"), strings.Contains(s, "喷子"):
		return "霰弹枪"
	case strings.Contains(s, "手枪"):
		return "手枪"
	case strings.Contains(s, "机枪"):
		return "轻机枪"
	case strings.Contains(s, "特殊"):
		return "特殊武器"
	case strings.Contains(s, "步枪"):
		return "步枪"
	default:
		return s
	}
}

// suffixCandidates 各分类下完整枪名可能的后缀（接口 gun 参数必须是完整枪名）
func suffixCandidates(mtype string) []string {
	switch mtype {
	case "步枪":
		return []string{"突击步枪", "战斗步枪", "步枪"}
	case "冲锋枪":
		return []string{"冲锋枪"}
	case "精确射手步枪":
		return []string{"精确射手步枪", "射手步枪"}
	case "狙击步枪":
		return []string{"狙击步枪", "精确射手步枪"}
	case "轻机枪":
		return []string{"轻机枪", "机枪"}
	case "霰弹枪":
		return []string{"霰弹枪"}
	case "手枪":
		return []string{"手枪"}
	case "特殊武器":
		return []string{"特殊武器"}
	default:
		return []string{"突击步枪", "冲锋枪", "精确射手步枪", "狙击步枪", "轻机枪", "霰弹枪", "手枪", "特殊武器", "步枪"}
	}
}

// fetchGunPage 拉取并解析一页改枪码（data 可能直接是数组，也可能是 {"data":[...]}）
func (c *Client) fetchGunPage(ctx context.Context, gun, mtype string, page int) (*gunListPage, error) {
	raw, err := c.GunBuildCode(ctx, GunBuildCodeParams{Page: page, Mtype: mtype, Gun: gun})
	if err != nil {
		return nil, err
	}
	var items []gunBuildItem
	if err := json.Unmarshal(raw, &items); err == nil {
		return &gunListPage{Data: items}, nil
	}
	var lp gunListPage
	if err := json.Unmarshal(raw, &lp); err != nil {
		return nil, fmt.Errorf("解析改枪码响应失败: %w", err)
	}
	return &lp, nil
}

// resolveGunName 把用户的模糊枪名解析为接口认可的完整枪名，返回规范名/分类/首页数据
func (c *Client) resolveGunName(ctx context.Context, gun, mtype string) (string, string, *gunListPage, error) {
	gun = strings.TrimSpace(gun)
	if gun == "" {
		return "", "", nil, fmt.Errorf("枪械名称不能为空")
	}
	mtype = mtypeAlias(mtype)

	// 1) 原名直查（用户或模型给的本来就是完整枪名）
	if lp, err := c.fetchGunPage(ctx, gun, mtype, 1); err == nil && len(lp.Data) > 0 {
		name := lp.Data[0].ObjectName
		if name == "" {
			name = gun
		}
		mt := lp.Data[0].Mtype
		if mt == "" {
			mt = mtype
		}
		return name, mt, lp, nil
	}

	// 2) 按分类后缀补全：K437 → K437突击步枪
	tried := map[string]bool{gun: true}
	for _, suffix := range suffixCandidates(mtype) {
		if strings.HasSuffix(gun, suffix) {
			continue
		}
		candidate := gun + suffix
		if tried[candidate] {
			continue
		}
		tried[candidate] = true
		lp, err := c.fetchGunPage(ctx, candidate, mtype, 1)
		if err != nil {
			continue
		}
		if len(lp.Data) > 0 {
			name := lp.Data[0].ObjectName
			mt := lp.Data[0].Mtype
			if mt == "" {
				mt = mtype
			}
			return name, mt, lp, nil
		}
	}

	// 3) 最后兜底：扫描分类列表页，用包含关系反查（应对别名/口语名）。
	// 只扫一个分类（最多 advisorMaxPages 页），命中即停，点数成本受长缓存摊薄。
	scanMtype := mtype
	if scanMtype == "" {
		return "", "", nil, fmt.Errorf("没有找到「%s」的社区改枪方案，请提供完整枪名（如 K437突击步枪）或枪械分类后重试", gun)
	}
	seenNames := make(map[string]bool)
	var available []string
	for p := 1; p <= advisorMaxPages; p++ {
		lp, err := c.fetchGunPage(ctx, "", scanMtype, p)
		if err != nil || len(lp.Data) == 0 {
			break
		}
		for i := range lp.Data {
			full := lp.Data[i].ObjectName
			if full == "" {
				continue
			}
			if !seenNames[full] {
				seenNames[full] = true
				available = append(available, full)
			}
			if strings.Contains(strings.ToLower(full), strings.ToLower(gun)) {
				hit, err := c.fetchGunPage(ctx, full, scanMtype, 1)
				if err == nil && len(hit.Data) > 0 {
					return full, scanMtype, hit, nil
				}
			}
		}
	}
	hint := ""
	if len(available) > 0 {
		limit := len(available)
		if limit > 15 {
			limit = 15
		}
		hint = "。该分类下可识别的枪名：" + strings.Join(available[:limit], "、")
	}
	return "", "", nil, fmt.Errorf("没有找到「%s」的社区改枪方案%s", gun, hint)
}

// RecommendGunBuilds 聚合社区改枪方案并按用户意图排序，返回 Top5 推荐
func (c *Client) RecommendGunBuilds(ctx context.Context, p GunAdvisorParams) (map[string]any, error) {
	intent := p.Intent
	if intent == "" {
		intent = "balanced"
	}
	valid := map[string]bool{"recoil": true, "hipshot": true, "stable": true, "control": true,
		"range": true, "balanced": true, "budget": true, "popular": true}
	if !valid[intent] {
		return nil, fmt.Errorf("不支持的偏好类型 intent=%s", intent)
	}

	fullName, mtype, firstPage, err := c.resolveGunName(ctx, p.Gun, p.Mtype)
	if err != nil {
		return nil, err
	}

	// 上游不在 data 里返回总数，顺序翻页直到空页或达到页数上限。
	// 顺序而非并发：避免对不存在的页号白白消耗资源点（每页调用都有6h长缓存）。
	pages := []*gunListPage{firstPage}
	for pg := 2; pg <= advisorMaxPages; pg++ {
		lp, err := c.fetchGunPage(ctx, fullName, mtype, pg)
		if err != nil {
			break // 单页失败不致命，已拿到的页照常参与推荐
		}
		if len(lp.Data) == 0 {
			break
		}
		pages = append(pages, lp)
	}

	// 汇总去重 + 解析八维属性
	seen := make(map[string]bool)
	var builds []scoredBuild
	scanned := 0
	for _, lp := range pages {
		if lp == nil {
			continue
		}
		for _, it := range lp.Data {
			scanned++
			code := it.SolutionCode
			if code == "" {
				continue
			}
			if seen[code] {
				continue
			}
			seen[code] = true

			// 预算过滤（价格未知的方案保留并标记，不武断丢弃）
			if p.Budget > 0 && it.Price > 0 && int(it.Price) > p.Budget {
				continue
			}
			var pair buildPropsPair
			if len(it.PropertiesDetail) > 0 && string(it.PropertiesDetail) != "null" {
				_ = json.Unmarshal(it.PropertiesDetail, &pair)
			}
			builds = append(builds, scoredBuild{item: it, props: pair})
		}
	}
	if len(builds) == 0 {
		return nil, fmt.Errorf("「%s」有社区方案但全部不符合条件（如预算 %d 过低），请放宽条件重试", fullName, p.Budget)
	}

	rankBuilds(builds, intent)

	topN := 5
	if len(builds) < topN {
		topN = len(builds)
	}
	out := make([]map[string]any, 0, topN)
	for i := 0; i < topN; i++ {
		b := builds[i]
		out = append(out, map[string]any{
			"rank":          i + 1,
			"name":          b.item.Name,
			"code":          b.item.SolutionCode,
			"mtype":         b.item.Mtype,
			"author":        b.item.Author,
			"price":         int(b.item.Price),
			"price_unknown": b.item.Price <= 0,
			"likes":         int(b.item.LikeNum),
			"applies":       int(b.item.ApplyNum),
			"props_origin":  propsToMap(b.props.Origin),
			"props_change":  propsToMap(b.props.Change),
			"score":         roundScore(b.score),
		})
	}

	return map[string]any{
		"gun":           fullName,
		"mtype":         mtype,
		"intent":        intent,
		"budget":        p.Budget,
		"scanned":       scanned,
		"matched":       len(builds),
		"data_source":   "三角洲数据帝社区改枪方案（烽火地带）",
		"answer_hint":   "请把 code 放在独立代码块中方便玩家复制；逐条说明该方案 props_change 相对偏好的取舍；最后给出游戏内导入步骤：改枪台→方案→输入改枪码。",
		"recommend_top": out,
	}, nil
}

// propsToMap 八维属性转可读 map（键名与游戏术语对齐）
func propsToMap(p buildProps) map[string]int {
	return map[string]int{
		"肉伤": int(p.MeatHarm), "射程": int(p.ShootDistance), "后座控制": int(p.Recoil),
		"操控": int(p.Control), "稳定": int(p.Stable), "腰射": int(p.HipShot),
		"甲伤": int(p.ArmorHarm), "射速": int(p.FireSpeed),
	}
}

func roundScore(s float64) float64 {
	return float64(int(s*100+0.5)) / 100
}

// rankBuilds 按意图对方案排序（原地）
func rankBuilds(builds []scoredBuild, intent string) {
	// 各维度池内最大绝对值，用于把不同量纲的属性归一化
	dims := []func(buildProps) int{
		func(p buildProps) int { return int(p.MeatHarm) },
		func(p buildProps) int { return int(p.ShootDistance) },
		func(p buildProps) int { return int(p.Recoil) },
		func(p buildProps) int { return int(p.Control) },
		func(p buildProps) int { return int(p.Stable) },
		func(p buildProps) int { return int(p.HipShot) },
		func(p buildProps) int { return int(p.ArmorHarm) },
		func(p buildProps) int { return int(p.FireSpeed) },
	}
	maxAbs := make([]float64, len(dims))
	for _, b := range builds {
		for i, d := range dims {
			v := d(b.props.Change)
			if v < 0 {
				v = -v
			}
			if float64(v) > maxAbs[i] {
				maxAbs[i] = float64(v)
			}
		}
	}
	norm := func(b *scoredBuild) []float64 {
		out := make([]float64, len(dims))
		for i, d := range dims {
			if maxAbs[i] > 0 {
				out[i] = float64(d(b.props.Change)) / maxAbs[i]
			}
		}
		return out
	}
	// 维度索引：0肉伤 1射程 2后座 3操控 4稳定 5腰射 6甲伤 7射速
	weightSets := map[string][]float64{
		"recoil":   {0, 0, 0.55, 0.15, 0.20, 0.10, 0, 0},
		"hipshot":  {0, 0, 0.10, 0.10, 0.20, 0.60, 0, 0},
		"stable":   {0, 0, 0.25, 0.25, 0.50, 0, 0, 0},
		"control":  {0, 0, 0.15, 0.55, 0.15, 0.15, 0, 0},
		"range":    {0.25, 0.50, 0, 0, 0, 0, 0.25, 0},
		"balanced": {0.125, 0.125, 0.125, 0.125, 0.125, 0.125, 0.125, 0.125},
	}
	popScore := func(b *scoredBuild) float64 {
		return float64(int(b.item.LikeNum)+2*int(b.item.ApplyNum)) / 100.0
	}

	if intent == "budget" {
		sort.SliceStable(builds, func(i, j int) bool {
			pi, pj := int(builds[i].item.Price), int(builds[j].item.Price)
			if (pi <= 0) != (pj <= 0) {
				return pi > 0 // 已知价格排在未知前面
			}
			if pi != pj {
				return pi < pj
			}
			return popScore(&builds[i]) > popScore(&builds[j])
		})
		return
	}
	if intent == "popular" {
		sort.SliceStable(builds, func(i, j int) bool {
			return popScore(&builds[i]) > popScore(&builds[j])
		})
		return
	}

	w := weightSets[intent]
	for i := range builds {
		n := norm(&builds[i])
		s := 0.0
		for k := range n {
			s += n[k] * w[k]
		}
		// 热度仅作极小的平局打破因子，避免冷门/空数据方案扎堆
		builds[i].score = s + popScore(&builds[i])*0.01
	}
	sort.SliceStable(builds, func(i, j int) bool {
		return builds[i].score > builds[j].score
	})
}
