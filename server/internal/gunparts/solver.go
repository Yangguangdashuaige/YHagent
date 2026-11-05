// internal/gunparts/solver.go
//
// 改枪组合求解器：在"槽位选择空间"上统计满足属性约束的全部组合。
// 纯函数、零依赖。导轨/贴片等条件槽接入后（M14 条件槽 18 个）组合空间
// 可达 10^18 量级，纯枚举不可行，采用两段混合策略：
//   - 静态槽：带后缀界剪枝的 DFS 枚举；
//   - 条件槽（静态选择固定提供者后）：
//       小空间 → 精确枚举（保留逐组合评估语义）；
//       大空间 → 按槽位卷积出"总和 → 组合数"分布表，查表精确计数，
//                top 用各槽贪心极值代表组合填充（每个静态分支一条）。
//     分布表按"提供者选项集内容"缓存：仅换配件名不换选项集的静态分支
//     （如 45 个瞄准镜里多数对条件槽等效）共享同一张表。
//
// 语义约定：
//   - 约束作用于"配件修正值总和"（游戏改枪面板显示的 +/- 值），乘区百分比修正不计入；
//   - 无出厂固定件的槽位追加"未安装"选项（空装=贡献 0），固定件槽位本身就是默认态；
//   - 条件槽（枪口随枪管）按所选提供者求值：选项集合随提供者变化，封锁/不存在均等价"未安装"；
//   - gte/lte 的 Top 为各静态分支的贪心最优代表（分支内和值最优，非全局逐组合排序）。
package gunparts

import (
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"math"
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
	maxEval      = 50_000_000 // 完整组合评估上限：超过直接中止，防止病态空间拖垮请求
	matchEps     = 1e-9       // 浮点容差
	defaultTop   = 10
	maxDistSums  = 200_000  // 单张分布表的最大取值数（防浮点粒度爆炸）
	maxDPWork    = 30_000_000 // 全部分布表构建的总操作数上限
	minEnumSpace = 64        // 单叶枚举空间下限：再小也走精确枚举
	maxEnumSpace = 4_000_000 // 单叶枚举空间上限：超过走分布统计
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
		op:          op,
		target:      target,
		topN:        topN,
		slots:       slots,
		cond:        cond,
		path:        make([]int, len(slots)),
		condPath:    make([]Option, len(cond)),
		feasible:    make([][]bool, len(slots)),
		result:      &SolveResult{Slots: make(map[string]*SlotStat, len(slots))},
		dpCache:     make(map[string]*condDist),
		groupHash:   make([]map[int]uint64, len(cond)),
		enumBudget:  enumBudgetFor(slots),
	}
	for i := range slots {
		r.feasible[i] = make([]bool, len(slots[i].Options))
	}
	// 条件槽选项集内容哈希（缓存键）：同一提供者选项集 → 同一分布表
	r.hashEmpty = hashOptions(emptyOpt)
	for k, c := range cond {
		if c.SlotIdx < 0 {
			continue
		}
		r.groupHash[k] = make(map[int]uint64, len(c.GroupByOpt))
		for pidx, opts := range c.GroupByOpt {
			r.groupHash[k][pidx] = hashOptions(opts)
		}
	}

	start := time.Now()
	r.buildBounds()
	r.dfs(0, 0)
	r.result.ElapsedMs = time.Since(start).Milliseconds()

	if r.aborted {
		if r.abortErr != "" {
			return nil, fmt.Errorf("%s", r.abortErr)
		}
		return nil, fmt.Errorf("组合空间超过评估上限 %d（已评估 %d），请提高约束数值或换更小的目标范围收窄", maxEval, r.result.Evaluated)
	}
	r.finalize()
	return r.result, nil
}

// enumBudgetFor 单个静态叶子的条件槽枚举预算：静态分支越多，每支预算越少
// （总量约 maxEval/4），夹在 [minEnumSpace, maxEnumSpace] 之间；超出预算走分布统计。
func enumBudgetFor(slots []SlotView) int64 {
	var f float64 = 1
	for _, sv := range slots {
		f *= float64(len(sv.Options))
		if f > 1e9 {
			f = 1e9
			break
		}
	}
	b := float64(maxEval) / (f * 4)
	if b < minEnumSpace {
		b = minEnumSpace
	}
	if b > maxEnumSpace {
		b = maxEnumSpace
	}
	return int64(b)
}

// runner DFS 执行器
type runner struct {
	op             Op
	target         float64
	topN           int
	slots          []SlotView
	cond           []CondView
	path           []int     // 静态槽当前选择序号
	condPath       []Option  // 条件槽当前选择
	feasible       [][]bool  // 静态槽选项命中置位
	suffixMax      []float64 // 剪枝上界（静态槽后缀 + 全部条件槽乐观值）
	suffixMin      []float64 // 剪枝下界
	condSufMax     []float64 // 当前静态叶下条件槽后缀界（枚举路径 O(1) 剪枝用）
	condSufMin     []float64
	enumBudget     int64                 // 单叶条件槽枚举空间预算
	dpCache        map[string]*condDist  // 选项集内容键 → 条件槽总和分布表
	groupHash      []map[int]uint64      // 条件槽 k：提供者选项序号 → 选项集内容哈希
	hashEmpty      uint64                // 未安装单选项的内容哈希
	dpWork         int                   // 分布表累计构建操作数
	result         *SolveResult
	aborted        bool
	abortErr       string // 非空时中止原因（覆盖默认超限文案）
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
		r.evalAll(cur)
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

// evalAll 静态槽全部定选后求值条件槽（提供者已定）：小空间精确枚举，
// 大空间走分布统计。导轨/贴片接入后单叶条件空间可达 10^15+，必须分流。
func (r *runner) evalAll(cur float64) {
	m := len(r.cond)
	if m == 0 {
		r.emit(cur)
		return
	}
	space := 1
	for k := 0; k < m; k++ {
		space *= len(r.condOpts(k))
		if space > int(r.enumBudget) {
			r.evalDP(cur)
			return
		}
	}
	// 枚举路径：预计算本叶条件槽后缀界（提供者固定，界不随递归变化）
	sufMax := make([]float64, m+1)
	sufMin := make([]float64, m+1)
	for j := m - 1; j >= 0; j-- {
		mx, mn := 0.0, 0.0 // "未安装"=0 恒参与极值（组内必含未安装或以空组兜底）
		for _, o := range r.condOpts(j) {
			if o.Add > mx {
				mx = o.Add
			}
			if o.Add < mn {
				mn = o.Add
			}
		}
		sufMax[j] = sufMax[j+1] + mx
		sufMin[j] = sufMin[j+1] + mn
	}
	r.condSufMax, r.condSufMin = sufMax, sufMin
	r.evalCond(0, cur)
}

// evalCond 条件槽精确枚举（小空间）：后缀界 O(1) 剪枝，叶子走 emit。
func (r *runner) evalCond(j int, cur float64) {
	if r.aborted {
		return
	}
	if (r.op == OpGte || r.op == OpEq) && cur+r.condSufMax[j] < r.target-matchEps {
		r.result.Pruned++
		return
	}
	if (r.op == OpLte || r.op == OpEq) && cur+r.condSufMin[j] > r.target+matchEps {
		r.result.Pruned++
		return
	}
	if j == len(r.cond) {
		r.emit(cur)
		return
	}
	for _, o := range r.condOpts(j) {
		r.condPath[j] = o
		r.evalCond(j+1, cur+o.Add)
		if r.aborted {
			return
		}
	}
}

// condDist 条件槽总和分布表：达到每个总和的组合数（升序），
// 附各槽贪心极值代表（gte 取最大加成 / lte 取最小，平手优先实名配件）。
type condDist struct {
	sums   []float64 // 升序去重
	counts []int64   // sums[i] 的组合数
	suff   []int64   // suff[i] = Σ counts[i:]（countGE(sums[i])）
	best   []Option  // 各条件槽的贪心代表选项
	maxSum float64
	minSum float64
}

func (d *condDist) countGE(x float64) int64 {
	i := sort.Search(len(d.sums), func(i int) bool { return d.sums[i] >= x-matchEps })
	return d.suff[i]
}

func (d *condDist) countLE(x float64) int64 {
	i := sort.Search(len(d.sums), func(i int) bool { return d.sums[i] > x+matchEps })
	return d.suff[0] - d.suff[i]
}

func (d *condDist) countEQ(x float64) int64 {
	i := sort.Search(len(d.sums), func(i int) bool { return d.sums[i] >= x-matchEps })
	if i < len(d.sums) && d.sums[i] <= x+matchEps {
		return d.counts[i]
	}
	return 0
}

// evalDP 分布统计路径（大空间）：查/建当前选项集的分布表，精确计数满足
// 约束的组合数；gte/lte 用贪心极值代表组合参与 top（分支内和值最优）。
func (r *runner) evalDP(cur float64) {
	key := r.distKey()
	d := r.dpCache[key]
	if d == nil {
		d = r.buildDist()
		if r.aborted {
			return
		}
		r.dpCache[key] = d
	}
	r.result.Evaluated++ // 一叶一次评估（分布查询）
	if r.result.Evaluated > maxEval {
		r.aborted = true
		return
	}
	var count int64
	switch r.op {
	case OpGte:
		count = d.countGE(r.target - cur)
	case OpLte:
		count = d.countLE(r.target - cur)
	default:
		count = d.countEQ(r.target - cur)
	}
	if count == 0 {
		r.result.Pruned++
		return
	}
	r.result.Matches = saturatingAdd(r.result.Matches, count)
	for i := range r.slots {
		r.feasible[i][r.path[i]] = true
	}
	if r.op == OpEq {
		return // eq 精确命中组合需子集重构，不产 top 代表（计数仍精确）
	}
	sum := cur + d.maxSum
	if r.op == OpLte {
		sum = cur + d.minSum
	}
	pos := r.topInsertPos(sum)
	if pos == -1 {
		return
	}
	parts := make(map[string]string, len(r.slots)+len(r.cond))
	for i := range r.slots {
		parts[r.slots[i].Slot] = r.slots[i].Options[r.path[i]].Name
	}
	for k := range r.cond {
		parts[r.cond[k].Slot] = d.best[k].Name
	}
	r.result.Top = append(r.result.Top, Combo{})
	copy(r.result.Top[pos+1:], r.result.Top[pos:])
	r.result.Top[pos] = Combo{Sum: sum, Parts: parts}
	if len(r.result.Top) > r.topN {
		r.result.Top = r.result.Top[:r.topN]
	}
}

// distKey 分布缓存键：各条件槽当前选项集的内容哈希拼接。
// 静态分支只要对所有条件槽给出相同选项集（换配件名不换选项集）即共享表。
func (r *runner) distKey() string {
	buf := make([]byte, 8*len(r.cond))
	for k, c := range r.cond {
		h := r.hashEmpty
		if c.SlotIdx >= 0 {
			if g, ok := r.groupHash[k][r.path[c.SlotIdx]]; ok {
				h = g
			}
		}
		binary.BigEndian.PutUint64(buf[k*8:], h)
	}
	return string(buf)
}

// buildDist 从末槽向前卷积构建总和分布表，同时产出贪心极值代表。
func (r *runner) buildDist() *condDist {
	m := len(r.cond)
	dist := map[float64]int64{0: 1}
	for k := m - 1; k >= 0; k-- {
		opts := r.condOpts(k)
		next := make(map[float64]int64, len(dist)*len(opts)/2+1)
		for s, c := range dist {
			for _, o := range opts {
				next[s+o.Add] = saturatingAdd(next[s+o.Add], c)
			}
		}
		dist = next
		r.dpWork += len(dist)
		if len(dist) > maxDistSums || r.dpWork > maxDPWork {
			r.abortErr = "条件槽属性取值粒度过细，无法统计组合分布；请换一个约束属性或收窄目标"
			r.aborted = true
			return nil
		}
	}
	d := &condDist{best: make([]Option, m)}
	for s := range dist {
		d.sums = append(d.sums, s)
	}
	sort.Float64s(d.sums)
	d.counts = make([]int64, len(d.sums))
	d.suff = make([]int64, len(d.sums)+1)
	for i, s := range d.sums {
		d.counts[i] = dist[s]
	}
	for i := len(d.sums) - 1; i >= 0; i-- {
		d.suff[i] = saturatingAdd(d.suff[i+1], d.counts[i])
	}
	d.maxSum, d.minSum = d.sums[len(d.sums)-1], d.sums[0]
	// 贪心代表：gte 每槽最大加成 / lte 最小；平手优先实名配件（top 可读性）
	for k := 0; k < m; k++ {
		opts := r.condOpts(k)
		best := opts[0]
		for _, o := range opts[1:] {
			better := false
			if r.op == OpLte {
				better = o.Add < best.Add || (o.Add == best.Add && best.Id == "" && o.Id != "")
			} else {
				better = o.Add > best.Add || (o.Add == best.Add && best.Id == "" && o.Id != "")
			}
			if better {
				best = o
			}
		}
		d.best[k] = best
	}
	return d
}

// hashOptions 选项集内容哈希（Id + Add 都参与，FNV-64）
func hashOptions(opts []Option) uint64 {
	h := fnv.New64a()
	var buf [8]byte
	for _, o := range opts {
		h.Write([]byte(o.Id))
		h.Write([]byte{0})
		binary.LittleEndian.PutUint64(buf[:], math.Float64bits(o.Add))
		h.Write(buf[:])
	}
	return h.Sum64()
}

// saturatingAdd 饱和加法：组合计数可达 int64 上限（10^18 量级），防溢出为负
func saturatingAdd(a, b int64) int64 {
	if b > 0 && a > math.MaxInt64-b {
		return math.MaxInt64
	}
	return a + b
}

// condOpts 条件槽 k 在当前静态选择（提供者）下的选项集
func (r *runner) condOpts(k int) []Option {
	c := r.cond[k]
	if c.SlotIdx >= 0 {
		if opts, ok := c.GroupByOpt[r.path[c.SlotIdx]]; ok {
			return opts
		}
	}
	return emptyOpt
}

// emit 一个完整组合评估完毕：判定约束、计数、置位、维护 top-K。
// 评估计数在 dfs/evalCond 的剪枝检查通过后进行（被剪掉的子树不计入）。
func (r *runner) emit(cur float64) {
	r.result.Evaluated++
	if r.result.Evaluated > maxEval {
		r.aborted = true
		return
	}
	switch r.op {
	case OpGte:
		if cur < r.target-matchEps {
			return
		}
	case OpLte:
		if cur > r.target+matchEps {
			return
		}
	case OpEq:
		if cur < r.target-matchEps || cur > r.target+matchEps {
			return
		}
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
	// 非可选件 = 出厂固定件：统一显示名。此前直接透传"内部配件 130xxxxx"
	// 这类原始 ID 占位名，模型会把它当成可改装配件编造中文名推荐给玩家
	// （实测 M14 后握把：唯一选项是非可选固定件，M14 本就不能换后握把）。
	nameOf := func(id string) string {
		p := st.ds.Parts[id]
		if p == nil {
			return "未知配件 " + id
		}
		if p.Selectable != nil && !*p.Selectable {
			return "出厂固定件（不可更换）"
		}
		return p.Name
	}
	addOf := func(id string) (float64, string) {
		p := st.ds.Parts[id]
		if p == nil {
			return 0, "未知配件 " + id
		}
		if m, ok := p.Modes[mode]; ok && m != nil {
			return m.Add[attr], nameOf(id)
		}
		return 0, nameOf(id)
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
