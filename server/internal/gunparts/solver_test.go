package gunparts

import (
	"os"
	"testing"
)

// fixture：2 静态槽 + 1 条件槽（枪口由枪管提供），手算验证
//
//	枪管槽: B1(+4) B2(-2)          握把槽: G1(+2) G2(+1)
//	条件槽枪口: B1→[M1(+2), M2(-1)]；B2 无枪口（未提供）
func fixtureSpace() ([]SlotView, []CondView) {
	slots := []SlotView{
		{Slot: "枪管", Options: []Option{
			{Id: "B1", Name: "枪管一", Add: 4},
			{Id: "B2", Name: "枪管二", Add: -2},
		}},
		{Slot: "握把", Options: []Option{
			{Id: "G1", Name: "握把一", Add: 2},
			{Id: "G2", Name: "握把二", Add: 1},
		}},
	}
	// BuildSpace 会为条件槽追加"未安装"（+0），此处手工构造含未安装
	cond := []CondView{{
		Slot: "枪口", SlotIdx: 0,
		GroupByOpt: map[int][]Option{
			0: {
				{Id: "M1", Name: "消音器", Add: 2},
				{Id: "M2", Name: "消焰器", Add: -1},
				{Id: "", Name: "未安装", Add: 0},
			},
			// B2（选项序号 1）缺失 = 未提供枪口槽 → 未安装
		},
		MaxOpt: 2, MinOpt: -1,
	}}
	return slots, cond
}

func TestSolveGte(t *testing.T) {
	slots, cond := fixtureSpace()
	res, err := Solve(slots, cond, OpGte, 5, 10)
	if err != nil {
		t.Fatal(err)
	}
	// 手算（B2 全剪：-2+0+2=0 < 5）：
	//   B1+M1+G1=8✓  B1+M1+G2=7✓  B1+空+G1=6✓  B1+M2+G1=5✓  B1+M2+G2=4✗  B1+空+G2=5✓ → 5 条
	if res.Matches != 5 {
		t.Errorf("matches 期望 5，得到 %d", res.Matches)
	}
	if len(res.Top) != 5 {
		t.Fatalf("top 期望 5 条，得到 %d", len(res.Top))
	}
	wantDesc := []float64{8, 7, 6, 5, 5}
	for i, c := range res.Top {
		if c.Sum != wantDesc[i] {
			t.Errorf("top[%d] 期望 %.0f，得到 %.0f", i, wantDesc[i], c.Sum)
		}
	}
	if res.Top[0].Parts["枪管"] != "枪管一" || res.Top[0].Parts["枪口"] != "消音器" || res.Top[0].Parts["握把"] != "握把一" {
		t.Errorf("top[0] 配件名不对: %v", res.Top[0].Parts)
	}
	// 评估数：B2 分支在静态层剪掉（suffixMax 检查在槽 0 入口），evaluated = B1 组合 3×2=6
	if res.Evaluated != 6 {
		t.Errorf("evaluated 期望 6，得到 %d", res.Evaluated)
	}
	// 可行性：枪管槽只有 B1 可行（1/2），握把槽 G1、G2 都可行（2/2）
	if res.Slots["枪管"].Feasible != 1 || res.Slots["握把"].Feasible != 2 {
		t.Errorf("feasible 期望 枪管1/握把2，得到 %d/%d", res.Slots["枪管"].Feasible, res.Slots["握把"].Feasible)
	}
}

func TestSolveLte(t *testing.T) {
	slots, cond := fixtureSpace()
	res, err := Solve(slots, cond, OpLte, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	// 手算（B1 最小 4+(-1)+1=4 > 0 全剪）：
	//   B2 无枪口=未安装：B2+G2=-1✓  B2+G1=0✓ → 2 条，升序
	if res.Matches != 2 {
		t.Fatalf("matches 期望 2，得到 %d", res.Matches)
	}
	if res.Top[0].Sum != -1 || res.Top[1].Sum != 0 {
		t.Errorf("lte 应升序，得到 %.0f, %.0f", res.Top[0].Sum, res.Top[1].Sum)
	}
	if res.Top[0].Parts["枪口"] != "未安装" {
		t.Errorf("B2 无枪口应为未安装，得到 %q", res.Top[0].Parts["枪口"])
	}
}

func TestSolveEq(t *testing.T) {
	slots, cond := fixtureSpace()
	res, err := Solve(slots, cond, OpEq, 5, 10)
	if err != nil {
		t.Fatal(err)
	}
	// B1+M2+G1=5、B1+空+G2=5 → 2 条
	if res.Matches != 2 {
		t.Fatalf("matches 期望 2，得到 %d", res.Matches)
	}
	for _, c := range res.Top {
		if c.Sum != 5 {
			t.Errorf("eq 结果应为 5，得到 %.0f", c.Sum)
		}
	}
}

func TestSolveTopNLimit(t *testing.T) {
	slots, cond := fixtureSpace()
	res, err := Solve(slots, cond, OpGte, -100, 3) // 全部 8 条组合都命中（B1:3枪口×2握把=6，B2:未安装×2握把=2）
	if err != nil {
		t.Fatal(err)
	}
	if res.Matches != 8 {
		t.Errorf("matches 期望 8，得到 %d", res.Matches)
	}
	if len(res.Top) != 3 {
		t.Errorf("top 应截断为 3，得到 %d", len(res.Top))
	}
}

// BuildSpace 测试：真实 schema → 选择空间转换（空装/固定件/条件槽提供者定位）
func TestBuildSpace(t *testing.T) {
	st := &Store{}
	fixed := false
	st.ds = Dataset{
		Parts: map[string]*PartInfo{
			"B1": {Id: "B1", Name: "枪管一", Modes: map[string]*AttrDelta{
				"sol": {Add: map[string]float64{"stability": 4}},
			}},
			"B2": {Id: "B2", Name: "固定枪管", Selectable: &fixed, Modes: map[string]*AttrDelta{
				"sol": {Add: map[string]float64{"stability": 1}},
			}},
			"G1": {Id: "G1", Name: "握把一", Modes: map[string]*AttrDelta{
				"sol": {Add: map[string]float64{"stability": 2}},
			}},
			"M1": {Id: "M1", Name: "消音器", Modes: map[string]*AttrDelta{
				"sol": {Add: map[string]float64{"stability": 2}},
			}},
		},
		Weapons: []WeaponInfo{{
			Id: "W", Name: "测试枪",
			Sockets: []SocketInfo{
				{Slot: "枪管", Options: []string{"B1", "B2"}}, // 含固定件 → 不加未安装
				{Slot: "握把", Options: []string{"G1"}},          // 无固定件 → 加未安装
			},
			ConditionalSockets: []ConditionalSocket{{
				Slot: "枪口",
				Providers: []ProviderOptions{
					{ProviderIds: []string{"B1"}, Options: []string{"M1"}},
					{ProviderIds: []string{"B2"}, Options: []string{}}, // 封锁
				},
			}},
		}},
	}
	st.byLower = map[string]*WeaponInfo{"测试枪": &st.ds.Weapons[0]}
	w, _ := st.FindWeapon("测试枪")
	if w == nil {
		t.Fatal("武器未找到")
	}
	slots, cond := st.BuildSpace(w, "sol", "stability")

	// 枪管槽：B1(+4) B2(+1)，含固定件 → 无未安装
	if len(slots[0].Options) != 2 {
		t.Errorf("含固定件的槽不应加未安装，选项数 %d", len(slots[0].Options))
	}
	if slots[0].Options[0].Add != 4 {
		t.Errorf("B1 stability add 期望 4")
	}
	// 握把槽：G1(+2) + 未安装(0)
	if len(slots[1].Options) != 2 {
		t.Fatalf("无固定件的槽应加未安装，选项数 %d", len(slots[1].Options))
	}
	if slots[1].Options[1].Name != "未安装" || slots[1].Options[1].Add != 0 {
		t.Errorf("未安装选项应为 0 贡献")
	}
	// 条件槽：B1 → [M1(+2), 未安装]；B2（固定件封锁）→ 只有未安装
	if len(cond) != 1 || cond[0].SlotIdx != 0 {
		t.Fatalf("条件槽应定位到枪管槽（SlotIdx=0），得到 %d 槽 %v", len(cond), cond)
	}
	grp := cond[0].GroupByOpt[0] // B1 选项序号 0
	if len(grp) != 2 || grp[0].Name != "消音器" || grp[0].Add != 2 || grp[1].Name != "未安装" {
		t.Errorf("B1 组应为 [消音器+2, 未安装]，得到 %v", grp)
	}
	if grp, blocked := cond[0].GroupByOpt[1]; blocked {
		// B2（封锁枪口）组 = 只有未安装
		if len(grp) != 1 || grp[0].Name != "未安装" || grp[0].Add != 0 {
			t.Errorf("B2 封锁组应仅有未安装，得到 %v", grp)
		}
	} else {
		t.Errorf("B2 是静态槽选项，封锁组应登记为仅未安装")
	}
	if cond[0].MaxOpt != 2 || cond[0].MinOpt != 0 {
		t.Errorf("条件槽乐观界应 Max=2 Min=0（空组不计负），得到 %.0f/%.0f", cond[0].MaxOpt, cond[0].MinOpt)
	}

	// 端到端小验证：stability ≥ 7 → 只有 B1+消音器+握把一 = 8 一条
	res, err := Solve(slots, cond, OpGte, 7, 10)
	if err != nil {
		t.Fatal(err)
	}
	if res.Matches != 1 || res.Top[0].Sum != 8 {
		t.Errorf("gte 7 期望唯一解 B1+M1+G1=8，得到 matches=%d", res.Matches)
	}
}

// TestSolveRealData 真实数据冒烟（开发机有 data/compat/gunparts.json 才跑）：
// 原始问题"M14 据枪+8 的所有改法"端到端验证
func TestSolveRealData(t *testing.T) {
	const path = "../../data/compat/gunparts.json"
	if _, err := os.Stat(path); err != nil {
		t.Skip("真实数据文件不存在，跳过冒烟")
	}
	st, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	w, cands := st.FindWeapon("M14")
	if w == nil {
		t.Fatalf("M14 未找到，候选 %v", cands)
	}
	slots, cond := st.BuildSpace(w, "sol", "stability")
	// 组合空间合理性：M14 静态槽 + 枪口条件槽
	if len(slots) != len(w.Sockets) {
		t.Fatalf("静态槽数不符")
	}
	res, err := Solve(slots, cond, OpGte, 8, 10)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("M14 据枪≥8：matches=%d evaluated=%d pruned=%d elapsed=%dms",
		res.Matches, res.Evaluated, res.Pruned, res.ElapsedMs)
	if res.Matches < 1 {
		t.Fatalf("M14 据枪+8 应有解（此前勘察确认洞察枪管+4 等正修正存在），得到 0")
	}
	if res.ElapsedMs > 5000 {
		t.Errorf("真实数据求解耗时异常：%dms", res.ElapsedMs)
	}
	for _, c := range res.Top {
		if c.Sum < 8 {
			t.Errorf("gte 8 结果混入 <8 的组合：%.0f", c.Sum)
		}
	}
	// 覆盖上限保护路径：病态宽松约束 + 巨大武器（K416 约 2800 万组合，含未安装）应可完成或优雅报错
	k, _ := st.FindWeapon("K416")
	if k != nil {
		kslots, kcond := st.BuildSpace(k, "sol", "stability")
		kres, kerr := Solve(kslots, kcond, OpGte, 100, 5) // 不可达约束 → 纯剪枝路径
		if kerr != nil {
			t.Logf("K416 gte 100：%v（剪枝路径应快速完成）", kerr)
		} else {
			t.Logf("K416 据枪≥100：matches=%d evaluated=%d elapsed=%dms", kres.Matches, kres.Evaluated, kres.ElapsedMs)
			if kres.Matches != 0 {
				t.Errorf("据枪≥100 不可能有解")
			}
		}
	}
}
