// internal/knowledge/tool.go
//
// 把静态知识库包装成 Agent 的第 24 个工具 search_knowledge。
// 模型自行判断"攻略/技巧/机制/配装/任务流程"类问题时调用本工具；
// 实时价格、库存、密码等动态数据仍走 delta_* 工具。
package knowledge

import (
	"context"
	"encoding/json"
	"fmt"

	"yhagent-server/internal/ai"
)

// Embedder 文本向量化能力（由 ai.SeedClient 实现，避免本包直接依赖豆包 SDK）
type Embedder interface {
	CreateEmbedding(ctx context.Context, texts []string) ([][]float32, error)
}

const (
	defaultTopK = 5
	maxTopK     = 8
	// 单条检索结果回传给模型的正文长度上限（切片本身约600字，这里再兜一道）
	maxResultRunes = 900
)

// RegisterTool 向工具注册中心注册知识库检索工具
func RegisterTool(reg *ai.Registry, store *Store, embedder Embedder) {
	reg.Register(NewSearchTool(store, embedder))
}

// NewSearchTool 构造 search_knowledge 函数工具
func NewSearchTool(store *Store, embedder Embedder) *ai.FunctionTool {
	return ai.NewFunctionTool(
		"search_knowledge",
		"搜索《三角洲行动》静态攻略知识库，包含人工整理的玩法机制、武器使用技巧、"+
			"地图攻略、配装推荐、任务流程、新手教程等资料。"+
			"当用户询问怎么玩、技巧、机制、打法、配装、任务攻略等非实时问题时优先调用本工具；"+
			"实时价格、市场行情、地图密码、库存等动态数据请使用 delta_* 工具。",
		map[string]any{
			"query": ai.StrProp("用户的问题或检索关键词，尽量保留原始语义，例如「M14怎么压枪」、「烽火地带带什么装备」"),
			"top_k": ai.IntProp("可选，返回资料条数，默认5，最大8"),
		},
		[]string{"query"},
		func(ctx context.Context, rawArgs json.RawMessage) (any, error) {
			var args struct {
				Query string `json:"query"`
				TopK  int    `json:"top_k"`
			}
			if err := json.Unmarshal(rawArgs, &args); err != nil {
				return nil, fmt.Errorf("参数格式错误: %w", err)
			}
			if args.Query == "" {
				return nil, fmt.Errorf("query 不能为空")
			}
			k := args.TopK
			if k <= 0 {
				k = defaultTopK
			}
			if k > maxTopK {
				k = maxTopK
			}

			vecs, err := embedder.CreateEmbedding(ctx, []string{args.Query})
			if err != nil {
				return nil, fmt.Errorf("向量化失败: %w", err)
			}
			if len(vecs) != 1 {
				return nil, fmt.Errorf("向量化返回异常：得到 %d 条向量", len(vecs))
			}

			hits := store.Search(Vec32(vecs[0]), k)
			results := make([]map[string]any, 0, len(hits))
			for i, h := range hits {
				content := truncate(h.Chunk.Content, maxResultRunes)
				results = append(results, map[string]any{
					"序号":  i + 1,
					"标题":  h.Chunk.Title,
					"出处":  h.Chunk.Source,
					"相关度": roundScore(h.Score),
					"内容":  content,
				})
			}

			if len(results) == 0 {
				return map[string]any{
					"query": args.Query,
					"提示":    "知识库中没有检索到相关资料，请基于你的常识谨慎回答并说明资料不足。",
				}, nil
			}
			return map[string]any{
				"query": args.Query,
				"提示": "以下是知识库中与问题最相关的资料，请优先依据这些资料回答中文提问；" +
					"若资料之间矛盾或与问题无关，请说明知识库资料不足，不要编造。",
				"资料": results,
			}, nil
		},
	)
}

func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "……(截断)"
}

// roundScore 相关度保留三位小数，模型侧可读性更好
func roundScore(v float32) float32 {
	return float32(int(v*1000+0.5)) / 1000
}
