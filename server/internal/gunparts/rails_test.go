package gunparts

import (
	"strings"
	"testing"
)

// TestM14RailSlotsSynced 同步产物中 M14 应含导轨/贴片/激光条件槽
// （partssync 阶段 31：shard 内嵌 socketProviders 来源接入后回归）
func TestM14RailSlotsSynced(t *testing.T) {
	st, err := NewStore("../../data/compat/gunparts.json")
	if err != nil {
		t.Fatalf("加载数据集: %v", err)
	}
	w, cands := st.FindWeapon("M14")
	if w == nil {
		t.Fatalf("M14 不在数据集（候选 %v）", cands)
	}
	want := map[string]bool{"上导轨": false, "左导轨": false, "右导轨": false, "上贴片": false, "前握把": false}
	for _, cs := range w.ConditionalSockets {
		if _, ok := want[cs.Slot]; ok {
			want[cs.Slot] = true
		}
	}
	for slot, got := range want {
		if !got {
			t.Errorf("M14 缺少条件槽 %s（导轨/贴片数据未同步）", slot)
		}
	}
	// 先进枪身系统应解锁左右上导轨
	for _, cs := range w.ConditionalSockets {
		if cs.Slot == "右导轨" {
			found := false
			for _, p := range cs.Providers {
				for _, pid := range p.ProviderIds {
					if part := st.Part(pid); part != nil && strings.Contains(part.Name, "先进枪身") {
						found = true
					}
				}
			}
			if !found {
				t.Errorf("右导轨 providers 中无先进枪身系统")
			}
		}
	}
}

// TestM14RailSolve M14 全槽 BuildSpace + Solve：
// 组合空间应包含导轨/贴片槽，激光配件（腰射精度）应出现在高腰射方案中
func TestM14RailSolve(t *testing.T) {
	st, err := NewStore("../../data/compat/gunparts.json")
	if err != nil {
		t.Fatalf("加载数据集: %v", err)
	}
	w, _ := st.FindWeapon("M14")
	slots, cond := st.BuildSpace(w, "sol", "waistAccuracy")
	if len(cond) < 10 {
		t.Fatalf("M14 条件槽应 ≥10（含导轨/贴片/握把），实际 %d", len(cond))
	}
	res, err := Solve(slots, cond, OpGte, 20, 3) // 腰射≥20：需要导轨激光/贴片的正修正才可达
	if err != nil {
		t.Fatalf("求解报错: %v", err)
	}
	if res == nil || len(res.Top) == 0 {
		t.Fatalf("求解无结果（matches=%d evaluated=%d pruned=%d elapsed=%dms）",
			res.Matches, res.Evaluated, res.Pruned, res.ElapsedMs)
	}
	t.Logf("matches=%d evaluated=%d pruned=%d elapsed=%dms top=%d",
		res.Matches, res.Evaluated, res.Pruned, res.ElapsedMs, len(res.Top))
	// Top 方案须出现导轨/贴片/握把槽的实名配件（激光镭指/护木片等，非"未安装"）
	railHit, laserHit := false, false
	for _, c := range res.Top {
		for slot, name := range c.Parts {
			if name == "未安装" {
				continue
			}
			if strings.Contains(slot, "导轨") || strings.Contains(slot, "贴片") || strings.Contains(slot, "握把") {
				railHit = true
				if strings.Contains(name, "激光") {
					laserHit = true
				}
			}
		}
	}
	if !railHit {
		t.Errorf("Top 方案未含导轨/贴片/握把实名配件")
	}
	if !laserHit {
		t.Errorf("Top 方案未含激光配件（右导轨常规激光应出现在高腰射方案中）")
	}
}
