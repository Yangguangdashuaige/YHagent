// internal/knowledge/store.go
//
// 知识库向量存储：进程内存中的"切片原文 + 归一化向量"，
// 检索时做点积（等价余弦相似度）取 TopK。
// 游戏攻略量级（几百到几千切片）下纯内存扫描毫秒级，无需外部向量数据库。
// 建库产物用 encoding/gob 序列化成单个文件，由 cmd/indexer 离线生成、
// 通过 go:embed 编进服务端二进制。
package knowledge

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"unicode"
)

// EmbedInputVersion 向量化输入格式版本。输入格式变更（如标题加重）后
// 旧索引里的向量与新格式分布不一致，必须全量重嵌；indexer 写入当前版本、
// 加载时校验不符即放弃复用。旧索引该字段为空串，视为不可复用。
const EmbedInputVersion = "titleleaf2x-20261018"

// Store 是建库产物的完整结构（gob 可序列化）
type Store struct {
	Model      string  // 使用的向量化模型名（校验查询向量维度用）
	Dimension  int     // 向量维度
	ChunkCount int     // 切片数（冗余字段，加载后快速判空）
	Chunks     []Chunk // 切片原文，下标与 Vectors 一一对应
	Vectors    []Vec32 // 已 L2 归一化的向量
	EmbedVer   string  // 向量化输入格式版本（见 EmbedInputVersion；空=旧格式）

	// BM25 倒排索引：首次混合检索时惰性构建
	// （未导出字段不参与 gob 序列化，不影响既有索引文件兼容性）
	bm25Once sync.Once
	bm25Idx  *BM25Index
}

// Vec32 稠密向量
type Vec32 []float32

// Hit 检索命中结果
type Hit struct {
	Chunk Chunk
	Score float32
}

// Len 返回切片数量
func (s *Store) Len() int {
	if s == nil {
		return 0
	}
	return len(s.Chunks)
}

// Search 用一条查询向量检索最相似的 k 个切片
func (s *Store) Search(query Vec32, k int) []Hit {
	n := len(s.Chunks)
	if n == 0 || len(query) == 0 {
		return nil
	}
	if k <= 0 || k > n {
		k = n
	}
	q := normalize(query)

	hits := make([]Hit, n)
	for i, v := range s.Vectors {
		hits[i] = Hit{Chunk: s.Chunks[i], Score: dot(q, v)}
	}
	// 全量排序在几千切片内成本可忽略；n 很大时可改堆排序，当前没必要
	sort.Slice(hits, func(i, j int) bool { return hits[i].Score > hits[j].Score })
	if k < len(hits) {
		hits = hits[:k]
	}
	return hits
}

// ---- 混合检索：向量 + BM25 双通道 RRF 融合 ----

const (
	hybridDepth = 50   // 每个通道进入融合的排名深度
	rrfK        = 60.0 // RRF 平滑常数（文献通用经验值）
	// BM25 通道权重按查询类型自适应（在 100 条标注集上网格搜索调出）：
	//   - 查询含数字（日期/编号/弹药规格等精确词元）时 BM25 命中极准
	//     （date_lookup 0%→80%、multi_hop 50%→100%），等权融合；
	//   - 纯中文语义查询（口语转述/同义改写）BM25 噪声大
	//     （colloquial 76.9%→53.8%），降权到 0.15 只做小幅纠偏。
	bmWeightNormal = 0.15
	bmWeightExact  = 1.0
)

// bm25Index 惰性构建 BM25 倒排索引（首次调用时建一次，之后复用）
func (s *Store) bm25Index() *BM25Index {
	s.bm25Once.Do(func() {
		s.bm25Idx = BuildBM25Index(s.Chunks)
	})
	return s.bm25Idx
}

// bm25WeightFor BM25 通道权重门控（规则见常量注释）：
// 查询含数字（含全角，日期/编号/弹药规格等精确词元）走等权，纯中文语义查询降权
func bm25WeightFor(queryText string) float64 {
	if strings.ContainsFunc(queryText, unicode.IsDigit) {
		return bmWeightExact
	}
	return bmWeightNormal
}

// fuseRRF 多通道 RRF 融合：channels[i] 是第 i 通道按相关度降序的命中列表，
// weights[i] 为其权重，score(d)=Σ wᵢ/(rrfK+rankᵢ(d))，融合后取 TopK。
// 跨通道的同一切片以 Chunk.ID 归并累计（主索引全局连续编号、补充索引
// 接续主索引最大 ID 编号，保证键全局唯一）；同分按 ID 升序保证结果确定。
func fuseRRF(channels [][]Hit, weights []float64, k int) []Hit {
	type acc struct {
		chunk Chunk
		score float64
	}
	m := make(map[int]*acc, 128)
	for ci, ch := range channels {
		w := weights[ci]
		for r, h := range ch {
			a := m[h.Chunk.ID]
			if a == nil {
				a = &acc{chunk: h.Chunk}
				m[h.Chunk.ID] = a
			}
			a.score += w / (rrfK + float64(r+1))
		}
	}
	list := make([]acc, 0, len(m))
	for _, a := range m {
		list = append(list, *a)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].score != list[j].score {
			return list[i].score > list[j].score
		}
		return list[i].chunk.ID < list[j].chunk.ID
	})
	if k > 0 && len(list) > k {
		list = list[:k]
	}
	hits := make([]Hit, len(list))
	for i, a := range list {
		hits[i] = Hit{Chunk: a.chunk, Score: float32(a.score)}
	}
	return hits
}

// SearchHybrid 向量 + BM25 混合检索：两通道各取前 hybridDepth 名，
// 按 RRF（Reciprocal Rank Fusion，k=60）加权融合，再取 TopK。
// RRF 只看排名不看原始分值，规避了余弦分值区间窄、
// 逐查询漂移的问题（正确与错误切片的余弦差常常只有 0.01 量级）。
// queryText 是生成 query 向量的原始文本，供 BM25 通道分词与权重门控；
// 纯向量 Search 保持不变，本方法不影响现有调用方。
func (s *Store) SearchHybrid(query Vec32, queryText string, k int) []Hit {
	n := len(s.Chunks)
	if n == 0 {
		return nil
	}
	if k <= 0 || k > n {
		k = n
	}
	// 通道一：向量余弦 Top-Depth（query 为空时返回 nil，通道自然缺席）
	vecCh := s.Search(query, hybridDepth)
	// 通道二：BM25 关键词 Top-Depth（无词元命中时为 nil）
	bmCh := s.bm25Index().Search(queryText, hybridDepth)
	return fuseRRF([][]Hit{vecCh, bmCh},
		[]float64{1.0, bm25WeightFor(queryText)}, k)
}

// BuildStore 由切片与对应向量构建存储（向量会被归一化），并做一致性校验
func BuildStore(model string, chunks []Chunk, vectors []Vec32) (*Store, error) {
	if len(chunks) != len(vectors) {
		return nil, fmt.Errorf("切片数(%d)与向量数(%d)不一致", len(chunks), len(vectors))
	}
	dim := 0
	normalized := make([]Vec32, len(vectors))
	for i, v := range vectors {
		if dim == 0 {
			dim = len(v)
			if dim == 0 {
				return nil, fmt.Errorf("第%d个向量维度为0", i)
			}
		}
		if len(v) != dim {
			return nil, fmt.Errorf("第%d个向量维度=%d，与首个向量维度%d不一致", i, len(v), dim)
		}
		normalized[i] = normalize(v)
	}
	if dim == 0 {
		dim = 0 // 允许空库占位
	}
	return &Store{
		Model:      model,
		Dimension:  dim,
		ChunkCount: len(chunks),
		Chunks:     chunks,
		Vectors:    normalized,
	}, nil
}

// EncodeGOB 序列化为 gob 字节
func (s *Store) EncodeGOB() ([]byte, error) {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(s); err != nil {
		return nil, fmt.Errorf("知识库序列化失败: %w", err)
	}
	return buf.Bytes(), nil
}

// DecodeGOB 从 gob 字节加载知识库
func DecodeGOB(data []byte) (*Store, error) {
	var s Store
	if err := gob.NewDecoder(bytes.NewReader(data)).Decode(&s); err != nil {
		return nil, fmt.Errorf("知识库索引文件解析失败: %w", err)
	}
	if s.ChunkCount != len(s.Chunks) {
		return nil, fmt.Errorf("知识库索引计数异常(记录%d/实际%d)，请重新建库", s.ChunkCount, len(s.Chunks))
	}
	return &s, nil
}

// normalize 返回 L2 归一化后的新向量；零向量原样返回
func normalize(v Vec32) Vec32 {
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	if sum == 0 {
		out := make(Vec32, len(v))
		copy(out, v)
		return out
	}
	norm := float32(math.Sqrt(sum))
	out := make(Vec32, len(v))
	for i, x := range v {
		out[i] = x / norm
	}
	return out
}

// dot 点积。向量均已归一化，点积即余弦相似度
func dot(a, b Vec32) float32 {
	var sum float32
	for i := range a {
		sum += a[i] * b[i]
	}
	return sum
}
