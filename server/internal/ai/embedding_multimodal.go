// internal/ai/embedding_multimodal.go
//
// 多模态向量化端点 /embeddings/multimodal 的纯标准库实现。
// 适用于 doubao-embedding-vision-* 接入点（ep- 开头）：
// 该模型只能走多模态端点，一次请求把「一组图文混合内容」编码成一个向量；
// 纯文本知识库使用时，每条文本单独请求即可。
// 优点：文本与图片处于同一向量空间，未来可直接支持「截图搜攻略」等跨模态检索，
// 建库与线上查询必须使用同一接入点。
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"yhagent-server/internal/config"
)

const (
	// multimodalPath 多模态向量化端点路径（拼在 ARK_BASE_URL 之后）
	multimodalPath = "/embeddings/multimodal"

	// mmEmbedConcurrency 批量建库时的并发请求数。
	// 多模态端点一次请求只出一个向量，批量 N 条文本 = N 个请求，用有界并发提速。
	mmEmbedConcurrency = 4
	// mmEmbedTimeout 单次向量化请求超时
	mmEmbedTimeout = 30 * time.Second

	// mmTextInstruction 纯文本检索场景的指令（建库与查询保持一致）。
	// 官方要求显式配置 instructions，Target_modality 声明目标模态、Instruction 引导编码目标。
	mmTextInstruction = "Target_modality: text.\n" +
		"Instruction: Represent the Chinese game guide text for semantic retrieval; " +
		"preserve weapon names, game terminology and gameplay meaning.\nQuery:"
)

// mmRequest 多模态向量化请求体（仅用到文本部分）
type mmRequest struct {
	Model          string            `json:"model"`
	Instructions   string            `json:"instructions"`
	EncodingFormat string            `json:"encoding_format"`
	Input          []mmTextInputPart `json:"input"`
}

type mmTextInputPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// mmResponse 多模态向量化响应：data 是单个对象（与标准文本端点的数组不同）
type mmResponse struct {
	Data struct {
		Embedding []float32 `json:"embedding"`
		Object    string    `json:"object"`
	} `json:"data"`
	Model string `json:"model"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// createMultimodalEmbeddings 批量文本向量化：有界并发，返回顺序与入参一致。
func createMultimodalEmbeddings(ctx context.Context, httpClient *http.Client, texts []string) ([][]float32, error) {
	results := make([][]float32, len(texts))
	errs := make([]error, len(texts))

	sem := make(chan struct{}, mmEmbedConcurrency)
	var wg sync.WaitGroup
	for i, t := range texts {
		wg.Add(1)
		go func(idx int, text string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			vec, err := embedOneMultimodalText(ctx, httpClient, text)
			results[idx] = vec
			errs[idx] = err
		}(i, t)
	}
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	return results, nil
}

// embedOneMultimodalText 调用一次多模态端点，把单条文本编码为向量
func embedOneMultimodalText(parent context.Context, httpClient *http.Client, text string) ([]float32, error) {
	cfg := config.AppConfig
	reqBody := mmRequest{
		Model:          cfg.DoubaoEmbeddingModel,
		Instructions:   mmTextInstruction,
		EncodingFormat: "float",
		Input:          []mmTextInputPart{{Type: "text", Text: text}},
	}
	raw, err := json.Marshal(&reqBody)
	if err != nil {
		return nil, fmt.Errorf("编码向量化请求失败: %w", err)
	}

	ctx, cancel := context.WithTimeout(parent, mmEmbedTimeout)
	defer cancel()

	url := strings.TrimRight(cfg.DoubaoBaseURL, "/") + multimodalPath
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("构造多模态向量化请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.DoubaoAPIKey)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求多模态向量化接口失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20)) // 单向量响应上限 32MB
	if err != nil {
		return nil, fmt.Errorf("读取向量化响应失败: %w", err)
	}

	var out mmResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("解析向量化响应失败(HTTP %d): %w; body: %.200s",
			resp.StatusCode, err, string(body))
	}
	if resp.StatusCode != http.StatusOK {
		if out.Error != nil {
			return nil, fmt.Errorf("多模态向量化接口返回 %d: %s (%s)",
				resp.StatusCode, out.Error.Message, out.Error.Code)
		}
		return nil, fmt.Errorf("多模态向量化接口返回 HTTP %d: %.200s", resp.StatusCode, string(body))
	}
	if len(out.Data.Embedding) == 0 {
		return nil, fmt.Errorf("多模态向量化响应为空向量")
	}
	return out.Data.Embedding, nil
}
