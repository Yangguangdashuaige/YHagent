// internal/knowledge/extra_store_test.go
//
// ExtraStore 混合检索（生产 search_knowledge 实际路径）的单元测试：
//   - 补充切片经 AddDocument 入库后能被 SearchHybrid 召回（BM25 通道生效）；
//   - 空补充库时退化为纯主索引行为；
//   - 结果确定性（同输入两次调用排序一致）。
package knowledge

import (
	"context"
	"testing"
)

// 编译期断言两个实现均满足 Searcher 接口
var (
	_ Searcher = (*Store)(nil)
	_ Searcher = (*ExtraStore)(nil)
)

// fakeEmbedder 按文本生成确定性向量的假向量化器（测试用）
type fakeEmbedder struct{ dim int }

func (f fakeEmbedder) CreateEmbedding(ctx context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, t := range texts {
		v := make([]float32, f.dim)
		for j, r := range []rune(t) {
			if j >= f.dim {
				break
			}
			v[j] = float32(r%7+1) / 7 // 确定性非零向量，不同文本方向不同
		}
		out[i] = v
	}
	return out, nil
}

func testBaseStore(t *testing.T) *Store {
	t.Helper()
	s, err := BuildStore("test-model", []Chunk{
		{ID: 1, Source: "压枪.md", Title: "压枪技巧 > M14", Content: "【压枪技巧 > M14】M14 后坐力大，压枪需要匀速下拉鼠标。"},
		{ID: 2, Source: "配件.md", Title: "配件 > 握把", Content: "【配件 > 握把】垂直握把可降低垂直后坐力。"},
	}, []Vec32{
		{1, 0, 0, 0, 0, 0, 0, 0},
		{0, 1, 0, 0, 0, 0, 0, 0},
	})
	if err != nil {
		t.Fatalf("构建主索引失败: %v", err)
	}
	return s
}

func TestExtraStoreSearchHybrid(t *testing.T) {
	base := testBaseStore(t)
	e, err := LoadExtraStore(base, t.TempDir())
	if err != nil {
		t.Fatalf("加载补充库失败: %v", err)
	}

	// 入库一篇含精确词元 K416 的补充文档
	n, err := e.AddDocument(context.Background(), "联网-K416攻略.md",
		"# K416 攻略\n\n## K416 压枪\n\nK416 后坐力低，适合新手压枪。\n",
		fakeEmbedder{8})
	if err != nil || n == 0 {
		t.Fatalf("AddDocument 失败: n=%d err=%v", n, err)
	}

	// 数字查询：K416 是补充文档专属词元（BM25 通道应把它拉进前排），
	// 同时主索引的压枪切片靠向量通道也在榜上
	query := Vec32{1, 0, 0, 0, 0, 0, 0, 0}
	hits := e.SearchHybrid(query, "K416 压枪技巧", 5)
	if len(hits) == 0 {
		t.Fatal("混合检索无结果")
	}
	var sawBase, sawExtra bool
	for _, h := range hits {
		switch h.Chunk.Source {
		case "压枪.md":
			sawBase = true
		case "联网-K416攻略.md":
			sawExtra = true
		}
	}
	if !sawExtra {
		t.Errorf("补充库切片未被混合检索召回: %+v", hits)
	}
	if !sawBase {
		t.Errorf("主索引切片未被召回: %+v", hits)
	}

	// 确定性：同输入两次调用结果一致
	hits2 := e.SearchHybrid(query, "K416 压枪技巧", 5)
	if len(hits) != len(hits2) {
		t.Fatalf("两次调用结果长度不一致: %d vs %d", len(hits), len(hits2))
	}
	for i := range hits {
		if hits[i].Chunk.ID != hits2[i].Chunk.ID {
			t.Fatalf("两次调用第%d名不一致: %d vs %d", i+1, hits[i].Chunk.ID, hits2[i].Chunk.ID)
		}
	}

	// 纯中文语义查询（无数字，BM25 降权路径）仍能召回
	hits = e.SearchHybrid(query, "压枪怎么练", 5)
	if len(hits) == 0 {
		t.Fatal("纯语义查询无结果")
	}
}

func TestExtraStoreSearchHybridEmptyExtra(t *testing.T) {
	base := testBaseStore(t)
	e, err := LoadExtraStore(base, t.TempDir())
	if err != nil {
		t.Fatalf("加载补充库失败: %v", err)
	}

	// 空补充库：退化为纯主索引行为，仍能召回
	hits := e.SearchHybrid(Vec32{1, 0, 0, 0, 0, 0, 0, 0}, "压枪技巧", 5)
	if len(hits) == 0 || hits[0].Chunk.Source != "压枪.md" {
		t.Fatalf("空补充库下混合检索异常: %+v", hits)
	}
}
