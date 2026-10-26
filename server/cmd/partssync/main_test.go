package main

import (
	"encoding/json"
	"testing"
)

func TestExtractAttrDelta(t *testing.T) {
	idxToKey := map[string]string{"2": "effectiveRange", "3": "recoilControl", "6": "stability"}
	raw := `[
		{"operation":"StaticAttributeModify","target":"DisplayAttrValues.3","modifier":"Addend","value":2},
		{"operation":"StaticAttributeModify","target":"DisplayAttrValues.6","modifier":"Addend","value":-4},
		{"operation":"StaticAttributeModify","target":"DisplayAttrValues.2","modifier":"Addend","value":0},
		{"operation":"StaticAttributeModify","target":"DisplayAttrValues.99","modifier":"Addend","value":5},
		{"operation":"StaticAttributeModify","target":"WeaponMainAttribute.MainAttrValues.3","modifier":"Addend","value":9},
		{"operation":"StaticAttributeModify","target":"DisplayAttrValues.6","modifier":"Mult_A","value":0.1},
		{"operation":"OtherOp","target":"DisplayAttrValues.3","modifier":"Addend","value":7},
		{"operation":"StaticAttributeModify","target":"DisplayAttrValues.3","modifier":"Addend","value":"ref-str"}
	]`
	var effects rawEffects
	if err := json.Unmarshal([]byte(raw), &effects); err != nil {
		t.Fatal(err)
	}
	d := extractAttrDelta(effects, idxToKey)
	if d == nil {
		t.Fatal("应为非 nil")
	}
	if d.Add["recoilControl"] != 2 {
		t.Errorf("recoilControl 期望 2，得到 %v", d.Add["recoilControl"])
	}
	if d.Add["stability"] != -4 {
		t.Errorf("stability 期望 -4，得到 %v", d.Add["stability"])
	}
	if _, ok := d.Add["effectiveRange"]; ok {
		t.Error("0 值加成不应记录")
	}
	if _, ok := d.Add["unknown"]; ok {
		t.Error("未知索引不应记录")
	}
	if d.Mul["stability"] != 0.1 {
		t.Errorf("乘区修正期望 0.1，得到 %v", d.Mul["stability"])
	}
	if len(d.Add) != 2 || len(d.Mul) != 1 {
		t.Errorf("映射大小不符: add=%d mul=%d", len(d.Add), len(d.Mul))
	}
}

func TestExtractAttrDeltaAllNoop(t *testing.T) {
	idxToKey := map[string]string{"3": "recoilControl"}
	raw := `[{"operation":"StaticAttributeModify","target":"DisplayAttrValues.3","modifier":"Addend","value":0}]`
	var effects rawEffects
	_ = json.Unmarshal([]byte(raw), &effects)
	if d := extractAttrDelta(effects, idxToKey); d != nil {
		t.Errorf("全为 no-op 时应返回 nil")
	}
}

func TestPartMergerIngestAndNames(t *testing.T) {
	m := &partMerger{
		sharedParts: map[string]*PartInfo{},
		itemNames:   map[string]string{"13130000155": "实用消焰器（本地化）", "99990000001": "本地化名"},
		idxToKey:    map[string]string{"6": "stability"},
	}
	// shard：一个有真名的可选配件 + 一个内部名固定件
	shardDisplay := rawDisplay{
		Objects: map[string]rawDisplayObject{
			"13130000155": {ObjectType: "part", CategoryId: "muzzle", RarityId: "绿", PlayerSelectable: true},
			"13020000384": {ObjectType: "part", CategoryId: "barrel", PlayerSelectable: false},
		},
		Strings: map[string]string{
			"13130000155": "实用消焰器",
			"13020000384": "内部配件 13020000384",
		},
	}
	shardEffects := map[string]map[string]rawEffects{
		"13130000155": {"sol": {
			{Operation: "StaticAttributeModify", Target: "DisplayAttrValues.6", Modifier: "Addend", Value: 3.0},
		}},
	}
	m.ingest(shardDisplay, shardEffects)

	// core：包含武器本体 display（ objectType=weapon，不应登记为配件）+
	// 一个只有 effects 没有 display 的配件（保底登记）+ 武器名
	coreDisplay := rawDisplay{
		Objects: map[string]rawDisplayObject{
			"18050000005": {ObjectType: "weapon", CategoryId: "marksmanRifle", PlayerSelectable: true},
		},
		Strings: map[string]string{"18050000005": "M14"},
	}
	coreEffects := map[string]map[string]rawEffects{
		"18050000005": {"sol": {
			{Operation: "StaticAttributeModify", Target: "DisplayAttrValues.6", Modifier: "Addend", Value: 99.0},
		}},
		"99990000001": {"sol": {
			{Operation: "StaticAttributeModify", Target: "DisplayAttrValues.6", Modifier: "Addend", Value: 1.0},
		}},
	}
	m.ingest(coreDisplay, coreEffects)
	m.resolveNames()

	if _, ok := m.sharedParts["18050000005"]; ok {
		t.Error("武器本体不应被登记为配件")
	}
	p := m.sharedParts["13130000155"]
	if p.Name != "实用消焰器" {
		t.Errorf("display 名优先，期望 实用消焰器，得到 %s", p.Name)
	}
	if p.Modes["sol"].Add["stability"] != 3 {
		t.Errorf("据枪稳定性期望 3")
	}
	if p.Selectable == nil || !*p.Selectable {
		t.Error("selectable 应为 true")
	}
	fixed := m.sharedParts["13020000384"]
	if fixed.Selectable == nil || *fixed.Selectable {
		t.Error("固定件 selectable 应为 false")
	}
	if fixed.Name != "内部配件 13020000384" {
		t.Errorf("内部件名称期望保留，得到 %s", fixed.Name)
	}
	if m.unnamedCount != 1 {
		t.Errorf("unnamedCount 期望 1（13020000384 为内部名），得到 %d", m.unnamedCount)
	}
	orphan := m.sharedParts["99990000001"]
	if orphan == nil || orphan.Name != "本地化名" {
		t.Errorf("无 display 配件应走 items.json fallback，得到 %v", orphan)
	}
}

func TestResolveNamesInternalFallback(t *testing.T) {
	m := &partMerger{
		sharedParts: map[string]*PartInfo{
			"88880000001": {Id: "88880000001", Modes: map[string]*AttrDelta{}},
		},
		itemNames: map[string]string{},
		idxToKey:  map[string]string{},
	}
	m.resolveNames()
	if m.sharedParts["88880000001"].Name != "内部配件 88880000001" {
		t.Errorf("最终 fallback 期望 内部配件 前缀名")
	}
	if m.unnamedCount != 1 {
		t.Errorf("unnamedCount 期望 1")
	}
}

func TestMergeConditionalSockets(t *testing.T) {
	raw := `{
		"ammoItemIds": ["37170200001"],
		"assembly": {
			"socketProviders": {
				"18050000005": {
					"sockets": [
						{"socketId": 2, "optionItemIds": ["13020000384", "13020000387"]}
					]
				},
				"13020000384": {
					"sockets": [
						{"socketId": 6, "optionItemIds": ["13130000164", "13130000169", "13130000208"]}
					]
				},
				"13020000387": {
					"sockets": [
						{"socketId": 6, "optionItemIds": ["13130000164", "13130000169", "13130000208"]},
						{"socketId": 40, "optionItemIds": null}
					]
				},
				"13020000388": {
					"sockets": [
						{"socketId": 6, "optionItemIds": ["13130000165"]}
					]
				}
			}
		}
	}`
	var sel rawSelection
	if err := json.Unmarshal([]byte(raw), &sel); err != nil {
		t.Fatal(err)
	}
	slotName := func(id int) string { return map[int]string{6: "枪口", 40: "槽位40"}[id] }
	cs := mergeConditionalSockets(&sel, "18050000005", slotName)
	if len(cs) != 2 {
		t.Fatalf("条件槽期望 2 个（socket6/40），得到 %d", len(cs))
	}
	// socketId 升序：6 在前
	s6 := cs[0]
	if s6.Slot != "枪口" || s6.SocketId != 6 {
		t.Errorf("第一个条件槽应为枪口(6)，得到 %d/%s", s6.SocketId, s6.Slot)
	}
	if len(s6.Providers) != 2 {
		t.Fatalf("枪口槽提供者组期望 2 组（相同选项合并），得到 %d", len(s6.Providers))
	}
	first := s6.Providers[0]
	if len(first.ProviderIds) != 2 || first.ProviderIds[0] != "13020000384" || first.ProviderIds[1] != "13020000387" {
		t.Errorf("选项相同的枪管应合并且按 ID 排序，得到 %v", first.ProviderIds)
	}
	if len(first.Options) != 3 || first.Options[0] != "13130000164" {
		t.Errorf("合并组选项期望 3 个，得到 %v", first.Options)
	}
	second := s6.Providers[1]
	if len(second.ProviderIds) != 1 || second.ProviderIds[0] != "13020000388" {
		t.Errorf("不同选项的枪管应独立成组，得到 %v", second.ProviderIds)
	}
	// socket 40：null 选项 → 空数组（封锁语义），槽名 fallback
	s40 := cs[1]
	if s40.Slot != "槽位40" {
		t.Errorf("槽名 fallback 期望 槽位40，得到 %s", s40.Slot)
	}
	if len(s40.Providers) != 1 || s40.Providers[0].Options == nil || len(s40.Providers[0].Options) != 0 {
		t.Errorf("null 选项应规范化为空数组（封锁语义），得到 %#v", s40.Providers)
	}
}

func TestMergeConditionalSocketsEmpty(t *testing.T) {
	// 只有武器本体 provider：无条件槽
	raw := `{"assembly":{"socketProviders":{"18050000005":{"sockets":[{"socketId":2,"optionItemIds":["x"]}]}}}}`
	var sel rawSelection
	if err := json.Unmarshal([]byte(raw), &sel); err != nil {
		t.Fatal(err)
	}
	if cs := mergeConditionalSockets(&sel, "18050000005", func(int) string { return "" }); cs != nil {
		t.Errorf("无配件 provider 时应返回 nil，得到 %v", cs)
	}
}
