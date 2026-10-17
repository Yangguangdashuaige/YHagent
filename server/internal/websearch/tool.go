// internal/websearch/tool.go
//
// 把联网搜索包装成 Agent 的函数工具 web_search。
// 工具执行成功后，除了把摘要回传给模型作答，还会通过 context 中的回调
// 把"候选知识卡片"推给网页端（SSE candidate 事件），由玩家/管理员确认入库。
package websearch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"yhagent-server/internal/ai"
)

// Candidate 待确认的联网知识卡片（服务端 → 网页端）
type Candidate struct {
	Query      string   `json:"query"`             // 实际搜索词
	Summary    string   `json:"summary"`           // 模型整理的摘要（入库原料）
	Sources    []Source `json:"sources"`           // 引用来源
	SearchedAt string   `json:"searched_at"`       // RFC3339 时间
	Activated  bool     `json:"activated"`         // 插件是否可用（不可用时只提示不推卡片）
}

type candidateFuncKey struct{}

// WithCandidateFunc 把候选知识回调注入 context（服务端 SSE 层注入）
func WithCandidateFunc(ctx context.Context, f func(Candidate)) context.Context {
	if f == nil {
		return ctx
	}
	return context.WithValue(ctx, candidateFuncKey{}, f)
}

func emitCandidate(ctx context.Context, c Candidate) {
	f, ok := ctx.Value(candidateFuncKey{}).(func(Candidate))
	if ok && f != nil {
		f(c)
	}
}

// RegisterTool 注册 web_search 函数工具
func RegisterTool(reg *ai.Registry, c *Client) {
	reg.Register(ai.NewFunctionTool(
		"web_search",
		"联网搜索公开网页信息，用于补充知识库没有收录或可能过时的《三角洲行动》资料，"+
			"例如近期活动、新版本内容、限时道具/收藏品的获取方法、新干员机制等。"+
			"当 search_knowledge 没有命中、命中资料明显与问题无关，或用户明确要求联网/查询最新消息时调用本工具。"+
			"实时市场价格、库存、密码仍使用 delta_* 工具，不要用本工具查询。",
		map[string]any{
			"query": ai.StrProp("准确的中文搜索关键词，建议包含「三角洲行动」和核心实体，" +
				"例如「三角洲行动 守夜人口琴 怎么获得」"),
		},
		[]string{"query"},
		func(ctx context.Context, rawArgs json.RawMessage) (any, error) {
			var args struct {
				Query string `json:"query"`
			}
			if err := json.Unmarshal(rawArgs, &args); err != nil {
				return nil, fmt.Errorf("参数格式错误: %w", err)
			}
			if args.Query == "" {
				return nil, errors.New("query 不能为空")
			}

			res, err := c.Search(ctx, args.Query)
			if err != nil {
				if errors.Is(err, ErrNotActivated) {
					// 明确告知模型原因，模型可降级为"建议稍后再问/咨询管理员"，不暴露控制台链接
					return nil, ErrNotActivated
				}
				return nil, err
			}

			// 推送候选卡片给前端（回调由 SSE 层提供，无回调时静默跳过）
			emitCandidate(ctx, Candidate{
				Query:      res.Query,
				Summary:    res.Answer,
				Sources:    res.Sources,
				SearchedAt: time.Now().Format(time.RFC3339),
				Activated:  true,
			})

			sources := make([]map[string]string, 0, len(res.Sources))
			for _, s := range res.Sources {
				sources = append(sources, map[string]string{"标题": s.Title, "链接": s.URL})
			}
			return map[string]any{
				"query":  args.Query,
				"提示": "以下内容来自联网实时搜索（非本地知识库）。请据此回答中文提问，" +
					"并在回答末尾用「📎 来源」列出参考链接；若搜索内容与问题无关或可信度不足，要明确说明。" +
					"不要宣称这些内容已被知识库收录。",
				"内容": res.Answer,
				"来源": sources,
			}, nil
		},
	))
}
