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
)

// Store 是建库产物的完整结构（gob 可序列化）
type Store struct {
	Model      string  // 使用的向量化模型名（校验查询向量维度用）
	Dimension  int     // 向量维度
	ChunkCount int     // 切片数（冗余字段，加载后快速判空）
	Chunks     []Chunk // 切片原文，下标与 Vectors 一一对应
	Vectors    []Vec32 // 已 L2 归一化的向量
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
