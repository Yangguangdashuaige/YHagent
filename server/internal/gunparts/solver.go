// internal/gunparts/solver.go
//
// 改枪组合枚举求解器：在"槽位选择空间"上做带剪枝的深度优先枚举，
// 返回满足属性约束的全部组合统计。纯函数、零依赖、毫秒级处理千万级空间。
//
// 语义约定：
//   - 约束作用于"配件修正值总和"（游戏改枪面板显示的 +/- 值），乘区百分比修正不计入；
//   - 无出厂固定件的槽位追加"未安装"选项（空装=贡献 0），固定件槽位本身就是默认态；
//   - 条件槽（枪口随枪管）按所选提供者求值：选项集合随提供者变化，封锁/不存在均等价"未安装"。
package gunparts

import (
	"fmt"
	"sort"
	"time"
)

// Option 槽位候选配件（已解析为约束属性下的贡献值）
type Option struct {
	Id   string  // 配件 ID；空串=未安装
	Name string  // 配件中文名（未安装）
	Add  float64 // 该配件在约束属性上的加成
}

// SlotView 静态槽位选择视图
type SlotView struct {
	Slot    string
	Options []Option
}

// CondView 条件槽位视图：选项由提供者（某静态槽的所选配件）决定
type CondView struct {
	Slot       string
	SlotIdx    int              // 提供者所在静态槽序号；-1=提供者不在任何槽，槽永不可用
	GroupByOpt map[int][]Option // 提供者选项序号 → 该组可选配件（已含"未安装"）；缺失=该提供者下槽不可用
	MaxOpt     float64          // 乐观上界（含未安装 0，用于剪枝）
	MinOpt     float64          // 乐观下界
}

// emptyOpt 预置"未安装"单选项（条件槽封锁/不存在时复用）
var emptyOpt = []Option{{Id: "", Name: "未安装"}}

// Op 约束方向
type Op string

const (
	OpGte Op = "gte" // sum ≥ value
	OpLte Op = "lte" // sum ≤ value
	OpEq  Op = "eq"  // sum == value
)

const (
	maxEval    = 50_000_000 // 完整组合评估上限：超过直接中止，防止病态空间拖垮请求
	matchEps   = 1e-9       // 浮点容差
	defaultTop = 10
)

// Combo 一条满足约束的组合
type Combo struct {
	Sum   float64           `json:"sum"`
	Parts map[string]string `json:"parts"` // 槽位名 → 配件名
}

// SlotUsage top-K 集合内某配件的使用频次
type SlotUsage struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// SlotStat 单槽位统计
type SlotStat struct {
	Feasible int         `json:"feasible"`  // 满足约束组合中出现过的选项数
	Total    int         `json:"total"`     // 该槽全部选项数（含未安装）
	TopUsage []SlotUsage `json:"top_usage"` // top-K 内使用频次前 3
}

// SolveResult 求解结果
type SolveResult struct {
	Matches   int64                `json:"matches"`  // 满足约束的组合总数
	Evaluated int64                `json:"evaluated"` // 完整评估的组合数（剪枝后）
	Pruned    int64                `json:"pruned"`   // 被上界剪枝跳过的分支数
	ElapsedMs int64                `json:"elapsed_ms"`
	Top       []Combo              `json:"top"`   // 最优 top-N（gte 降序 / lte 升序）
	Slots     map[string]*SlotStat `json:"slots"` // 静态槽可行性与使用分布
}

// Solve 在选择空间上枚举满足约束的全部组合。
// op 为空时默认 gte；topN<=0 时取默认 10，上限 20。
func Solve(slots []SlotView, cond []CondView, op Op, target float64, topN int) (*SolveResult, error) {
	if op == "" {
		op = OpGte
	}
	if op != OpGte && op != OpLte && op != OpEq {
		return nil, fmt.Errorf("未知约束方向 %q（支持 gte/lte/eq）", op)
	}
	if topN <= 0 {
		topN = defaultTop
	}
	if topN > 20 {
		topN = 20
	}
	if len(slots) == 0 {
		return nil, fmt.Errorf("选择空间为空")
	}

	r := &runner{
		op:       op,
		target:   target,
		topN:     topN,
		slots:    slots,
		cond:     cond,
		path:     make([]int, len(slots)),
		condPath: make([]Option, len(cond)),
		feasible: make([][]bool, len(slots)),
		result:   &SolveResult{Slots: make(map[string]*SlotStat, len(slots))},
	}
	for i := range slots {
		r.feasible[i] = make([]bool, len(slots[i].Options))
	}

	start := time.Now()
	r.buildBounds()
	r.dfs(0, 0)
	r.result.ElapsedMs = time.Since(start).Milliseconds()

	if r.aborted {
		return nil, fmt.Errorf("组合空间超过评估上限 %d（已评估 %d），请提高约束数值或换更小的目标范围收窄", maxEval, r.result.Evaluated)
	}
	r.finalize()
	return r.result, nil
}

// runner DFS 执行器
type runner struct {
	op         Op
	target     float64
	topN       int
	slots      []SlotView
	cond       []CondView
	path       []int     // 静态槽当前选择序号
	condPath   []Option  // 条件槽当前选择
	feasible   [][]bool  // 静态槽选项命中置位
	suffixMax  []float64 // 剪枝上界
	suffixMin  []float64 // 剪枝下界
	result     *SolveResult
	aborted    bool
}

// buildBounds 预计算后缀上/下界：从槽 i 到末尾（含全部条件槽的乐观值）能贡献的极值
func (r *runner) buildBounds() {
	n := len(r.slots)
	r.suffixMax = make([]float64, n+1)
	r.suffixMin = make([]float64, n+1)
	var condMax, condMin float64
	for _, c := range r.cond {
		condMax += c.MaxOpt
		condMin += c.MinOpt
	}
	r.suffixMax[n] = condMax
	r.suffixMin[n] = condMin
	for i := n - 1; i >= 0; i-- {
		var mx, mn float64
		if len(r.slots[i].Options) > 0 {
			mx, mn = r.slots[i].Options[0].Add, r.slots[i].Options[0].Add
			for _, o := range r.slots[i].Options[1:] {
				if o.Add > mx {
					mx = o.Add
				}
				if o.Add < mn {
					mn = o.Add
				}
			}
		}
		r.suffixMax[i] = mx + r.suffixMax[i+1]
		r.suffixMin[i] = mn + r.suffixMin[i+1]
	}
}

// dfs 枚举静态槽：前缀 + 剩余极值无法满足约束时整棵子树剪枝
func (r *runner) dfs(i int, cur float64) {
	if r.aborted {
		return
	}
	if (r.op == OpGte || r.op == OpEq) && cur+r.suffixMax[i] < r.target-matchEps {
		r.result.Pruned++
		return
	}
	if (r.op == OpLte || r.op == OpEq) && cur+r.suffixMin[i] > r.target+matchEps {
		r.result.Pruned++
		return
	}
	if i == len(r.slots) {
		r.evalCond(0, cur)
		return
	}
	for k := range r.slots[i].Options {
		r.path[i] = k
		r.dfs(i+1, cur+r.slots[i].Options[k].Add)
		if r.aborted {
			return
		}
	}
}

// evalCond 静态槽选完后求值条件槽（选项随提供者变化），全部定下再判定
func (r *runner) evalCond(j int, cur float64) {
	if r.aborted {
		return
	}
	if j == len(r.cond) {
		r.emit(cur)
		return
	}
	c := r.cond[j]
	var opts []Option
	if c.SlotIdx >= 0 {
		opts = c.GroupByOpt[r.path[c.SlotIdx]]
	}
	if opts == nil {
		opts = emptyOpt // 提供者未开放该槽（封锁/不存在）→ 只能未安装
	}
	for _, o := range opts {
		r.condPath[j] = o
		r.evalCond(j+1, cur+o.Add)
		if r.aborted {
			return
		}
	}
}

// emit 一个完整组合评估完毕：判定约束、计数、置位、维护 top-K
func (r *runner) emit(cur float64) {
	r.result.Evaluated++
	if r.result.Evaluated > maxEval {
		r.aborted = true
		return
	}
	ok := false
	switch r.op {
	case OpGte:
		ok = cur >= r.target-matchEps
	case OpLte:
		ok = cur <= r.target+matchEps
	case OpEq:
		ok = cur >= r.target-matchEps && cur <= r.target+matchEps
	}
	if !ok {
		return
	}
	r.result.Matches++
	for i := range r.slots {
		r.feasible[i][r.path[i]] = true
	}
	pos := r.topInsertPos(cur)
	if pos == -1 {
		return
	}
	parts := make(map[string]string, len(r.slots)+len(r.cond))
	for i := range r.slots {
		parts[r.slots[i].Slot] = r.slots[i].Options[r.path[i]].Name
	}
	for j := range r.cond {
		parts[r.cond[j].Slot] = r.condPath[j].Name
	}
	r.result.Top = append(r.result.Top, Combo{})
	copy(r.result.Top[pos+1:], r.result.Top[pos:])
	r.result.Top[pos] = Combo{Sum: cur, Parts: parts}
	if len(r.result.Top) > r.topN {
		r.result.Top = r.result.Top[:r.topN]
	}
}

// topInsertPos 返回新组合在 top 中的插入位置；-1=进不了榜
func (r *runner) topInsertPos(sum float64) int {
	if len(r.result.Top) >= r.topN {
		last := r.result.Top[len(r.result.Top)-1].Sum
		better := false
		if r.op == OpLte {
			better = sum < last-matchEps // lte 升序：保留最小
		} else {
			better = sum > last+matchEps // gte/eq 降序：保留最大
		}
		if !better {
			return -1
		}
	}
	lo, hi := 0, len(r.result.Top)
	for lo < hi {
		mid := (lo + hi) / 2
		var isGreater bool
		if r.op == OpLte {
			isGreater = sum < r.result.Top[mid].Sum
		} else {
			isGreater = sum > r.result.Top[mid].Sum
		}
		if isGreater {
			hi = mid
		} else {
			lo = mid + 1
		}
	}
	return lo
}

// finalize 汇总槽位统计（可行性 + top-K 内使用分布）
func (r *runner) finalize() {
	for i := range r.slots {
		stat := &SlotStat{Total: len(r.slots[i].Options)}
		for _, hit := range r.feasible[i] {
			if hit {
				stat.Feasible++
			}
		}
		usage := map[string]int{}
		for _, c := range r.result.Top {
			if name := c.Parts[r.slots[i].Slot]; name != "" {
				usage[name]++
			}
		}
		stat.TopUsage = topUsage(usage, 3)
		r.result.Slots[r.slots[i].Slot] = stat
	}
}

func topUsage(usage map[string]int, n int) []SlotUsage {
	if len(usage) == 0 {
		return nil
	}
	list := make([]SlotUsage, 0, len(usage))
	for name, cnt := range usage {
		list = append(list, SlotUsage{Name: name, Count: cnt})
	}
	sort.Slice(list, func(a, b int) bool {
		if list[a].Count != list[b].Count {
			return list[a].Count > list[b].Count
		}
		return list[a].Name < list[b].Name
	})
	if len(list) > n {
		list = list[:n]
	}
	return list
}

// BuildSpace 把武器条目转换为求解器选择空间。
//   - 静态槽：全部选项解析为 attr 下的 add 贡献；无出厂固定件的槽追加"未安装"；
//   - 条件槽：按提供者所在静态槽建索引，组选项追加"未安装"；
//   - mode 下无属性数据的配件按贡献 0 处理。
func (st *Store) BuildSpace(w *WeaponInfo, mode string, attr string) ([]SlotView, []CondView) {
	addOf := func(id string) (float64, string) {
		p := st.ds.Parts[id]
		if p == nil {
			return 0, "未知配件 " + id
		}
		if m, ok := p.Modes[mode]; ok && m != nil {
			return m.Add[attr], p.Name
		}
		return 0, p.Name
	}

	slots := make([]SlotView, 0, len(w.Sockets))
	type loc struct{ slot, idx int }
	where := map[string]loc{} // 配件 ID → 静态槽/选项序号（条件槽提供者定位用）
	for si, sock := range w.Sockets {
		sv := SlotView{Slot: sock.Slot}
		hasFixed := false
		for _, id := range sock.Options {
			add, name := addOf(id)
			if p := st.ds.Parts[id]; p != nil && p.Selectable != nil && !*p.Selectable {
				hasFixed = true
			}
			sv.Options = append(sv.Options, Option{Id: id, Name: name, Add: add})
			where[id] = loc{si, len(sv.Options) - 1}
		}
		if !hasFixed {
			// 可选槽允许空装（贡献 0）；出厂固定件槽的默认态即固定件本身
			sv.Options = append(sv.Options, Option{Id: "", Name: "未安装"})
		}
		slots = append(slots, sv)
	}

	cond := make([]CondView, 0, len(w.ConditionalSockets))
	for _, cs := range w.ConditionalSockets {
		cv := CondView{Slot: cs.Slot, SlotIdx: -1, GroupByOpt: map[int][]Option{}}
		for _, group := range cs.Providers {
			var opts []Option
			for _, id := range group.Options {
				add, name := addOf(id)
				opts = append(opts, Option{Id: id, Name: name, Add: add})
			}
			opts = append(opts, Option{Id: "", Name: "未安装"})
			for _, pid := range group.ProviderIds {
				if l, ok := where[pid]; ok {
					cv.SlotIdx = l.slot
					cv.GroupByOpt[l.idx] = opts
				}
			}
			for _, o := range opts {
				if o.Add > cv.MaxOpt {
					cv.MaxOpt = o.Add
				}
				if o.Add < cv.MinOpt {
					cv.MinOpt = o.Add
				}
			}
		}
		cond = append(cond, cv)
	}
	return slots, cond
}
