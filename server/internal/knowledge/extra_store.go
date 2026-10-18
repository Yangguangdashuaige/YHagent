// internal/knowledge/extra_store.go
//
// 运行时补充知识库：主索引 index.gob 在编译期 go:embed 进二进制、运行时只读，
// 玩家确认采纳的联网知识无法写回主索引。ExtraStore 在主索引之外维护一层
// 可写的补充索引（data/kb-extra/extra.gob + docs/*.md），启动时加载、
// 检索时与主索引合并排序、采纳新文档时切片→向量化→追加→原子落盘。
//
// 规模假设：补充文档几十到几百篇，全量重建索引成本可忽略，故每次入库直接重建
// 整个 extra Store，代码简单且不会产生 ID/向量错位。
package knowledge

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	extraIndexFile = "extra.gob"
	extraDocsDir   = "docs"
	embedBatchSize = 16
)

// Searcher 检索能力（*Store 与 *ExtraStore 均实现）
type Searcher interface {
	Search(query Vec32, k int) []Hit
	// SearchHybrid 向量 + BM25 混合检索；queryText 是生成 query 向量的原始文本
	SearchHybrid(query Vec32, queryText string, k int) []Hit
}

// EmbedProgressFunc 分批向量化进度：done 个切片已完成，共 total 个。
// 由 ingest 包通过 context 注入，用于 SSE 实时入库进度。
type EmbedProgressFunc func(done, total int)

type embedProgressKey struct{}

// WithEmbedProgress 把向量化进度回调挂到 context；f 为 nil 时原样返回
func WithEmbedProgress(ctx context.Context, f EmbedProgressFunc) context.Context {
	if f == nil {
		return ctx
	}
	return context.WithValue(ctx, embedProgressKey{}, f)
}

func emitEmbedProgress(ctx context.Context, done, total int) {
	if f, ok := ctx.Value(embedProgressKey{}).(EmbedProgressFunc); ok && f != nil {
		f(done, total)
	}
}

// ExtraStore 主索引 + 运行时补充索引
type ExtraStore struct {
	mu    sync.RWMutex
	base  *Store // 编译期嵌入的主索引（只读）
	extra *Store // 运行时补充索引（可写）
	dir   string // 补充库目录（extra.gob 与 docs/ 所在）
}

// LoadExtraStore 加载补充库。目录不存在会自动创建；索引文件缺失时以空库启动；
// 索引损坏不阻塞服务（备份为 .corrupt 后从空库开始）。
func LoadExtraStore(base *Store, dir string) (*ExtraStore, error) {
	if base == nil {
		return nil, errors.New("base 主索引不能为 nil")
	}
	if strings.TrimSpace(dir) == "" {
		return nil, errors.New("补充库目录不能为空")
	}
	if err := os.MkdirAll(filepath.Join(dir, extraDocsDir), 0o755); err != nil {
		return nil, fmt.Errorf("创建补充库目录失败: %w", err)
	}
	e := &ExtraStore{base: base, dir: dir, extra: mustEmptyStore(base.Model)}

	indexPath := filepath.Join(dir, extraIndexFile)
	raw, err := os.ReadFile(indexPath)
	if errors.Is(err, fs.ErrNotExist) {
		return e, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取补充索引失败: %w", err)
	}
	st, err := DecodeGOB(raw)
	if err != nil {
		// 损坏文件备份留档，避免反复尝试解析坏文件
		backup := fmt.Sprintf("%s.corrupt-%s", indexPath, time.Now().Format("20060102-150405"))
		_ = os.Rename(indexPath, backup)
		log.Printf("[knowledge] 补充索引损坏，已备份到 %s 并以空库启动: %v", backup, err)
		return e, nil
	}
	if base.Dimension != 0 && st.Dimension != 0 && st.Dimension != base.Dimension {
		return nil, fmt.Errorf("补充索引维度(%d)与主索引(%d)不一致，疑似更换过向量化模型，请清理 %s 后重启",
			st.Dimension, base.Dimension, dir)
	}
	e.extra = st
	log.Printf("[knowledge] 补充知识库已加载：%d 个切片", st.Len())
	return e, nil
}

// Search 合并检索主索引与补充索引，返回全局 TopK（两侧各自已按分数降序）
func (e *ExtraStore) Search(query Vec32, k int) []Hit {
	e.mu.RLock()
	base, extra := e.base, e.extra
	e.mu.RUnlock()

	a := base.Search(query, k)
	b := extra.Search(query, k)
	if len(b) == 0 {
		return a
	}
	merged := mergeHitsByScore(a, b)
	if k > 0 && len(merged) > k {
		merged = merged[:k]
	}
	return merged
}

// mergeHitsByScore 归并两个按分数降序的命中列表（同分时 a 侧优先），结果仍降序
func mergeHitsByScore(a, b []Hit) []Hit {
	merged := make([]Hit, 0, len(a)+len(b))
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		if a[i].Score >= b[j].Score {
			merged = append(merged, a[i])
			i++
		} else {
			merged = append(merged, b[j])
			j++
		}
	}
	merged = append(merged, a[i:]...)
	merged = append(merged, b[j:]...)
	return merged
}

// SearchHybrid 混合检索主索引与补充索引（生产 search_knowledge 走的路径）：
//   - 向量通道：两侧余弦命中按分数归并取全局 Top-Depth（余弦分跨库可比）；
//   - BM25 通道：主/补两侧各取关键词 Top-Depth 后按原始分归并再截断到
//     Top-Depth。两侧语料规模悬殊（4484 vs 几十），IDF 统计不严格可比，
//     但 RRF 只消费名次且权重有门控，跨库分差被强阻尼，不做索引合并。
//
// 补充库是整体替换式重建（AddDocument 换掉整个 *Store），BM25 索引随
// *Store 实例惰性构建、随旧实例一起被垃圾回收，因此无需任何失效逻辑。
func (e *ExtraStore) SearchHybrid(query Vec32, queryText string, k int) []Hit {
	e.mu.RLock()
	base, extra := e.base, e.extra
	e.mu.RUnlock()

	total := base.Len() + extra.Len()
	if total == 0 {
		return nil
	}
	if k <= 0 || k > total {
		k = total
	}

	// 通道一：两侧余弦归并 Top-Depth（复用 Search 的归并逻辑）
	vecCh := e.Search(query, hybridDepth)
	// 通道二：两侧 BM25 子榜单归并后截断 Top-Depth
	bmCh := mergeHitsByScore(
		base.bm25Index().Search(queryText, hybridDepth),
		extra.bm25Index().Search(queryText, hybridDepth))
	if len(bmCh) > hybridDepth {
		bmCh = bmCh[:hybridDepth]
	}
	return fuseRRF([][]Hit{vecCh, bmCh},
		[]float64{1.0, bm25WeightFor(queryText)}, k)
}

// ExtraLen 当前补充切片数（监控/日志用）
func (e *ExtraStore) ExtraLen() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.extra.Len()
}

// AddDocument 采纳一篇 Markdown 文档入库：切片 → 批量向量化 → 重建补充索引 → 落盘。
// 同名 source 重新采纳时整体替换（先删旧片再追加新片）。返回新增切片数。
func (e *ExtraStore) AddDocument(ctx context.Context, source, markdown string, embedder Embedder) (int, error) {
	if strings.TrimSpace(source) == "" {
		return 0, errors.New("source 不能为空")
	}
	if strings.TrimSpace(markdown) == "" {
		return 0, errors.New("文档内容为空")
	}
	if embedder == nil {
		return 0, errors.New("向量化能力未注入")
	}

	// 1. 切片（ID 先占位，重建时统一编号）
	chunks := ChunkMarkdown(source, markdown, 0)
	if len(chunks) == 0 {
		return 0, errors.New("文档切片结果为空，检查 Markdown 是否缺少正文")
	}
	texts := make([]string, len(chunks))
	for i, c := range chunks {
		// 与 indexer 一致：标题加重进向量化输入，保证主/补索引向量分布一致
		texts[i] = EmbedInput(c)
	}

	// 2. 分批向量化（网络调用在锁外执行，避免阻塞检索）
	vectors := make([]Vec32, 0, len(texts))
	for start := 0; start < len(texts); start += embedBatchSize {
		end := start + embedBatchSize
		if end > len(texts) {
			end = len(texts)
		}
		vecs, err := embedder.CreateEmbedding(ctx, texts[start:end])
		if err != nil {
			return 0, fmt.Errorf("入库向量化失败: %w", err)
		}
		if len(vecs) != end-start {
			return 0, fmt.Errorf("入库向量化返回数量异常：期望%d 得到%d", end-start, len(vecs))
		}
		for _, v := range vecs {
			vectors = append(vectors, Vec32(v))
		}
		emitEmbedProgress(ctx, end, len(texts))
	}

	// 3. 加锁：剔除同名旧文档 → 合并 → 重建补充索引
	e.mu.Lock()
	defer e.mu.Unlock()

	keptChunks := make([]Chunk, 0, e.extra.Len()+len(chunks))
	keptVecs := make([]Vec32, 0, cap(keptChunks))
	for i, c := range e.extra.Chunks {
		if c.Source != source {
			keptChunks = append(keptChunks, c)
			keptVecs = append(keptVecs, e.extra.Vectors[i])
		}
	}
	nextID := maxChunkID(e.base) // 补充片 ID 接在主索引之后，保证全局唯一
	for _, c := range keptChunks {
		if c.ID > nextID {
			nextID = c.ID
		}
	}
	for i := range chunks {
		nextID++
		chunks[i].ID = nextID
	}
	keptChunks = append(keptChunks, chunks...)
	keptVecs = append(keptVecs, vectors...)

	store, err := BuildStore(e.base.Model, keptChunks, keptVecs)
	if err != nil {
		return 0, err
	}

	// 4. 持久化：先写临时文件再 rename（同文件系统内原子替换），并保存 md 原文
	if err := e.persistLocked(store, source, markdown); err != nil {
		return 0, err
	}
	e.extra = store
	return len(chunks), nil
}

// persistLocked 调用方已持写锁
func (e *ExtraStore) persistLocked(store *Store, source, markdown string) error {
	gobData, err := store.EncodeGOB()
	if err != nil {
		return err
	}
	tmp := filepath.Join(e.dir, extraIndexFile+".tmp")
	if err := os.WriteFile(tmp, gobData, 0o644); err != nil {
		return fmt.Errorf("写补充索引临时文件失败: %w", err)
	}
	if err := os.Rename(tmp, filepath.Join(e.dir, extraIndexFile)); err != nil {
		return fmt.Errorf("替换补充索引失败: %w", err)
	}

	docPath := filepath.Join(e.dir, extraDocsDir, filepath.Base(source))
	if err := os.WriteFile(docPath, []byte(markdown), 0o644); err != nil {
		return fmt.Errorf("保存文档原文失败: %w", err)
	}
	return nil
}

// mustEmptyStore 构造与主索引同模型名的空补充库
func mustEmptyStore(model string) *Store {
	s, err := BuildStore(model, nil, nil)
	if err != nil {
		// 入参恒为空切片，唯一可能的错误是长度不一致，此处不可能发生
		panic(err)
	}
	return s
}

// maxChunkID 返回库内最大切片 ID，空库返回 0
func maxChunkID(s *Store) int {
	maxID := 0
	for _, c := range s.Chunks {
		if c.ID > maxID {
			maxID = c.ID
		}
	}
	return maxID
}
