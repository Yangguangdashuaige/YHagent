// internal/ai/rerank.go
//
// 检索结果重排（rerank）能力：对混合检索产出的候选切片按"能否回答 query"
// 重新打分排序，修复余弦/RRF 排序中的近名混淆、日期错位等问题。
//
// 两个实现，接口一致，可分别接入后在 eval 上对比择优：
//   - LLMReranker：复用方舟 chat 接口与 ARK_API_KEY，用 lite 档模型关闭深度思考
//     做列表排序（零新增凭据，延迟约 1-3s）；
//   - VikingReranker：火山 VikingDB 服务的专用重排模型 doubao-seed-rerank
//     （cross-encoder，亚秒级；鉴权走火山 IAM AK/SK 的 V4 签名，与 ARK key 无关）。
//
// 注：方舟 /api/v3 下没有公开 rerank 端点（实测 404、模型列表亦无 rerank 模型），
// 火山的 rerank 模型统一挂在 VikingDB 服务下，故有 VikingReranker 一路。
package ai

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"yhagent-server/internal/config"
)

// RerankDoc 一条待重排的候选文档
type RerankDoc struct {
	Title   string // 面包屑标题（如 "武器攻略 > M14 > 压枪技巧"）
	Content string // 正文（可截断）
}

// Reranker 检索候选重排能力：返回与 docs 顺序对齐的相关性分数（越大越相关）
type Reranker interface {
	// Name 实现标识（报告/日志用），如 "llm:doubao-seed-2-1-lite"、"viking:doubao-seed-rerank"
	Name() string
	Rerank(ctx context.Context, query string, docs []RerankDoc) ([]float64, error)
}

// ---------------- LLM 重排 ----------------

const (
	llmRerankTimeout   = 60 * time.Second
	llmRerankDocRunes  = 220 // 单条候选喂给 LLM 的正文截断长度
	llmRerankMaxCand   = 50  // 候选数上限（防止 prompt 膨胀）
	llmRerankSysPrompt = "你是游戏《三角洲行动》攻略知识库的检索结果重排器。根据用户问题判断每条候选资料的内容能否直接回答该问题，按相关度从高到低输出编号排序。注意区分名称相近但不同的游戏物品与机制（如不同的背包、枪械型号、日期不同的公告）；与问题无关的资料排在后面。只输出 JSON，不要输出任何其他文字，格式：{\"rank\":[编号,编号,…]}，编号从 1 开始，必须不重不漏覆盖全部候选。"
)

// LLMReranker 用方舟 chat 接口（非流式）做列表重排
type LLMReranker struct {
	client *SeedClient
	model  string
}

// NewLLMReranker 构造 LLM 重排器；模型未配置（ARK_RERANK_LLM_MODEL 留空）时
// 返回 nil，调用方据此跳过 LLM 重排（显式配置才启用，避免未开通模型的无效调用）
func NewLLMReranker(c *SeedClient) *LLMReranker {
	model := config.AppConfig.RerankLLMModel
	if model == "" {
		return nil
	}
	return &LLMReranker{client: c, model: model}
}

func (r *LLMReranker) Name() string { return "llm:" + r.model }

// Rerank 一次 chat 调用完成全部候选的列表排序，返回按位置换算的单调分数。
// 分数长度恒等于入参 docs（超出 llmRerankMaxCand 的尾部按原序垫负分）
func (r *LLMReranker) Rerank(ctx context.Context, query string, docs []RerankDoc) ([]float64, error) {
	if len(docs) == 0 {
		return nil, nil
	}
	head := docs
	if len(head) > llmRerankMaxCand {
		head = head[:llmRerankMaxCand]
	}

	var sb strings.Builder
	sb.WriteString("【用户问题】")
	sb.WriteString(query)
	sb.WriteString("\n\n【候选资料】")
	for i, d := range head {
		sb.WriteString(fmt.Sprintf("\n%d.【%s】%s", i+1, d.Title, truncateRunes(d.Content, llmRerankDocRunes)))
	}

	content, err := r.chatOnce(ctx, llmRerankSysPrompt, sb.String())
	if err != nil {
		return nil, err
	}
	order := parseRankList(content, len(head))
	// 位置换算分数：第 p 名得 len(order)-p，保证单调
	scores := make([]float64, len(docs))
	assigned := make([]bool, len(head))
	for pos, idx := range order {
		scores[idx] = float64(len(order) - pos)
		assigned[idx] = true
	}
	// 防御模型漏号：未排到的 head 候选按原序垫 0 分以下；超限尾部更低
	next := 0.0
	for i := 0; i < len(head); i++ {
		if !assigned[i] {
			scores[i] = next
			next -= 0.001
		}
	}
	for i := len(head); i < len(docs); i++ {
		scores[i] = -1 - float64(i)*0.001
	}
	return scores, nil
}

// chatOnce 单轮非流式 chat（关闭深度思考、低温）。不复用 ChatStream：
// rerank 需要独立的小模型与 thinking 参数，走标准库直调避免改动现有客户端逻辑。
func (r *LLMReranker) chatOnce(ctx context.Context, system, user string) (string, error) {
	body := map[string]any{
		"model": r.model,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
		"thinking":    map[string]string{"type": "disabled"},
		"temperature": 0.1,
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, llmRerankTimeout)
	defer cancel()

	url := strings.TrimRight(config.AppConfig.DoubaoBaseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+config.AppConfig.DoubaoAPIKey)

	resp, err := r.client.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("LLM 重排请求失败: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", fmt.Errorf("LLM 重排读取响应失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("LLM 重排接口返回 HTTP %d: %.200s", resp.StatusCode, string(data))
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Code    any    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return "", fmt.Errorf("LLM 重排响应解析失败: %w", err)
	}
	if out.Error != nil {
		return "", fmt.Errorf("LLM 重排接口错误: %s (%v)", out.Error.Message, out.Error.Code)
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("LLM 重排响应无 choices")
	}
	return out.Choices[0].Message.Content, nil
}

var rankNumRe = regexp.MustCompile(`\d+`)

// parseRankList 从模型输出里鲁棒提取编号序列：优先解析 {"rank":[...]} JSON，
// 失败则退化为全文数字扫描；越界/重复编号剔除，缺失编号按原序补尾，
// 保证返回的是 n 个不重复下标（0-based）。
func parseRankList(content string, n int) []int {
	content = strings.TrimSpace(content)
	// 剥掉 ```json 围栏
	if i := strings.Index(content, "["); i >= 0 {
		if j := strings.LastIndex(content, "]"); j > i {
			var nums []int
			if err := json.Unmarshal([]byte(content[i:j+1]), &nums); err == nil {
				if order := validOrder(nums, n); order != nil {
					return order
				}
			}
		}
	}
	// 退化：全文数字扫描
	nums := make([]int, 0, n)
	for _, m := range rankNumRe.FindAllString(content, -1) {
		if v, err := strconv.Atoi(m); err == nil {
			nums = append(nums, v)
		}
	}
	if order := validOrder(nums, n); order != nil {
		return order
	}
	// 完全解析失败：保持原序
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	return order
}

// validOrder 把 1-based 编号列表清洗成合法 0-based 全排列；输入不可用时返回 nil
func validOrder(nums []int, n int) []int {
	seen := make([]bool, n)
	order := make([]int, 0, n)
	for _, v := range nums {
		if v >= 1 && v <= n && !seen[v-1] {
			seen[v-1] = true
			order = append(order, v-1)
		}
	}
	if len(order) == 0 {
		return nil
	}
	for i := 0; i < n; i++ {
		if !seen[i] {
			order = append(order, i)
		}
	}
	return order
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// ---------------- VikingDB 专用重排模型 ----------------

const (
	// SaaS 环境真实端点：volc-sdk-golang service/vikingdb 的 BatchRerank 所用路径
	// （注意不是知识库文档里的 /api/knowledge/service/rerank，那是私有化实例路径，实测 404）
	vikingRerankPath    = "/api/index/batch_rerank"
	vikingRerankTimeout = 30 * time.Second
)

// VikingReranker 调 VikingDB 服务的专用重排模型。
// 鉴权：火山 IAM AK/SK 的 HMAC-SHA256 V4 签名（service=air，region=cn-beijing）。
// 请求/响应格式与 volc-sdk-golang VikingDBService.BatchRerank 完全一致：
// body {"datas":[{query,content,title}…]}，响应 data 为与输入对齐的 float 数组。
type VikingReranker struct {
	ak    string
	sk    string
	host  string
	model string // 报告/日志标识（SaaS 端点由服务端定模型，不随请求下发）
	http  *http.Client
}

// NewVikingReranker 从配置构造；AK/SK 未配置时返回 nil（调用方据此跳过该实现）
func NewVikingReranker() *VikingReranker {
	cfg := config.AppConfig
	if cfg.VikingAccessKey == "" || cfg.VikingSecretKey == "" {
		return nil
	}
	return &VikingReranker{
		ak:    cfg.VikingAccessKey,
		sk:    cfg.VikingSecretKey,
		host:  cfg.VikingHost,
		model: cfg.VikingRerankModel,
		http:  &http.Client{Timeout: vikingRerankTimeout},
	}
}

func (v *VikingReranker) Name() string { return "viking:" + v.model }

type vikingRerankData struct {
	Query   string `json:"query"`
	Title   string `json:"title,omitempty"`
	Content string `json:"content"`
}

// Rerank 单次请求批量重排（datas 与响应 data 数组一一对应）
func (v *VikingReranker) Rerank(ctx context.Context, query string, docs []RerankDoc) ([]float64, error) {
	if len(docs) == 0 {
		return nil, nil
	}
	datas := make([]vikingRerankData, len(docs))
	for i, d := range docs {
		datas[i] = vikingRerankData{Query: query, Title: d.Title, Content: d.Content}
	}
	raw, err := json.Marshal(map[string]any{"datas": datas})
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, vikingRerankTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://"+v.host+vikingRerankPath, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	v.sign(req, raw)

	resp, err := v.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("VikingDB 重排请求失败: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("VikingDB 重排读取响应失败: %w", err)
	}
	// 响应两种形态都兼容：data 直接是分数数组（SDK 实测形态），
	// 或 data.scores 数组（知识库文档形态）
	var out struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Scores []float64 `json:"scores"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("VikingDB 重排响应解析失败(HTTP %d): %w; body: %.200s",
			resp.StatusCode, err, string(data))
	}
	if resp.StatusCode != http.StatusOK || (out.Code != 0 && out.Message != "") {
		return nil, fmt.Errorf("VikingDB 重排接口错误(HTTP %d, code=%d): %s",
			resp.StatusCode, out.Code, out.Message)
	}
	// data 为数组形态：重新解析
	var scores []float64
	if err := json.Unmarshal(out2Data(data), &scores); err == nil && len(scores) > 0 {
		if len(scores) != len(docs) {
			return nil, fmt.Errorf("VikingDB 重排返回 %d 个分数，与 %d 条候选不一致", len(scores), len(docs))
		}
		return scores, nil
	}
	if len(out.Data.Scores) > 0 {
		if len(out.Data.Scores) != len(docs) {
			return nil, fmt.Errorf("VikingDB 重排返回 %d 个分数，与 %d 条候选不一致", len(out.Data.Scores), len(docs))
		}
		return out.Data.Scores, nil
	}
	return nil, fmt.Errorf("VikingDB 重排响应无分数(HTTP %d): %.200s", resp.StatusCode, string(data))
}

// out2Data 提取响应 JSON 中 "data" 字段的原始值（可能是数组或对象）
func out2Data(body []byte) json.RawMessage {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(body, &m); err != nil {
		return nil
	}
	return m["data"]
}

// sign 给请求加火山 OpenAPI V4 签名（HMAC-SHA256，service=air/cn-beijing）。
// 与 volc-sdk-golang base.Sign4 逐字段对齐（含 X-Content-Sha256 头签名），
// 用标准库实现避免把 SDK 提升为直接依赖。
// 算法文档：https://www.volcengine.com/docs/6369/67265
func (v *VikingReranker) sign(req *http.Request, payload []byte) {
	now := time.Now().UTC()
	shortDate := now.Format("20060102")
	longDate := now.Format("20060102T150405Z")
	region, service := "cn-beijing", "air"

	payloadHash := hex.EncodeToString(sumSHA256(payload))
	req.Header.Set("X-Content-Sha256", payloadHash)
	req.Header.Set("X-Date", longDate)

	// 参与签名的头按字母序：content-type / host / x-content-sha256 / x-date
	canonicalHeaders := "content-type:application/json\n" +
		"host:" + v.host + "\n" +
		"x-content-sha256:" + payloadHash + "\n" +
		"x-date:" + longDate + "\n"
	signedHeaders := "content-type;host;x-content-sha256;x-date"
	canonicalRequest := strings.Join([]string{
		http.MethodPost,
		req.URL.Path,
		"",               // query string 为空
		canonicalHeaders, // 自带尾换行，拼接后与 SDK 一致形成空行分隔
		signedHeaders,
		payloadHash,
	}, "\n")

	crHash := sumSHA256([]byte(canonicalRequest))
	credentialScope := shortDate + "/" + region + "/" + service + "/request"
	stringToSign := strings.Join([]string{
		"HMAC-SHA256",
		longDate,
		credentialScope,
		hex.EncodeToString(crHash),
	}, "\n")

	kDate := hmacSHA256Str([]byte(v.sk), shortDate)
	kRegion := hmacSHA256Str(kDate, region)
	kService := hmacSHA256Str(kRegion, service)
	kSigning := hmacSHA256Str(kService, "request")
	signature := hex.EncodeToString(hmacSHA256Str(kSigning, stringToSign))

	req.Header.Set("Authorization", fmt.Sprintf(
		"HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		v.ak, credentialScope, signedHeaders, signature))
}

func sumSHA256(data []byte) []byte {
	h := sha256.Sum256(data)
	return h[:]
}

func hmacSHA256(key, data []byte) []byte {
	m := hmac.New(sha256.New, key)
	m.Write(data)
	return m.Sum(nil)
}

func hmacSHA256Str(key []byte, data string) []byte {
	return hmacSHA256(key, []byte(data))
}

// SortDocsByScore 是给调用方（eval/生产工具）的通用兜底：按分数降序稳定排序
// docs（同分保持原顺序），返回排序后的新下标序列。
func SortDocsByScore(scores []float64) []int {
	idx := make([]int, len(scores))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool { return scores[idx[a]] > scores[idx[b]] })
	return idx
}
