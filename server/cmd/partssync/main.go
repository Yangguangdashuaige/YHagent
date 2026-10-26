// cmd/partssync —— 枪械配件兼容数据同步工具
//
// 数据源：dfttk.com（三角洲 TTK 数据站）公开静态数据 /data/v3/，
// 该数据是其枪械改装计算器（builder）的前端数据，覆盖：
//   catalog/weapons.json            武器目录（68 把，含品类/可选性/战斗数值）
//   packs/build-shared/cores/{id}.json   每枪构建模板：静态槽位→可选配件（兼容表本体）、
//                                  coupling 装配规则、武器专属配件的名称/属性
//   packs/selection/weapons/{id}.json    每枪选择运行时包：配件 keyed 的 socketProviders——
//                                  枪口槽（socket 6）不在静态槽位里，由枪管配件提供
//                                  （不同枪管提供各自的枪口选项，一体消音枪管则封锁该槽）；
//                                  另含适配弹药列表、extraOccupancy（配件额外占用槽位）
//   packs/build-shared/shards/*.json     共享配件分片（枪口/枪管/瞄具/弹匣…按类分片）
//   locales/zh-CN/*.json            中文本地化（物品名/槽位名/通用词条）
//   common.json                     面板属性索引映射（DisplayAttrValues.N → 属性键）
//
// 产出：{out}/compat/gunparts.json —— 供 gun_combo_solve 枚举求解器使用的
// 完整"枪 → 槽位 → 可选配件 → 三模式属性修正"数据集。
//
// 用法（在 server/ 目录下运行）：
//   go run ./cmd/partssync                 # 增量（有缓存则跳过网络）
//   go run ./cmd/partssync -full           # 忽略缓存全量重拉
//   go run ./cmd/partssync -weapon 18050000005   # 只同步指定武器（调试用）
//
// 仅依赖标准库；原始响应缓存到 .cache/partssync/，失败重试 3 次。
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"
)

const (
	defaultBase  = "https://dfttk.com/data/v3"
	defaultOut   = "data"
	defaultCache = ".cache/partssync"
	userAgent    = "YHagent-partssync/1.0 (game assistant knowledge sync)"
)

// ---------- dfttk 原始结构（只声明用到的字段，多余字段 json 自动忽略） ----------

type rawManifest struct {
	DataVersion string `json:"dataVersion"`
	Files       map[string]struct {
		Bytes int64 `json:"bytes"`
	} `json:"files"`
}

type rawWeaponCatalog struct {
	WeaponIds []string `json:"weaponIds"`
	Objects   map[string]struct {
		ObjectType       string `json:"objectType"`
		CategoryId       string `json:"categoryId"`
		PlayerSelectable bool   `json:"playerSelectable"`
	} `json:"objects"`
}

type rawLocaleItems struct {
	Items map[string]string `json:"items"`
}

type rawLocaleSockets struct {
	Sockets map[string]string `json:"sockets"`
}

type rawCommon struct {
	MainAttributes map[string]struct {
		Key        string `json:"key"`
		Visibility string `json:"visibility"`
	} `json:"mainAttributes"`
}

// rawEffects 某配件在某模式下的效果列表（value 宽松处理：数值型操作之外存在字符串引用值）
type rawEffects []struct {
	Operation string `json:"operation"`
	Target    string `json:"target"`
	Modifier  string `json:"modifier"`
	Value     any    `json:"value"`
}

// rawDisplay display 表（shard / core 通用，嵌套结构）
type rawDisplay struct {
	Objects map[string]rawDisplayObject `json:"objects"`
	Strings map[string]string           `json:"strings"`
}

type rawCore struct {
	WeaponId string `json:"weaponId"`
	Payload  struct {
		Assembly struct {
			SocketProviders map[string]struct {
				Sockets []struct {
					SocketId      int      `json:"socketId"`
					OptionItemIds []string `json:"optionItemIds"`
				} `json:"sockets"`
			} `json:"socketProviders"`
			Coupling map[string]json.RawMessage `json:"coupling"`
		} `json:"assembly"`
		EffectsByItem map[string]map[string]rawEffects `json:"effectsByItemId"`
		Display       rawDisplay                       `json:"display"`
	} `json:"payload"`
}

type rawDisplayObject struct {
	ObjectType       string `json:"objectType"`
	CategoryId       string `json:"categoryId"`
	RarityId         string `json:"rarityId"`
	PlayerSelectable bool   `json:"playerSelectable"`
}

type rawShard struct {
	ShardId string `json:"shardId"`
	Payload struct {
		EffectsByItem map[string]map[string]rawEffects `json:"effectsByItemId"`
		Display       rawDisplay                       `json:"display"`
	} `json:"payload"`
}

// rawSelection packs/selection/weapons/{id}.json —— 配件选择运行时包。
// 关键结构：assembly.socketProviders 除武器本体 key 外还包含配件 key（多为枪管），
// 配件 provider 提供随其挂载才开放的槽位——枪口槽（socket 6）的真实来源。
// 武器本体 key 下的槽位是"当前默认装配的运行态"（选项被收缩），静态兼容表以 core 为准，
// 故本工具只取配件 keyed 的 providers。
type rawSelection struct {
	AmmoItemIds []string `json:"ammoItemIds"`
	Assembly    struct {
		SocketProviders map[string]struct {
			Sockets []struct {
				SocketId      int      `json:"socketId"`
				OptionItemIds []string `json:"optionItemIds"`
			} `json:"sockets"`
		} `json:"socketProviders"`
		ExtraOccupancy json.RawMessage `json:"extraOccupancy"`
	} `json:"assembly"`
}

// ---------- 产出数据集结构 ----------

type AttrDelta struct {
	Add map[string]float64 `json:"add,omitempty"` // 面板属性加成（据枪稳定性+3 之类）
	Mul map[string]float64 `json:"mul,omitempty"` // 面板属性乘区修正（少见，非 0 才记录）
}

type PartInfo struct {
	Id         string                `json:"id"`
	Name       string                `json:"name"`
	Category   string                `json:"category"`   // muzzle/barrel/scope…（配件分类）
	Rarity     string                `json:"rarity,omitempty"`
	Selectable *bool                 `json:"selectable,omitempty"` // nil=未知；false=出厂固定件（不可主动选择）
	Modes      map[string]*AttrDelta `json:"modes"`                // sol/mp/bd → 属性修正
}

type SocketInfo struct {
	SocketId int        `json:"socketId"`
	Slot     string     `json:"slot"`     // 槽位中文名（瞄准镜/枪口…）
	Options  []string   `json:"options"`  // 可选配件 ID（顺序保持源数据）
}

type WeaponInfo struct {
	Id       string       `json:"id"`
	Name     string       `json:"name"`
	Category string       `json:"category"` // 品类 ID（assaultRifle 等）
	CategoryName string   `json:"categoryName,omitempty"`
	Sockets  []SocketInfo `json:"sockets"`
	// ConditionalSockets 由配件挂载后开放的槽位（枪管→枪口），选项随提供者变化
	ConditionalSockets []ConditionalSocket `json:"conditionalSockets,omitempty"`
	// AmmoItemIds 适配弹药物品 ID（来自 selection pack）
	AmmoItemIds []string `json:"ammoItemIds,omitempty"`
	Coupling json.RawMessage `json:"coupling,omitempty"` // 装配规则原样保留（求解器用）
	// ExtraOccupancy 配件额外占用槽位规则（原样保留：某配件挂载后封锁其他槽）
	ExtraOccupancy json.RawMessage `json:"extraOccupancy,omitempty"`
}

// ProviderOptions 选项集合相同的提供者合并为一条（如多根枪管提供同一组枪口选项）
type ProviderOptions struct {
	ProviderIds []string `json:"providerIds"` // 提供该槽位的配件 ID（如枪管）
	Options     []string `json:"options"`     // 该提供者下可选配件；空数组=封锁该槽（一体消音枪管）
}

// ConditionalSocket 由配件提供的条件槽位：不挂提供者则槽不存在，
// 挂不同提供者开放不同选项。求解器按"所选枪管 → 其所在组的选项"求值。
type ConditionalSocket struct {
	SocketId  int               `json:"socketId"`
	Slot      string            `json:"slot"` // 槽位中文名（枪口…）
	Providers []ProviderOptions `json:"providers"`
}

type Dataset struct {
	Version       string              `json:"version"` // dfttk manifest.dataVersion
	Source        string              `json:"source"`
	FetchedAt     string              `json:"fetchedAt"`
	Attributes    map[string]string   `json:"attributes"`    // 属性键 → 中文名
	Weapons       []WeaponInfo        `json:"weapons"`
	Parts         map[string]*PartInfo `json:"parts"`        // 全部配件（含武器专属）
}

// 面板属性中文名（游戏口径的兜底表；dfttk locales 有则覆盖）
var fallbackAttrNames = map[string]string{
	"fireRate":        "射速",
	"effectiveRange":  "有效射程",
	"recoilControl":   "后坐力控制",
	"waistAccuracy":   "腰际射击精度",
	"handling":        "操控速度",
	"stability":       "据枪稳定性",
}

var fallbackCategoryNames = map[string]string{
	"assaultRifle":   "突击步枪",
	"battleRifle":    "战斗步枪",
	"marksmanRifle":  "射手步枪",
	"sniperRifle":    "狙击步枪",
	"submachineGun":  "冲锋枪",
	"lightMachineGun": "轻机枪",
	"shotgun":        "霰弹枪",
	"pistol":         "手枪",
	"bow":            "弓箭",
}

// ---------- 抓取（带缓存 + 重试） ----------

type fetcher struct {
	base  string
	cache string
	full  bool
	http  *http.Client
}

func (f *fetcher) get(rel string) ([]byte, error) {
	cacheKey := strings.ReplaceAll(rel, "/", "_")
	cachePath := filepath.Join(f.cache, cacheKey)
	if !f.full {
		if b, err := os.ReadFile(cachePath); err == nil {
			return b, nil
		}
	}
	url := f.base + "/" + rel
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 2 * time.Second)
		}
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", userAgent)
		resp, err := f.http.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("请求失败: %w", err)
			continue
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("读取响应失败: %w", err)
			continue
		}
		if resp.StatusCode != 200 {
			lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
			continue
		}
		// SPA 兜底页防御：非 JSON 响应视为失败（正确路径本就该返回 JSON）
		if len(body) == 0 || (body[0] != '{' && body[0] != '[') {
			lastErr = fmt.Errorf("响应非 JSON（疑似兜底页）: %s", truncStr(string(body), 60))
			continue
		}
		_ = os.MkdirAll(f.cache, 0o755)
		_ = os.WriteFile(cachePath, body, 0o644)
		return body, nil
	}
	return nil, fmt.Errorf("%s: %w", rel, lastErr)
}

func truncStr(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// ---------- 解析与装配 ----------

// attrIndexMap 把 common.json 的 mainAttributes 索引表转成 idx→key
func attrIndexMap(c *rawCommon) map[string]string {
	m := make(map[string]string)
	for idx, a := range c.MainAttributes {
		m[idx] = a.Key
	}
	return m
}

// extractAttrDelta 从效果列表提取面板属性修正（DisplayAttrValues.N 的 Addend/Mult_A）
func extractAttrDelta(effects rawEffects, idxToKey map[string]string) *AttrDelta {
	var d AttrDelta
	for _, e := range effects {
		if e.Operation != "StaticAttributeModify" {
			continue
		}
		if !strings.HasPrefix(e.Target, "DisplayAttrValues.") {
			continue
		}
		v, isNum := e.Value.(float64)
		if !isNum {
			continue // 引用型/字符串值不参与面板数值修正
		}
		idx := strings.TrimPrefix(e.Target, "DisplayAttrValues.")
		key, ok := idxToKey[idx]
		if !ok || key == "" {
			continue
		}
		switch e.Modifier {
		case "Addend":
			if v != 0 {
				if d.Add == nil {
					d.Add = map[string]float64{}
				}
				d.Add[key] += v
			}
		case "Mult_A":
			if v != 0 {
				if d.Mul == nil {
					d.Mul = map[string]float64{}
				}
				d.Mul[key] += v
			}
		}
	}
	if d.Add == nil && d.Mul == nil {
		return nil
	}
	return &d
}

// partMerger 汇聚共享 shard 与武器专属 core 的配件数据，名称解析三级 fallback
type partMerger struct {
	sharedParts   map[string]*PartInfo
	itemNames     map[string]string
	idxToKey      map[string]string
	unnamedCount  int
}

func (m *partMerger) ingest(display rawDisplay, effects map[string]map[string]rawEffects) {
	for id, obj := range display.Objects {
		if obj.ObjectType != "part" {
			continue
		}
		p := m.sharedParts[id]
		if p == nil {
			p = &PartInfo{Id: id, Modes: map[string]*AttrDelta{}}
			m.sharedParts[id] = p
		}
		if obj.CategoryId != "" {
			p.Category = obj.CategoryId
		}
		if obj.RarityId != "" {
			p.Rarity = obj.RarityId
		}
		sel := obj.PlayerSelectable
		p.Selectable = &sel
		if name, ok := display.Strings[id]; ok && name != "" && !strings.HasPrefix(name, "内部配件") {
			p.Name = name
		}
		for mode, effs := range effects[id] {
			if d := extractAttrDelta(effs, m.idxToKey); d != nil {
				p.Modes[mode] = d
			}
		}
	}
	// display.Objects 里缺失但 effects 存在的配件也登记（保底，防引用悬空）；
	// 有 display 记录但不是 part 的（如武器本体）跳过，防止误登记
	for id := range effects {
		if _, hasDisplay := display.Objects[id]; hasDisplay {
			continue
		}
		if _, ok := m.sharedParts[id]; !ok {
			m.sharedParts[id] = &PartInfo{Id: id, Modes: map[string]*AttrDelta{}}
		}
	}
}

// resolveNames 对无 display 名的配件做 locale fallback；
// unnamedCount 统计最终仍为内部名（无正式名）的配件数
func (m *partMerger) resolveNames() {
	for id, p := range m.sharedParts {
		if p.Name == "" {
			if n, ok := m.itemNames[id]; ok && n != "" {
				p.Name = n
			} else {
				p.Name = "内部配件 " + id
			}
		}
		if strings.HasPrefix(p.Name, "内部配件") {
			m.unnamedCount++
		}
	}
}

// mergeConditionalSockets 从 selection pack 提取配件 keyed 的条件槽位：
//   - 跳过武器本体 key（其槽位是默认装配运行态，静态兼容表以 core 为准）；
//   - 按 socketId 分组；选项集合完全相同的提供者合并为一条 ProviderOptions；
//   - 提供者按 ID 排序保证输出稳定。
//
// slotName 由调用方用槽位本地化表填充（与静态槽同源）。
func mergeConditionalSockets(sel *rawSelection, weaponId string, slotName func(int) string) []ConditionalSocket {
	type providerEntry struct {
		id   string
		opts []string
	}
	groups := map[int][]providerEntry{}
	var socketOrder []int
	for pid, prov := range sel.Assembly.SocketProviders {
		if pid == weaponId {
			continue
		}
		for _, s := range prov.Sockets {
			if _, seen := groups[s.SocketId]; !seen {
				socketOrder = append(socketOrder, s.SocketId)
			}
			opts := s.OptionItemIds
			if opts == nil {
				opts = []string{} // 空选项=该提供者封锁此槽，保留显式语义
			}
			groups[s.SocketId] = append(groups[s.SocketId], providerEntry{id: pid, opts: opts})
		}
	}
	if len(socketOrder) == 0 {
		return nil
	}
	sort.Ints(socketOrder)

	out := make([]ConditionalSocket, 0, len(socketOrder))
	for _, sid := range socketOrder {
		entries := groups[sid]
		slices.SortFunc(entries, func(a, b providerEntry) int {
			return strings.Compare(a.id, b.id)
		})
		cs := ConditionalSocket{SocketId: sid, Slot: slotName(sid), Providers: []ProviderOptions{}}
		for _, e := range entries {
			merged := false
			for i := range cs.Providers {
				if slices.Equal(cs.Providers[i].Options, e.opts) {
					cs.Providers[i].ProviderIds = append(cs.Providers[i].ProviderIds, e.id)
					merged = true
					break
				}
			}
			if !merged {
				cs.Providers = append(cs.Providers, ProviderOptions{
					ProviderIds: []string{e.id},
					Options:     e.opts,
				})
			}
		}
		out = append(out, cs)
	}
	return out
}

func main() {
	base := flag.String("base", defaultBase, "dfttk 数据根地址")
	out := flag.String("out", defaultOut, "输出根目录（产出 {out}/compat/gunparts.json）")
	cache := flag.String("cache", defaultCache, "原始响应缓存目录")
	full := flag.Bool("full", false, "忽略缓存全量重拉")
	weaponFilter := flag.String("weapon", "", "只同步指定武器 ID（逗号分隔，调试用）")
	flag.Parse()

	start := time.Now()
	if err := run(*base, *out, *cache, *full, *weaponFilter); err != nil {
		log.Fatalf("[partssync] 失败: %v", err)
	}
	log.Printf("[partssync] 完成，耗时 %s", time.Since(start).Round(time.Millisecond))
}

func run(base, outDir, cacheDir string, full bool, weaponFilter string) error {
	f := &fetcher{
		base:  strings.TrimRight(base, "/"),
		cache: cacheDir,
		full:  full,
		http:  &http.Client{Timeout: 60 * time.Second},
	}

	// 1. manifest：数据版本 + 文件清单（用于枚举全部 shard）
	manifestBytes, err := f.get("manifest.json")
	if err != nil {
		return fmt.Errorf("拉取 manifest 失败: %w", err)
	}
	var manifest rawManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return fmt.Errorf("解析 manifest 失败: %w", err)
	}
	log.Printf("[partssync] 数据版本: %s（文件 %d 个）", manifest.DataVersion, len(manifest.Files))

	// 2. 武器目录
	catalogBytes, err := f.get("catalog/weapons.json")
	if err != nil {
		return fmt.Errorf("拉取武器目录失败: %w", err)
	}
	var catalog rawWeaponCatalog
	if err := json.Unmarshal(catalogBytes, &catalog); err != nil {
		return fmt.Errorf("解析武器目录失败: %w", err)
	}

	// 3. 本地化 + 属性映射
	var items rawLocaleItems
	if b, err := f.get("locales/zh-CN/items.json"); err == nil {
		_ = json.Unmarshal(b, &items)
	} else {
		log.Printf("[partssync] 警告: 拉取物品本地化失败（名称 fallback 降级）: %v", err)
	}
	var sockets rawLocaleSockets
	if b, err := f.get("locales/zh-CN/sockets.json"); err == nil {
		_ = json.Unmarshal(b, &sockets)
	} else {
		log.Printf("[partssync] 警告: 拉取槽位本地化失败: %v", err)
	}
	var common rawCommon
	if b, err := f.get("common.json"); err == nil {
		_ = json.Unmarshal(b, &common)
	} else {
		return fmt.Errorf("拉取 common.json（属性映射）失败: %w", err)
	}
	idxToKey := attrIndexMap(&common)

	// 4. 共享配件 shard（从 manifest 文件清单枚举，防新增分片遗漏）
	var shardFiles []string
	for rel := range manifest.Files {
		if strings.HasPrefix(rel, "packs/build-shared/shards/") && strings.HasSuffix(rel, ".json") {
			shardFiles = append(shardFiles, rel)
		}
	}
	merger := &partMerger{
		sharedParts: map[string]*PartInfo{},
		itemNames:   items.Items,
		idxToKey:    idxToKey,
	}
	for _, rel := range shardFiles {
		b, err := f.get(rel)
		if err != nil {
			return fmt.Errorf("拉取 shard 失败: %w", err)
		}
		var shard rawShard
		if err := json.Unmarshal(b, &shard); err != nil {
			return fmt.Errorf("解析 %s 失败: %w", rel, err)
		}
		merger.ingest(shard.Payload.Display, shard.Payload.EffectsByItem)
	}
	log.Printf("[partssync] 共享 shard %d 个 → 配件 %d 条", len(shardFiles), len(merger.sharedParts))

	// 5. 每把武器 core：槽位兼容表 + 武器专属配件 + coupling
	var wanted map[string]bool
	if weaponFilter != "" {
		wanted = map[string]bool{}
		for _, id := range strings.Split(weaponFilter, ",") {
			wanted[strings.TrimSpace(id)] = true
		}
	}
	ds := &Dataset{
		Version:    manifest.DataVersion,
		Source:     f.base,
		FetchedAt:  time.Now().Format(time.RFC3339),
		Attributes: map[string]string{},
		Parts:      merger.sharedParts,
	}
	// 属性中文名：兜底表起步
	for k, v := range fallbackAttrNames {
		ds.Attributes[k] = v
	}

	var weapons []WeaponInfo
	failed := 0
	for _, wid := range catalog.WeaponIds {
		if wanted != nil && !wanted[wid] {
			continue
		}
		obj, ok := catalog.Objects[wid]
		if !ok || !obj.PlayerSelectable {
			continue
		}
		b, err := f.get(path.Join("packs", "build-shared", "cores", wid+".json"))
		if err != nil {
			log.Printf("[partssync] 警告: 武器 %s core 拉取失败，跳过: %v", wid, err)
			failed++
			continue
		}
		var core rawCore
		if err := json.Unmarshal(b, &core); err != nil {
			log.Printf("[partssync] 警告: 武器 %s core 解析失败，跳过: %v", wid, err)
			failed++
			continue
		}
		// 武器专属配件并入配件表
		merger.ingest(core.Payload.Display, core.Payload.EffectsByItem)

		// selection pack：配件 keyed providers（枪管→枪口）、适配弹药、额外占用
		selBytes, err := f.get(path.Join("packs", "selection", "weapons", wid+".json"))
		if err != nil {
			log.Printf("[partssync] 警告: 武器 %s selection pack 拉取失败，跳过: %v", wid, err)
			failed++
			continue
		}
		var sel rawSelection
		if err := json.Unmarshal(selBytes, &sel); err != nil {
			log.Printf("[partssync] 警告: 武器 %s selection pack 解析失败，跳过: %v", wid, err)
			failed++
			continue
		}

		w := WeaponInfo{
			Id:       wid,
			Name:     items.Items[wid],
			Category: obj.CategoryId,
			CategoryName: fallbackCategoryNames[obj.CategoryId],
		}
		if w.Name == "" {
			w.Name = "武器 " + wid
		}
		slotName := func(socketId int) string {
			if n, ok := sockets.Sockets[fmt.Sprint(socketId)]; ok && n != "" {
				return n
			}
			return fmt.Sprintf("槽位%d", socketId)
		}
		if sp, ok := core.Payload.Assembly.SocketProviders[wid]; ok {
			for _, s := range sp.Sockets {
				si := SocketInfo{
					SocketId: s.SocketId,
					Slot:     slotName(s.SocketId),
					Options:  s.OptionItemIds,
				}
				w.Sockets = append(w.Sockets, si)
			}
		}
		w.ConditionalSockets = mergeConditionalSockets(&sel, wid, slotName)
		w.AmmoItemIds = sel.AmmoItemIds
		if c, ok := core.Payload.Assembly.Coupling[wid]; ok {
			w.Coupling = c
		}
		if len(sel.Assembly.ExtraOccupancy) > 0 {
			w.ExtraOccupancy = sel.Assembly.ExtraOccupancy
		}
		weapons = append(weapons, w)
	}
	merger.resolveNames()
	ds.Weapons = weapons

	// 6. 统计 + 原子落盘（条件槽位的选项/引用与静态槽同口径计数）
	totalOptions, condOptions, missingParts := 0, 0, 0
	condSocketCount := 0
	for _, w := range ds.Weapons {
		for _, s := range w.Sockets {
			totalOptions += len(s.Options)
			for _, pid := range s.Options {
				if _, ok := ds.Parts[pid]; !ok {
					missingParts++
				}
			}
		}
		condSocketCount += len(w.ConditionalSockets)
		for _, cs := range w.ConditionalSockets {
			for _, po := range cs.Providers {
				condOptions += len(po.Options)
				for _, pid := range po.Options {
					if _, ok := ds.Parts[pid]; !ok {
						missingParts++
					}
				}
				// 提供者本身也必须是已登记配件（如枪管）
				for _, prid := range po.ProviderIds {
					if _, ok := ds.Parts[prid]; !ok {
						missingParts++
					}
				}
			}
		}
	}
	named := 0
	for _, p := range ds.Parts {
		if !strings.HasPrefix(p.Name, "内部配件") {
			named++
		}
	}
	log.Printf("[partssync] 武器 %d 把（失败 %d）/ 静态槽选项 %d / 条件槽 %d 个（选项 %d）/ 配件 %d 条（有正式名 %d，内部名 %d）/ 引用缺失配件 %d 个",
		len(ds.Weapons), failed, totalOptions, condSocketCount, condOptions, len(ds.Parts), named, merger.unnamedCount, missingParts)
	if failed > 0 {
		return fmt.Errorf("%d 把武器数据包（core/selection）拉取失败，请重跑（缓存会跳过已成功的）", failed)
	}
	if missingParts > 0 {
		log.Printf("[partssync] 警告: 存在槽位/提供者引用但配件表缺失的 ID（可能是 shard 清单外的新分片）")
	}

	outPath := filepath.Join(outDir, "compat", "gunparts.json")
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(ds, "", " ")
	if err != nil {
		return err
	}
	tmp := outPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, outPath); err != nil {
		return err
	}
	log.Printf("[partssync] 已写入 %s（%.1f MB）", outPath, float64(len(data))/1024/1024)
	return nil
}
