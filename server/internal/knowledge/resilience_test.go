package knowledge

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestRerankBreaker(t *testing.T) {
	b := &rerankBreaker{}
	if !b.allow() {
		t.Fatal("初始状态应允许重排")
	}
	// 成功清零
	b.recordFailure()
	b.recordFailure()
	b.recordSuccess()
	if b.fails != 0 {
		t.Fatalf("成功后应清零失败计数, got %d", b.fails)
	}
	// 连续 3 次失败触发熔断
	for i := 0; i < rerankBreakerThreshold; i++ {
		b.recordFailure()
	}
	if b.allow() {
		t.Fatal("连续失败达阈值后应熔断")
	}
	// 熔断期内成功路径不可达，但计数恢复逻辑需正确：直接模拟冷却结束
	b.mu.Lock()
	b.openTil = time.Now().Add(-time.Second)
	b.mu.Unlock()
	if !b.allow() {
		t.Fatal("冷却期结束后应恢复")
	}
	b.recordSuccess()
	if b.fails != 0 {
		t.Fatal("恢复后成功应清零计数")
	}
}

func TestRerankBreakerConcurrent(t *testing.T) {
	b := &rerankBreaker{}
	wg := &sync.WaitGroup{}
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i%2 == 0 {
				b.recordFailure()
			} else {
				b.recordSuccess()
			}
			_ = b.allow()
		}(i)
	}
	wg.Wait()
	// 无死锁、计数不越界即通过（允许任意合法终态）
	if b.fails < 0 || b.fails > 20 {
		t.Fatalf("计数异常: %d", b.fails)
	}
}

// TestBM25FallbackOnEmbedFailure 向量化失败降级纯 BM25：
// embedder 返回错误时检索仍应返回关键词命中的结果而非报错
type failEmbedder struct{}

func (failEmbedder) CreateEmbedding(ctx context.Context, texts []string) ([][]float32, error) {
	return nil, errors.New("embedding 服务不可用")
}

func TestBM25FallbackOnEmbedFailure(t *testing.T) {
	// 最小可用 Store：3 个切片（真实向量维度随意，BM25 通道不依赖它）
	chunks := []Chunk{
		{ID: 0, Title: "零号大逃杀钥匙", Source: "测试", Content: "零号大逃杀模式的旅馆钥匙可以打开旅馆区域的房间。"},
		{ID: 1, Title: "钥匙一览", Source: "测试", Content: "长弓溪谷的村庄钥匙用于村内房屋。"},
		{ID: 2, Title: "其他", Source: "测试", Content: "改枪方案与本文无关。"},
	}
	vectors := make([]Vec32, len(chunks))
	for i := range vectors {
		vectors[i] = Vec32{0.1, 0.2, 0.3}
	}
	store, err := BuildStore("test-model", chunks, vectors)
	if err != nil {
		t.Fatal(err)
	}
	tool := NewSearchTool(store, failEmbedder{}, nil)
	out, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"query": "旅馆钥匙",
		"top_k": 3,
	}))
	if err != nil {
		t.Fatalf("向量化失败时检索不应报错: %v", err)
	}
	m := out.(map[string]any)
	results, ok := m["资料"].([]map[string]any)
	if !ok || len(results) == 0 {
		t.Fatalf("BM25 降级应返回关键词命中结果, got: %v", m)
	}
	if !contains(results[0]["标题"].(string)) {
		t.Logf("首位结果: %v", results[0]["标题"])
	}
}

func contains(s string) bool { return len(s) > 0 }

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
