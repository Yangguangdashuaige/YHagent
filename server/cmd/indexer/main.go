// cmd/indexer/main.go
//
// 静态知识库离线建库工具（只在你自己的开发机上运行，不部署到服务器）：
//
//  1. 读取 data/knowledge/*.md（跳过 README* 和下划线 _ 开头的文件）；
//  2. 按标题/表格/段落切成约600字的知识切片；
//  3. 批量调用豆包向量化接口生成向量；
//  4. 序列化成 index.gob，写入 internal/knowledge/embedded/index.gob，
//     服务端 go build 时通过 go:embed 编入二进制。
//
// 用法（在 server/ 目录下）：
//
//	go run ./cmd/indexer            # 默认增量：只对新增/改动切片重新向量化
//	go run ./cmd/indexer -full      # 全量：忽略旧索引，全部切片重新向量化
//
// 每次增删改 data/knowledge/ 下的文档后，重新跑一次再构建/部署服务端。
// 增量模式以"来源文件名+切片原文"为键复用旧 index.gob 里的向量，
// 只有没命中的切片（新文档、被编辑过的段落）才调用嵌入接口；
// 更换嵌入模型或旧索引缺失/损坏时自动退回全量。
package main

import (
	"context"
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

// embedBatchSize 单次向量化请求的切片数。单片约600字，16片一批约1万 token，
// 远低于向量化模型的 TPM 上限，同时把请求数压到最少。
const embedBatchSize = 16

func main() {
	srcDir := flag.String("src", "data/knowledge", "Markdown 知识文档目录")
	outPath := flag.String("out", "internal/knowledge/embedded/index.gob", "索引输出路径（go:embed 目录）")
	fullRebuild := flag.Bool("full", false, "全量重建：忽略旧索引，重新向量化全部切片（默认增量）")
	flag.Parse()

	log.Println("[indexer] 加载配置...")
	if err := config.Load(); err != nil {
		log.Fatalf("配置加载失败: %v", err)
	}
	if config.AppConfig.DoubaoAPIKey == "" {
		log.Fatal("缺少 ARK_API_KEY，请确认 server/.env 存在且已配置")
	}

	// 1. 收集并排序文档，保证每次建库的切片编号稳定
	docs, err := collectMarkdown(*srcDir)
	if err != nil {
		log.Fatalf("读取知识目录失败: %v", err)
	}
	if len(docs) == 0 {
		log.Printf("[indexer] %s 下没有可用的 .md 文档，生成空索引占位", *srcDir)
	}

	// 2. 切片
	var chunks []knowledge.Chunk
	for _, doc := range docs {
		parts := knowledge.ChunkMarkdown(doc.name, doc.content, len(chunks))
		chunks = append(chunks, parts...)
		log.Printf("[indexer] %-30s -> %d 个切片", doc.name, len(parts))
	}
	log.Printf("[indexer] 共 %d 篇文档、%d 个切片", len(docs), len(chunks))

	// 3. 增量向量化：命中旧索引的切片直接复用原向量，未命中（新增/改动）的才调接口。
	model := config.AppConfig.DoubaoEmbeddingModel
	cache := map[string]knowledge.Vec32{}
	if !*fullRebuild {
		cache = loadVectorCache(*outPath, model)
	}
	vectors := make([]knowledge.Vec32, len(chunks))
	var pendingIdx []int
	var pendingText []string
	for i, c := range chunks {
		if v, ok := cache[c.Source+"\x00"+c.Content]; ok && len(v) > 0 {
			vectors[i] = v
			continue
		}
		pendingIdx = append(pendingIdx, i)
		pendingText = append(pendingText, c.Content)
	}
	log.Printf("[indexer] 共 %d 切片：复用旧向量 %d 片，需新嵌入 %d 片",
		len(chunks), len(chunks)-len(pendingText), len(pendingText))

	// 仅对未命中切片批量调用嵌入接口（空库跳过 API 调用，直接写占位索引）
	client := ai.NewSeedClient()
	for start := 0; start < len(pendingText); start += embedBatchSize {
		end := start + embedBatchSize
		if end > len(pendingText) {
			end = len(pendingText)
		}
		batch := pendingText[start:end]
		vecs, err := embedWithRetry(client, batch)
		if err != nil {
			log.Fatalf("第 %d-%d 片向量化失败: %v\n"+
				"若是 ModelNotOpen 错误，请先到火山方舟控制台开通 %s",
				start+1, end, err, model)
		}
		if len(vecs) != len(batch) {
			log.Fatalf("向量数量不匹配：请求%d条，返回%d条", len(batch), len(vecs))
		}
		for j, v := range vecs {
			vectors[pendingIdx[start+j]] = knowledge.Vec32(v)
		}
		log.Printf("[indexer] 已向量化新增切片 %d/%d 片", end, len(pendingText))
	}
	if cache == nil && len(chunks) > 0 {
		log.Printf("[indexer] 提示：未找到可用旧索引（或加了 -full），本次为全量嵌入")
	}

	// 4. 构建存储并写出 gob
	store, err := knowledge.BuildStore(config.AppConfig.DoubaoEmbeddingModel, chunks, vectors)
	if err != nil {
		log.Fatalf("构建索引失败: %v", err)
	}
	data, err := store.EncodeGOB()
	if err != nil {
		log.Fatalf("序列化失败: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(*outPath), 0o755); err != nil {
		log.Fatalf("创建输出目录失败: %v", err)
	}
	if err := os.WriteFile(*outPath, data, 0o644); err != nil {
		log.Fatalf("写入索引文件失败: %v", err)
	}
	log.Printf("[indexer] 完成：%s（%d 切片，%d 维，%.1f KB）",
		*outPath, store.Len(), store.Dimension, float64(len(data))/1024)
}

type doc struct {
	name    string
	content string
}

// collectMarkdown 收集目录下的 .md 文件，跳过 README* 与 _ 开头（写作指南等非知识文件）
func collectMarkdown(dir string) ([]doc, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		if !strings.HasSuffix(strings.ToLower(n), ".md") {
			continue
		}
		base := strings.TrimSuffix(n, filepath.Ext(n))
		if strings.HasPrefix(base, "README") || strings.HasPrefix(base, "_") {
			continue
		}
		names = append(names, n)
	}
	sort.Strings(names)

	docs := make([]doc, 0, len(names))
	for _, n := range names {
		b, err := os.ReadFile(filepath.Join(dir, n))
		if err != nil {
			return nil, fmt.Errorf("读取 %s: %w", n, err)
		}
		docs = append(docs, doc{name: n, content: string(b)})
	}
	return docs, nil
}

// loadVectorCache 读取旧索引，返回 "来源文件名\x00切片原文" -> 已归一化向量 的映射。
// 旧文件不存在、解析失败或嵌入模型已更换时返回 nil（调用方据此走全量嵌入）。
func loadVectorCache(path, model string) map[string]knowledge.Vec32 {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("[indexer] 未发现旧索引 %s，将全量嵌入", path)
		return nil
	}
	store, err := knowledge.DecodeGOB(data)
	if err != nil {
		log.Printf("[indexer] 旧索引解析失败（%v），将全量嵌入", err)
		return nil
	}
	if store.Model != model {
		log.Printf("[indexer] 嵌入模型已变更（%s -> %s），旧向量不可复用，全量嵌入",
			store.Model, model)
		return nil
	}
	m := make(map[string]knowledge.Vec32, len(store.Chunks))
	for i, c := range store.Chunks {
		m[c.Source+"\x00"+c.Content] = store.Vectors[i]
	}
	log.Printf("[indexer] 已加载旧索引：%d 切片、%d 维，增量模式（-full 可强制全量）",
		len(store.Chunks), store.Dimension)
	return m
}

// embedWithRetry 单批向量化，失败退避重试一次（应对偶发网络抖动/限流）
func embedWithRetry(client *ai.SeedClient, batch []string) (vecs [][]float32, err error) {
	for attempt := 1; attempt <= 2; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		vecs, err = client.CreateEmbedding(ctx, batch)
		cancel()
		if err == nil {
			return vecs, nil
		}
		if attempt == 1 {
			log.Printf("[indexer] 向量化失败（%v），3秒后重试...", err)
			time.Sleep(3 * time.Second)
		}
	}
	return nil, err
}
