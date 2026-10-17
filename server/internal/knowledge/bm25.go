// internal/knowledge/bm25.go
//
// BM25 关键词倒排索引（零三方依赖），与向量检索互补：
//   - 精确词元（日期"4月16日"、枪械代号 K416、弹药规格 7.62x51mm、干员代号等）
//     在稠密向量空间里被语义稀释，而在词元空间里可以精确命中；
//   - 标题（面包屑，含文档名/日期/小节名）权重 3 倍于正文，缓解同文档内
//     小节排序错误——答案所在小节的标题包含查询关键词时应排到前面。
//
// 分词规则（查询与文档同一套，保证词元空间一致）：
//   - 连续汉字 → 二元组（"代号旅人"→代号/号旅/旅人），孤立单字单独成词；
//   - 连续 ASCII 字母/数字及内部的 .-+ → 整词（"7.62x51mm"、"K416"、
//     "DVC12+P" 不被拆散），词元边缘的 .-+ 剥掉（句尾句点等）；
//   - 其余字符（空格、中英文标点、全角符号）均为分隔符。
//
// 打分：score = 3.0×BM25(标题字段) + 1.0×BM25(正文字段)，
// k1=1.2、b=0.75，IDF = ln(1 + (N-df+0.5)/(df+0.5))。
package knowledge

import (
	"math"
	"sort"
	"strings"
	"unicode"
)

// BM25 参数与字段权重
const (
	bm25K1     = 1.2  // 词频饱和度
	bm25B      = 0.75 // 文档长度归一化强度
	bm25TitleW = 3.0  // 标题字段权重（面包屑含日期/代号/小节名，信息密度高）
	bm25BodyW  = 1.0  // 正文字段权重
)

// posting 一个词元在某文档中的词频（标题/正文分别记录，int32 控制内存）
type posting struct {
	docID   int32
	tfTitle int32
	tfBody  int32
}

// BM25Index 进程内倒排索引，由 Store 在首次混合检索时惰性构建
type BM25Index struct {
	chunks   []Chunk
	postings map[string][]posting
	df       map[string]int // 词元出现在多少个文档
	titleLen []int32        // 每文档标题词元数
	bodyLen  []int32        // 每文档正文词元数
	avgTitle float64
	avgBody  float64
}

// bmRank BM25 排名条目
type bmRank struct {
	docID int
	score float64
}

// BuildBM25Index 为一批切片构建 BM25 索引。chunks 与 Store.Chunks 共享底层数组，
// 构建后切片内容不可变（当前主索引加载后只读，满足该约束）。
func BuildBM25Index(chunks []Chunk) *BM25Index {
	ix := &BM25Index{
		chunks:   chunks,
		postings: make(map[string][]posting),
		df:       make(map[string]int),
		titleLen: make([]int32, len(chunks)),
		bodyLen:  make([]int32, len(chunks)),
	}
	var sumTitle, sumBody int
	for id, ch := range chunks {
		tt := tokenizeBM25(ch.Title)
		bt := tokenizeBM25(ch.Content)
		ix.titleLen[id], ix.bodyLen[id] = int32(len(tt)), int32(len(bt))
		sumTitle += len(tt)
		sumBody += len(bt)

		tfT := make(map[string]int32, len(tt))
		for _, t := range tt {
			tfT[t]++
		}
		tfB := make(map[string]int32, len(bt))
		for _, t := range bt {
			tfB[t]++
		}
		// 标题词与正文词取并集建倒排，一个文档对每个词元只有一条 posting
		for t, n := range tfT {
			ix.postings[t] = append(ix.postings[t], posting{int32(id), n, tfB[t]})
			ix.df[t]++
		}
		for t, n := range tfB {
			if _, ok := tfT[t]; ok {
				continue
			}
			ix.postings[t] = append(ix.postings[t], posting{int32(id), 0, n})
			ix.df[t]++
		}
	}
	if n := len(chunks); n > 0 {
		ix.avgTitle = float64(sumTitle) / float64(n)
		ix.avgBody = float64(sumBody) / float64(n)
	}
	return ix
}

// rank 返回 BM25 得分最高的 depth 个文档（降序；同分按 docID 升序保证确定性）。
// depth<=0 表示不截断。
func (ix *BM25Index) rank(query string, depth int) []bmRank {
	toks := tokenizeBM25(query)
	n := len(ix.chunks)
	if len(toks) == 0 || n == 0 {
		return nil
	}
	scores := make([]float64, n)
	for _, t := range toks {
		pl, ok := ix.postings[t]
		if !ok {
			continue
		}
		idf := math.Log(1 + (float64(n)-float64(ix.df[t])+0.5)/(float64(ix.df[t])+0.5))
		for _, p := range pl {
			if p.tfTitle > 0 && ix.avgTitle > 0 {
				scores[p.docID] += bm25TitleW * idf * bm25TF(int(p.tfTitle), int(ix.titleLen[p.docID]), ix.avgTitle)
			}
			if p.tfBody > 0 && ix.avgBody > 0 {
				scores[p.docID] += bm25BodyW * idf * bm25TF(int(p.tfBody), int(ix.bodyLen[p.docID]), ix.avgBody)
			}
		}
	}
	out := make([]bmRank, 0, 64)
	for id, sc := range scores {
		if sc > 0 {
			out = append(out, bmRank{id, sc})
		}
	}
	sort.Slice(out, func(a, b int) bool {
		if out[a].score != out[b].score {
			return out[a].score > out[b].score
		}
		return out[a].docID < out[b].docID
	})
	if depth > 0 && depth < len(out) {
		out = out[:depth]
	}
	return out
}

// Search BM25 关键词检索 TopK（供评测 bm25 模式与离线排查使用）
func (ix *BM25Index) Search(query string, k int) []Hit {
	n := len(ix.chunks)
	if n == 0 {
		return nil
	}
	if k <= 0 || k > n {
		k = n
	}
	ranks := ix.rank(query, k)
	hits := make([]Hit, 0, len(ranks))
	for _, r := range ranks {
		hits = append(hits, Hit{Chunk: ix.chunks[r.docID], Score: float32(r.score)})
	}
	return hits
}

// bm25TF BM25 词频项：tf 饱和增长 + 文档长度归一化
func bm25TF(tf, dl int, avgdl float64) float64 {
	t := float64(tf)
	return t * (bm25K1 + 1) / (t + bm25K1*(1-bm25B+bm25B*float64(dl)/avgdl))
}

// tokenizeBM25 查询/文档统一分词器（规则见文件头注释）
func tokenizeBM25(s string) []string {
	runes := []rune(strings.ToLower(s))
	var toks []string
	i := 0
	for i < len(runes) {
		r := runes[i]
		switch {
		case unicode.Is(unicode.Han, r):
			j := i
			for j < len(runes) && unicode.Is(unicode.Han, runes[j]) {
				j++
			}
			run := runes[i:j]
			if len(run) == 1 {
				toks = append(toks, string(run))
			} else {
				for x := 0; x+1 < len(run); x++ {
					toks = append(toks, string(run[x:x+2]))
				}
			}
			i = j
		case isBM25WordChar(r):
			j := i
			for j < len(runes) && isBM25WordChar(runes[j]) {
				j++
			}
			tok := strings.Trim(string(runes[i:j]), ".-+")
			if tok != "" {
				toks = append(toks, tok)
			}
			i = j
		default:
			i++
		}
	}
	return toks
}

// isBM25WordChar ASCII 字母/数字及内部连接符（.-+），如 7.62x51mm、DVC12+P。
// 注意只认 ASCII：汉字也是 unicode 字母，若用 IsLetter 会把"8月14日的…"
// 这类"数字开头接汉字"的整串吞成一个巨型词元，导致检索落空。
func isBM25WordChar(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
		r == '.' || r == '-' || r == '+'
}
