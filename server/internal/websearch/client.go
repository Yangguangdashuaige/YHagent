// Package websearch 通过火山方舟 Responses API 的原生联网内容插件（web_search）
// 把"知识库没覆盖到/已过时"的问题联网核实，搜索摘要与来源链接回传给 Agent 作答，
// 同时通过 context 回调把候选知识推给网页端，供管理员确认后写入补充知识库。
//
// 注意：使用前需在火山方舟控制台开通「联网内容插件」：
//
//	https://console.volcengine.com/common-buy/CC_content_plugin
//	未开通时上游会返回 code=ToolNotOpen，本包统一转成 ErrNotActivated。
//
// 纯 net/http 实现（项目零三方依赖约定），不引入方舟 SDK 里的 Responses 类型
// （ark-runtime-go v0.6.0 的 Chat 工具只生成了 function 类型，无法表达 web_search）。
package websearch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Source 一条引用来源
type Source struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

// Result 联网搜索结果
type Result struct {
	Query   string   // 模型实际使用的搜索词（可能对原问题做了改写）
	Answer  string   // 模型基于网页内容生成的中文摘要
	Sources []Source // 引用来源（去重）
}

// ErrNotActivated 账号未开通联网内容插件
var ErrNotActivated = errors.New("火山方舟「联网内容插件」尚未开通，请管理员先到控制台开通")

const defaultBaseURL = "https://ark.cn-beijing.volces.com/api/v3"

// Client 方舟 Responses API 客户端
type Client struct {
	apiKey  string
	baseURL string
	model   string
	http    *http.Client
}

// New 创建客户端。model 可直接传推理接入点 ep-xxx（实测 /responses 接受接入点）。
func New(apiKey, baseURL, model string) *Client {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return &Client{
		apiKey:  apiKey,
		baseURL: baseURL,
		model:   model,
		// 联网检索 + 生成通常 10~40s，留足余量；上层 Agent 工具超时已放宽到 45s
		http: &http.Client{Timeout: 40 * time.Second},
	}
}

// responsesRequest /responses 接口请求体（只放需要的字段）
type responsesRequest struct {
	Model  string        `json:"model"`
	Stream bool          `json:"stream"`
	Tools  []interface{} `json:"tools"`
	Input  []respMessage `json:"input"`
}

type respMessage struct {
	Role    string     `json:"role"`
	Content []respPart `json:"content"`
}

type respPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// arkError 方舟错误响应体
type arkError struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// Search 执行一次联网搜索并返回模型整理后的摘要与来源
func (c *Client) Search(ctx context.Context, query string) (*Result, error) {
	if strings.TrimSpace(c.apiKey) == "" || strings.TrimSpace(c.model) == "" {
		return nil, errors.New("联网搜索未配置：缺少 ARK_API_KEY 或 ARK_MODEL_ENDPOINT")
	}
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, errors.New("搜索关键词不能为空")
	}

	reqBody := responsesRequest{
		Model:  c.model,
		Stream: false,
		Tools: []interface{}{
			// 官方联网内容插件；模型自行决定是否真正发起搜索
			map[string]any{"type": "web_search"},
		},
		Input: []respMessage{{
			Role: "user",
			Content: []respPart{{
				Type: "input_text",
				Text: "请联网搜索以下关于游戏《三角洲行动》的问题，依据搜索到的网页内容用中文简洁作答，" +
					"保留具体步骤、点位、前置条件、活动时间等关键信息：" + q,
			}},
		}},
	}
	raw, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/responses", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("联网搜索请求失败: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("读取搜索响应失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		var ae arkError
		if json.Unmarshal(data, &ae) == nil && ae.Error.Code != "" {
			if ae.Error.Code == "ToolNotOpen" {
				return nil, ErrNotActivated
			}
			return nil, fmt.Errorf("联网搜索上游错误(%s): %s", ae.Error.Code, ae.Error.Message)
		}
		return nil, fmt.Errorf("联网搜索失败，HTTP %d", resp.StatusCode)
	}

	// 响应为 OpenAI Responses 风格 JSON，字段在不同模型版本间可能有差异，做兼容式解析
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("搜索响应解析失败: %w", err)
	}
	out := parseResponses(doc)
	out.Query = firstNonEmpty(out.Query, q)
	if strings.TrimSpace(out.Answer) == "" {
		return nil, errors.New("联网搜索未返回有效内容，请换个关键词再试")
	}
	return out, nil
}

// parseResponses 从 Responses 对象里提取：模型实际搜索词、回答正文、引用来源
func parseResponses(doc map[string]any) *Result {
	out := &Result{}

	output, _ := doc["output"].([]any)
	for _, item := range output {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		switch m["type"] {
		case "message":
			parts, _ := m["content"].([]any)
			for _, p := range parts {
				pm, ok := p.(map[string]any)
				if !ok {
					continue
				}
				t, _ := pm["type"].(string)
				text, _ := pm["text"].(string)
				if (t == "output_text" || t == "text" || t == "") && text != "" {
					if out.Answer != "" {
						out.Answer += "\n"
					}
					out.Answer += text
				}
			}
		case "web_search_call":
			if out.Query == "" {
				out.Query = extractQuery(m)
			}
		}
	}

	// 引用来源的标准位置是 message.content[].annotations[]（type=url_citation），
	// 优先从这里精确提取；部分模型版本层级不同，再用递归兜底。
	// 注意：annotations 里还有 logo_url 等 http(s) 图片链接，递归时必须排除这些噪声键。
	seen := map[string]bool{}
	addSource := func(title, u string) {
		u = strings.TrimSpace(u)
		if !strings.HasPrefix(u, "http") || seen[u] {
			return
		}
		seen[u] = true
		out.Sources = append(out.Sources, Source{Title: strings.TrimSpace(title), URL: u})
	}
	for _, item := range output {
		m, ok := item.(map[string]any)
		if !ok || m["type"] != "message" {
			continue
		}
		parts, _ := m["content"].([]any)
		for _, p := range parts {
			pm, ok := p.(map[string]any)
			if !ok {
				continue
			}
			annos, _ := pm["annotations"].([]any)
			for _, a := range annos {
				am, ok := a.(map[string]any)
				if !ok {
					continue
				}
				if t, _ := am["type"].(string); t != "" && t != "url_citation" {
					continue
				}
				title, _ := am["title"].(string)
				u, _ := am["url"].(string)
				addSource(title, u)
			}
		}
	}

	if len(out.Sources) == 0 {
		var walk func(any)
		walk = func(v any) {
			switch x := v.(type) {
			case map[string]any:
				if u, _ := x["url"].(string); strings.HasPrefix(u, "http") {
					title, _ := x["title"].(string)
					addSource(title, u)
				}
				for k, vv := range x {
					if isNoiseURLKey(k) {
						continue
					}
					walk(vv)
				}
			case []any:
				for _, vv := range x {
					walk(vv)
				}
			}
		}
		walk(doc)
	}

	return out
}

// isNoiseURLKey 递归兜底时需要跳过的非引用类链接字段（图标/图片签名地址等）
func isNoiseURLKey(k string) bool {
	switch k {
	case "logo_url", "icon_url", "favicon_url", "avatar_url", "image_url", "image", "thumbnail":
		return true
	}
	return false
}

// extractQuery 取模型实际搜索词。真实响应里字段为 action.query（字符串，
// 多关键词以分号分隔）；兼容部分版本的 action.queries 数组形态。
func extractQuery(m map[string]any) string {
	action, _ := m["action"].(map[string]any)
	if action == nil {
		return ""
	}
	if s, _ := action["query"].(string); strings.TrimSpace(s) != "" {
		s = strings.TrimSpace(s)
		if i := strings.IndexAny(s, ";；"); i > 0 {
			s = strings.TrimSpace(s[:i])
		}
		return s
	}
	queries, _ := action["queries"].([]any)
	for _, q := range queries {
		switch v := q.(type) {
		case string:
			if strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		case map[string]any:
			if s, _ := v["query"].(string); s != "" {
				return strings.TrimSpace(s)
			}
			if s, _ := v["text"].(string); s != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
