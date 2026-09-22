// cmd/indexer/main.go
//
// 静态知识库离线建库工具（只在你自己的开发机上运行，不部署到服务器）：
//
//  1. 读取 knowledge/*.md（跳过 README* 和下划线 _ 开头的文件）；
//  2. 按标题/表格/段落切成约600字的知识切片；
//  3. 批量调用豆包向量化接口生成向量；
//  4. 序列化成 index.gob，写入 internal/knowledge/embedded/index.gob，
//     服务端 go build 时通过 go:embed 编入二进制。
//
// 用法（在 server/ 目录下）：
//
//	go run ./cmd/indexer
//
// 每次增删改 knowledge/ 下的文档后，重新跑一次再构建/部署服务端。
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
	srcDir := flag.String("src", "knowledge", "Markdown 知识文档目录")
	outPath := flag.String("out", "internal/knowledge/embedded/index.gob", "索引输出路径（go:embed 目录）")
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

	// 3. 批量向量化（空库跳过 API 调用，直接写占位索引）
	client := ai.NewSeedClient()
	vectors := make([]knowledge.Vec32, 0, len(chunks))
	if len(chunks) > 0 {
		texts := make([]string, len(chunks))
		for i, c := range chunks {
			texts[i] = c.Content
		}
		for start := 0; start < len(texts); start += embedBatchSize {
			end := start + embedBatchSize
			if end > len(texts) {
				end = len(texts)
			}
			batch := texts[start:end]
			vecs, err := embedWithRetry(client, batch)
			if err != nil {
				log.Fatalf("第 %d-%d 片向量化失败: %v\n"+
					"若是 ModelNotOpen 错误，请先到火山方舟控制台开通 %s",
					start+1, end, err, config.AppConfig.DoubaoEmbeddingModel)
			}
			if len(vecs) != len(batch) {
				log.Fatalf("向量数量不匹配：请求%d条，返回%d条", len(batch), len(vecs))
			}
			for _, v := range vecs {
				vectors = append(vectors, knowledge.Vec32(v))
			}
			log.Printf("[indexer] 已向量化 %d/%d 片", end, len(texts))
		}
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
