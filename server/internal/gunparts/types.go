// internal/gunparts/types.go
//
// 配件兼容数据集加载层。数据文件 data/compat/gunparts.json 由 cmd/partssync
// 生成（dfttk.com 静态数据），本包只做加载与查找，schema 字段与 partssync
// 的输出结构保持一致——该工具是 main 包无法被 import，故此处按 JSON 契约重复声明。
package gunparts

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// AttrDelta 单配件在某模式下的属性修正（面板五维）
type AttrDelta struct {
	Add map[string]float64 `json:"add,omitempty"` // 面板属性加成（据枪稳定性+3 之类）
	Mul map[string]float64 `json:"mul,omitempty"` // 面板属性乘区修正（少见）
}

// PartInfo 配件条目
type PartInfo struct {
	Id         string                `json:"id"`
	Name       string                `json:"name"`
	Category   string                `json:"category"` // muzzle/barrel/scope…（配件分类）
	Rarity     string                `json:"rarity,omitempty"`
	Selectable *bool                 `json:"selectable,omitempty"` // nil=未知；false=出厂固定件
	Modes      map[string]*AttrDelta `json:"modes"`               // sol/mp/bd → 属性修正
}

// SocketInfo 静态槽位（武器本体自带）
type SocketInfo struct {
	SocketId int      `json:"socketId"`
	Slot     string   `json:"slot"`    // 槽位中文名（瞄准镜/枪口…）
	Options  []string `json:"options"` // 可选配件 ID
}

// ProviderOptions 选项集合相同的提供者合并组（如多根枪管提供同一组枪口选项）
type ProviderOptions struct {
	ProviderIds []string `json:"providerIds"` // 提供该槽位的配件 ID（如枪管）
	Options     []string `json:"options"`     // 该提供者下可选配件；空数组=封锁该槽
}

// ConditionalSocket 由配件挂载后开放的条件槽位（枪管→枪口）
type ConditionalSocket struct {
	SocketId  int               `json:"socketId"`
	Slot      string            `json:"slot"`
	Providers []ProviderOptions `json:"providers"`
}

// WeaponInfo 武器条目
type WeaponInfo struct {
	Id                  string              `json:"id"`
	Name                string              `json:"name"`
	Category            string              `json:"category"`
	CategoryName        string              `json:"categoryName,omitempty"`
	Sockets             []SocketInfo        `json:"sockets"`
	ConditionalSockets  []ConditionalSocket `json:"conditionalSockets,omitempty"`
	AmmoItemIds         []string            `json:"ammoItemIds,omitempty"`
	Coupling            json.RawMessage     `json:"coupling,omitempty"`
	ExtraOccupancy      json.RawMessage     `json:"extraOccupancy,omitempty"`
}

// Dataset gunparts.json 顶层结构
type Dataset struct {
	Version    string               `json:"version"` // dfttk manifest.dataVersion
	Source     string               `json:"source"`
	FetchedAt  string               `json:"fetchedAt"`
	Attributes map[string]string    `json:"attributes"` // 属性键 → 中文名
	Weapons    []WeaponInfo         `json:"weapons"`
	Parts      map[string]*PartInfo `json:"parts"`
}

// attrFallback 属性键 → 游戏口径中文名（dataset.Attributes 优先，缺失时兜底）
var attrFallback = map[string]string{
	"stability":     "据枪稳定性",
	"recoilControl": "后坐力控制",
	"handling":      "操控速度",
	"waistAccuracy": "腰射精度",
	"effectiveRange": "有效射程",
}

// Store 数据集内存索引：武器名称小写查找 + 属性中文名
type Store struct {
	ds      Dataset
	byLower map[string]*WeaponInfo
}

// NewStore 从指定路径加载 gunparts.json（通常为 data/compat/gunparts.json）
func NewStore(path string) (*Store, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配件数据集 %s: %w", path, err)
	}
	var ds Dataset
	if err := json.Unmarshal(raw, &ds); err != nil {
		return nil, fmt.Errorf("解析配件数据集: %w", err)
	}
	if len(ds.Weapons) == 0 {
		return nil, fmt.Errorf("配件数据集无武器条目")
	}
	st := &Store{ds: ds, byLower: make(map[string]*WeaponInfo, len(ds.Weapons))}
	for i := range ds.Weapons {
		w := &ds.Weapons[i]
		st.byLower[strings.ToLower(w.Name)] = w
	}
	return st, nil
}

// Version 数据版本（dfttk manifest）
func (s *Store) Version() string { return s.ds.Version }

// WeaponCount 武器条目数
func (s *Store) WeaponCount() int { return len(s.ds.Weapons) }

// AttrName 属性键中文名
func (s *Store) AttrName(key string) string {
	if n, ok := s.ds.Attributes[key]; ok && n != "" {
		return n
	}
	if n, ok := attrFallback[key]; ok {
		return n
	}
	return key
}

// FindWeapon 按名称或 ID 查找武器：精确 ID → 精确名（不区分大小写）→ 包含匹配。
// 返回 (nil, 候选名列表) 表示命中多个，调用方应提示重试。
func (s *Store) FindWeapon(q string) (*WeaponInfo, []string) {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil, nil
	}
	// 精确 ID
	for i := range s.ds.Weapons {
		if s.ds.Weapons[i].Id == q {
			return &s.ds.Weapons[i], nil
		}
	}
	// 精确名称
	if w, ok := s.byLower[strings.ToLower(q)]; ok {
		return w, nil
	}
	// 包含匹配（不区分大小写）
	lq := strings.ToLower(q)
	var names []string
	for i := range s.ds.Weapons {
		if strings.Contains(strings.ToLower(s.ds.Weapons[i].Name), lq) {
			names = append(names, s.ds.Weapons[i].Name)
		}
	}
	return nil, names
}

// Part 查配件（求解器与工具组装输出用）
func (s *Store) Part(id string) *PartInfo {
	return s.ds.Parts[id]
}
