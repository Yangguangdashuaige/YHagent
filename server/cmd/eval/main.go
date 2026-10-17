// cmd/eval/main.go
//
// 检索质量离线评测工具（只在开发机运行，不部署）：
//
//	go run ./cmd/eval                                          # 纯向量检索（基线，默认）
//	go run ./cmd/eval -mode=bm25                               # 纯 BM25 关键词检索
//	go run ./cmd/eval -mode=hybrid -tag=hybrid                 # 向量+BM25 RRF 混合检索
//	go run ./cmd/eval "-dataset=eval/dataset.coarse.jsonl"     # 文件级对照口径
//	go run ./cmd/eval -tag=hybrid                              # 给本次报告打标签
//
// 两份独立标注集（同一批 query，粒度不同，用 -dataset 切换）：
//   - eval/dataset.fine.jsonl  切片级（默认，主口径）：expected_chunks 填
//     {source, title_contains} 或 {source, title_exact}，必须命中"该文件里
//     含答案的那一段"才算对，反映答案切片召回。
//   - eval/dataset.coarse.jsonl 文件级（对照口径）：expected 填来源文件名，
//     TopK 任意切片来自该文件即算对，反映来源文档召回。
//   - title_exact 用于顶层摘要片（其 Title 只有文档名，与所有小节面包屑同前缀）。
//   - Hit@K：期望目标是否出现在 TopK；MRR@8：首个正确结果排名的倒数；
//   - 结果按总体 / category(物品·公告·机制) / difficulty / qtype 分组；
//   - 报告 JSON 落 eval/reports/，带每条 query 的 TopK 与排名，供 badcase 分析；
//   - query 向量按模型名缓存到 .cache/，迭代检索策略时不重复花 embedding 费用。
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/gob"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"yhagent-server/internal/ai"
	"yhagent-server/internal/config"
	"yhagent-server/internal/knowledge"
)

// Case 标注集一行
type Case struct {
	ID       string `json:"id"`
	Query    string `json:"query"`
	Category string `json:"category"` // item / announce / mechanism
	QType    string `json:"qtype"`    // exact / semantic / alias / temporal
	// Difficulty easy / medium / hard
	Difficulty string `json:"difficulty"`
	// Expected 文件级标注（旧口径）：期望来源文件名，OR
	Expected []string `json:"expected,omitempty"`
	// ExpectedChunks 切片级标注（新口径）：非空时优先于 Expected
	ExpectedChunks []ExpectedChunk `json:"expected_chunks,omitempty"`
	// Gold 标准答案要点，预留给端到端答案评测层，本层检索评测不使用
	Gold string `json:"gold,omitempty"`
	Note string `json:"note,omitempty"`
}

// ExpectedChunk 切片级标注目标。两种标题匹配二选一：
//   - title_contains：面包屑包含该小标题（常规用法）；
//   - title_exact：面包屑与该串全等，用于命中文档顶层摘要片
//     （摘要片 Title 只有文档名，而所有小节面包屑都以文档名为前缀，contains 无法区分）。
type ExpectedChunk struct {
	Source        string `json:"source"`
	TitleContains string `json:"title_contains,omitempty"`
	TitleExact    string `json:"title_exact,omitempty"`
}

// granularity 返回该 case 的判定口径
func (c Case) granularity() string {
	if len(c.ExpectedChunks) > 0 {
		return "chunk"
	}
	return "file"
}

// queryCache query 向量缓存（模型变更后自动作废）
type queryCache struct {
	Model string
	Vecs  map[string][]float32
}

// caseResult 单条评测明细
type caseResult struct {
	ID             string          `json:"id"`
	Query          string          `json:"query"`
	Category       string          `json:"category"`
	QType          string          `json:"qtype"`
	Difficulty     string          `json:"difficulty"`
	Granularity    string          `json:"granularity"`         // file / chunk
	Rank           int             `json:"rank"`                // 首个期望目标的排名(1-based)，0 表示未命中
	HitTitle       string          `json:"hit_title,omitempty"` // 命中切片的面包屑标题（切片级口径）
	TopK           []topEntry      `json:"top_k"`
	Expected       []string        `json:"expected,omitempty"`
	ExpectedChunks []ExpectedChunk `json:"expected_chunks,omitempty"`
}

type topEntry struct {
	Source string  `json:"source"`
	Title  string  `json:"title"`
	Score  float32 `json:"score"`
}

// metrics 一组样本的聚合指标
type metrics struct {
	N      int     `json:"n"`
	HitAt1 float64 `json:"hit@1"`
	HitAt3 float64 `json:"hit@3"`
	HitAt5 float64 `json:"hit@5"`
	HitAt8 float64 `json:"hit@8"`
	MRR8   float64 `json:"mrr@8"`
}

// report 落盘报告
type report struct {
	Tag          string             `json:"tag"`
	Mode         string             `json:"mode"` // vector / bm25 / hybrid
	Time         string             `json:"time"`
	Model        string             `json:"model"`
	Dimension    int                `json:"dimension"`
	ChunkCount   int                `json:"chunk_count"`
	K            int                `json:"k"`
	Overall      map[string]metrics `json:"overall"` // key: all / file / chunk，一眼对比两种口径
	ByCategory   map[string]metrics `json:"by_category"`
	ByDifficulty map[string]metrics `json:"by_difficulty"`
	ByQType      map[string]metrics `json:"by_qtype"`
	Cases        []caseResult       `json:"cases"`
}

func main() {
	dataset := flag.String("dataset", "eval/dataset.fine.jsonl", "标注测试集 JSONL（fine=切片级，coarse=文件级）")
	indexPath := flag.String("index", "internal/knowledge/embedded/index.gob", "向量索引 gob")
	k := flag.Int("k", 8, "评测 TopK（Hit@1/3/5/8 固定计算，此值影响报告中 TopK 明细条数）")
	tag := flag.String("tag", "baseline", "报告标签：baseline / hybrid / rerank …")
	cachePath := flag.String("qcache", ".cache/eval_queries.gob", "query 向量缓存文件")
	mode := flag.String("mode", "vector", "检索模式：vector=纯向量(基线) / bm25=纯关键词 / hybrid=向量+BM25 RRF融合")
	flag.Parse()

	switch *mode {
	case "vector", "bm25", "hybrid":
	default:
		log.Fatalf("未知检索模式 %q（可选 vector / bm25 / hybrid）", *mode)
	}

	if err := config.Load(); err != nil {
		log.Fatalf("配置加载失败: %v", err)
	}
	if *mode != "bm25" { // bm25 纯关键词模式不需要向量化
		if config.AppConfig.DoubaoAPIKey == "" {
			log.Fatal("缺少 ARK_API_KEY，请确认 server/.env 已配置")
		}
	}

	// 1. 加载标注集
	cases, err := loadCases(*dataset)
	if err != nil {
		log.Fatalf("读取标注集失败: %v", err)
	}
	log.Printf("[eval] 标注集 %d 条（%s）", len(cases), *dataset)

	// 2. 加载向量索引
	raw, err := os.ReadFile(*indexPath)
	if err != nil {
		log.Fatalf("读取索引失败: %v（先跑 go run ./cmd/indexer）", err)
	}
	store, err := knowledge.DecodeGOB(raw)
	if err != nil {
		log.Fatalf("解析索引失败: %v", err)
	}
	log.Printf("[eval] 索引加载：%d 切片、%d 维、模型 %s", store.ChunkCount, store.Dimension, store.Model)

	// 3. query 向量化（走缓存，缺失的批量请求）；bm25 纯关键词模式跳过
	cache := loadQueryCache(*cachePath, store.Model)
	if *mode != "bm25" {
		client := ai.NewSeedClient()
		var missing []Case
		for _, c := range cases {
			if _, ok := cache.Vecs[c.Query]; !ok {
				missing = append(missing, c)
			}
		}
		if len(missing) > 0 {
			log.Printf("[eval] %d 条 query 未命中缓存，批量向量化中…", len(missing))
			texts := make([]string, len(missing))
			for i, c := range missing {
				texts[i] = c.Query
			}
			for start := 0; start < len(texts); start += 16 {
				end := min(start+16, len(texts))
				ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
				vecs, err := client.CreateEmbedding(ctx, texts[start:end])
				cancel()
				if err != nil {
					log.Fatalf("query 向量化失败: %v", err)
				}
				for j, v := range vecs {
					cache.Vecs[texts[start+j]] = v
				}
				log.Printf("[eval] 已向量化 %d/%d query", end, len(texts))
			}
			saveQueryCache(*cachePath, cache)
		}
	}

	// 4. 逐条检索 + 判定（三种模式共用同一套判定口径）
	var bmIdx *knowledge.BM25Index
	if *mode == "bm25" {
		bmIdx = knowledge.BuildBM25Index(store.Chunks)
		log.Printf("[eval] BM25 倒排索引构建完成（%d 切片）", store.ChunkCount)
	}
	results := make([]caseResult, 0, len(cases))
	for _, c := range cases {
		var hits []knowledge.Hit
		switch *mode {
		case "bm25":
			hits = bmIdx.Search(c.Query, *k)
		case "hybrid":
			hits = store.SearchHybrid(knowledge.Vec32(cache.Vecs[c.Query]), c.Query, *k)
		default:
			hits = store.Search(knowledge.Vec32(cache.Vecs[c.Query]), *k)
		}
		fileWant := map[string]bool{}
		for _, s := range c.Expected {
			fileWant[s] = true
		}
		rank, hitTitle := 0, ""
		entries := make([]topEntry, 0, len(hits))
		for i, h := range hits {
			entries = append(entries, topEntry{
				Source: h.Chunk.Source,
				Title:  h.Chunk.Title,
				Score:  float32(int(h.Score*1000+0.5)) / 1000,
			})
			if rank == 0 && chunkMatches(c, h.Chunk, fileWant) {
				rank = i + 1
				hitTitle = h.Chunk.Title
			}
		}
		results = append(results, caseResult{
			ID: c.ID, Query: c.Query, Category: c.Category, QType: c.QType,
			Difficulty: c.Difficulty, Granularity: c.granularity(),
			Rank: rank, HitTitle: hitTitle, TopK: entries,
			Expected: c.Expected, ExpectedChunks: c.ExpectedChunks,
		})
	}

	// 5. 汇总 + 落盘 + 打印
	rep := buildReport(*tag, *mode, store, *k, results)
	writeReport(rep)
	printReport(rep)
}

// chunkMatches 是评测判定的唯一口径（单点真相）：
//   - 切片级标注：切片来源文件必须相同，且切片面包屑标题包含指定小标题；
//   - 文件级标注：切片来源文件名命中即可（旧口径，宽松）。
func chunkMatches(c Case, ch knowledge.Chunk, fileWant map[string]bool) bool {
	if len(c.ExpectedChunks) > 0 {
		for _, ec := range c.ExpectedChunks {
			if ch.Source != ec.Source {
				continue
			}
			if ec.TitleExact != "" {
				if ch.Title == ec.TitleExact {
					return true
				}
			} else if strings.Contains(ch.Title, ec.TitleContains) {
				return true
			}
		}
		return false
	}
	return fileWant[ch.Source]
}

func loadCases(path string) ([]Case, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []Case
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 || line[0] == '#' {
			continue
		}
		var c Case
		if err := json.Unmarshal(line, &c); err != nil {
			return nil, fmt.Errorf("第%d行 JSON 错误: %w", lineNo, err)
		}
		if len(c.Expected) == 0 && len(c.ExpectedChunks) == 0 {
			return nil, fmt.Errorf("第%d行 %s 缺少 expected 或 expected_chunks", lineNo, c.ID)
		}
		for i, ec := range c.ExpectedChunks {
			if ec.Source == "" || (ec.TitleContains == "" && ec.TitleExact == "") {
				return nil, fmt.Errorf("第%d行 %s expected_chunks[%d] 缺少 source 或 title_contains/title_exact", lineNo, c.ID, i)
			}
		}
		out = append(out, c)
	}
	return out, sc.Err()
}

func loadQueryCache(path, model string) *queryCache {
	c := &queryCache{Model: model, Vecs: map[string][]float32{}}
	data, err := os.ReadFile(path)
	if err != nil {
		return c
	}
	var saved queryCache
	if err := gob.NewDecoder(bytes.NewReader(data)).Decode(&saved); err != nil || saved.Model != model {
		log.Printf("[eval] query 缓存模型不符或损坏，重新建立")
		return c
	}
	log.Printf("[eval] 载入 %d 条已缓存 query 向量", len(saved.Vecs))
	return &saved
}

func saveQueryCache(path string, c *queryCache) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		log.Printf("[eval] 缓存目录创建失败: %v", err)
		return
	}
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(c); err != nil {
		log.Printf("[eval] 缓存编码失败: %v", err)
		return
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		log.Printf("[eval] 缓存写入失败: %v", err)
	}
}

func buildReport(tag, mode string, store *knowledge.Store, k int, results []caseResult) *report {
	rep := &report{
		Tag: tag, Mode: mode, Time: time.Now().Format("2006-01-02 15:04:05"),
		Model: store.Model, Dimension: store.Dimension, ChunkCount: store.ChunkCount,
		K: k, Cases: results,
		Overall:    map[string]metrics{},
		ByCategory: map[string]metrics{}, ByDifficulty: map[string]metrics{}, ByQType: map[string]metrics{},
	}
	rep.Overall["all"] = aggregate(results, func(c caseResult) bool { return true })
	rep.Overall["file"] = aggregate(results, func(c caseResult) bool { return c.Granularity == "file" })
	rep.Overall["chunk"] = aggregate(results, func(c caseResult) bool { return c.Granularity == "chunk" })
	for _, key := range []string{"item", "announce", "mechanism"} {
		rep.ByCategory[key] = aggregate(results, func(c caseResult) bool { return c.Category == key })
	}
	for _, key := range []string{"easy", "medium", "hard"} {
		rep.ByDifficulty[key] = aggregate(results, func(c caseResult) bool { return c.Difficulty == key })
	}
	qtypes := map[string]bool{}
	for _, r := range results {
		qtypes[r.QType] = true
	}
	for qt := range qtypes {
		rep.ByQType[qt] = aggregate(results, func(c caseResult) bool { return c.QType == qt })
	}
	return rep
}

// aggregate 计算一组样本的 Hit@1/3/5/8 与 MRR@8
func aggregate(results []caseResult, pick func(caseResult) bool) metrics {
	var m metrics
	for _, r := range results {
		if !pick(r) {
			continue
		}
		m.N++
		if r.Rank > 0 && r.Rank <= 1 {
			m.HitAt1++
		}
		if r.Rank > 0 && r.Rank <= 3 {
			m.HitAt3++
		}
		if r.Rank > 0 && r.Rank <= 5 {
			m.HitAt5++
		}
		if r.Rank > 0 && r.Rank <= 8 {
			m.HitAt8++
		}
		if r.Rank > 0 {
			m.MRR8 += 1.0 / float64(r.Rank)
		}
	}
	if m.N == 0 {
		return m
	}
	m.HitAt1 /= float64(m.N)
	m.HitAt3 /= float64(m.N)
	m.HitAt5 /= float64(m.N)
	m.HitAt8 /= float64(m.N)
	m.MRR8 /= float64(m.N)
	return m
}

func writeReport(rep *report) {
	dir := "eval/reports"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Printf("[eval] 报告目录创建失败: %v", err)
		return
	}
	name := fmt.Sprintf("%s-%s.json", rep.Tag, time.Now().Format("20060102-150405"))
	data, _ := json.MarshalIndent(rep, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
		log.Printf("[eval] 报告写入失败: %v", err)
		return
	}
	log.Printf("[eval] 完整报告（含 badcase TopK）：eval/reports/%s", name)
}

func printReport(rep *report) {
	fmt.Println()
	fmt.Printf("================ 检索评测报告 [%s] ================\n", rep.Tag)
	fmt.Printf("时间 %s | 模式 %s | 模型 %s | 索引 %d 片 / %d 维 | 样本 %d\n",
		rep.Time, rep.Mode, rep.Model, rep.ChunkCount, rep.Dimension, rep.Overall["all"].N)
	printMetrics("总体(all)   ", rep.Overall["all"])
	if rep.Overall["chunk"].N > 0 {
		printMetrics("  切片级标注 ", rep.Overall["chunk"])
	}
	if rep.Overall["file"].N > 0 {
		printMetrics("  文件级标注 ", rep.Overall["file"])
	}
	fmt.Println("--- 按类目 ---")
	keys := make([]string, 0, len(rep.ByCategory))
	for k := range rep.ByCategory {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		printMetrics("  "+pad(k, 10), rep.ByCategory[k])
	}
	fmt.Println("--- 按难度 ---")
	for _, k := range []string{"easy", "medium", "hard"} {
		if m, ok := rep.ByDifficulty[k]; ok {
			printMetrics("  "+pad(k, 10), m)
		}
	}
	fmt.Println("--- 按问法 ---")
	keys = keys[:0]
	for k := range rep.ByQType {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		printMetrics("  "+pad(k, 10), rep.ByQType[k])
	}

	// badcase：Top5 未命中的逐条列出，直接指导下一轮优化
	fmt.Println("--- Badcase（Hit@5 未命中 / 排名靠后）---")
	bad := 0
	for _, c := range rep.Cases {
		if c.Rank == 0 || c.Rank > 5 {
			bad++
			top := ""
			for i, t := range c.TopK {
				if i >= 3 {
					break
				}
				top += fmt.Sprintf("  #%d %s | %s(%.3f)\n", i+1, truncate(t.Title, 60), t.Source, t.Score)
			}
			mark := "未命中"
			if c.Rank > 5 {
				mark = fmt.Sprintf("rank=%d", c.Rank)
			}
			fmt.Printf("[%s] %s（%s/%s/%s/%s）[%s]\n期望: %s\nTop3:\n%s",
				c.ID, c.Query, c.Category, c.QType, c.Difficulty, c.Granularity, mark,
				describeExpected(c), top)
		}
	}
	if bad == 0 {
		fmt.Println("无 ✅")
	}
}

func printMetrics(name string, m metrics) {
	fmt.Printf("%s n=%-3d Hit@1=%5.1f%% Hit@3=%5.1f%% Hit@5=%5.1f%% Hit@8=%5.1f%% MRR@8=%.3f\n",
		name, m.N, m.HitAt1*100, m.HitAt3*100, m.HitAt5*100, m.HitAt8*100, m.MRR8)
}

func pad(s string, n int) string {
	for len([]rune(s)) < n {
		s += " "
	}
	return s
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// describeExpected 把一条 case 的期望目标格式化成可读串，用于 badcase 输出
func describeExpected(c caseResult) string {
	if len(c.ExpectedChunks) > 0 {
		parts := make([]string, len(c.ExpectedChunks))
		for i, ec := range c.ExpectedChunks {
			title := ec.TitleContains
			if ec.TitleExact != "" {
				title = "=" + ec.TitleExact
			}
			parts[i] = fmt.Sprintf("%s > 「%s」", ec.Source, title)
		}
		return "切片 " + strings.Join(parts, " 或 ")
	}
	return "文件 " + strings.Join(c.Expected, " 或 ")
}
