// cmd/crawler/ocr.go
//
// 纯图片公告的视觉转写（OCR）：官网不少更新公告整篇是一张长图海报，
// 文字提取不到。这里把海报下载下来（超长图纵向切片，避免被模型压缩后
// 看不清小字），调用 server/.env 配置的豆包多模态对话接入点（与线上
// Agent 同一个 ARK_MODEL_ENDPOINT），逐字转写成 Markdown。
//
// 触发条件：htmlToMarkdown 后有效文字不足 100 字，且运行时加了 -ocr。
package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/draw"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"yhagent-server/internal/config"
)

const (
	chatCompletionsPath = "/chat/completions"

	// 海报切片参数：视觉模型会缩放超长图，超过一定高度后小字糊成一团，
	// 因此纵向切成多片、片间留重叠防止行被切断。
	maxSegHeight = 2400
	segOverlap   = 160
	jpegQuality  = 80

	segsPerBatch    = 4  // 每次视觉请求携带的图片切片数（太多会超 token/被压缩）
	maxTotalSegs    = 60 // 单篇公告总切片上限（如44页赛事手册/十几张长图的赛季前瞻）
	ocrMaxTokens    = 8000
	ocrTimeout      = 240 * time.Second
	downloadTimeout = 60 * time.Second
	ocrMaxRetries   = 2
	batchDelay      = 2 * time.Second
)

// ocrPrompt 是视觉转写指令。要求严格忠实原文、输出可直接喂切片器的 Markdown。
var ocrPrompt = strings.TrimSpace(`
你在处理《三角洲行动》游戏官网公告的长图海报。请逐字识别图中的全部文字并转写，要求：
1. 只输出 Markdown 正文，不要输出任何 HTML 标签、页眉 Logo、装饰符号（如 △、[ PART 01 ]、分隔线）；
2. 用 ## 表示公告内的一级分节标题（例如【武器平衡性调整】写成 ## 武器平衡性调整），用 ### 表示其下的子分组；
3. 编号条目用有序列表，并列要点用 - 无序列表；成排的数值/奖励数据用 Markdown 表格；
4. 武器名、干员名、数字、倍率、时间、箭头符号（↑ ↓ →）必须原样保留，禁止脑补、概括或改写；
5. 不要重复公告的大标题（文件里已有标题），不要写开场白、总结或任何解释；
6. 多张图片按从上到下的顺序连续转写，自然衔接，不要标注"图1/图片一"之类；
7. 如果图片是纯装饰封面、没有任何实质公告文字，只输出 [NO_TEXT]，不要输出其他内容。
`)

var (
	imgSrcRe    = regexp.MustCompile(`(?i)<img[^>]+src="([^"]+)"`)
	htmlTagRe   = regexp.MustCompile(`<[^>]+>`)
	partTagRe   = regexp.MustCompile(`^\s*(?:\[?\s*PART[^\]]*\]?|△+|---+|\*\*\*+)\s*$`)
	blankLineRe = regexp.MustCompile(`\n{3,}`)
	// 处罚名单类海报：整篇是成百上千个玩家ID/昵称，对攻略检索无价值且极耗 token
	banTitleRe = regexp.MustCompile(`处罚公告|封禁|违规账号|处罚名单`)
)

type visionClient struct {
	http    *http.Client
	apiKey  string
	model   string
	baseURL string
}

func newVisionClient(_ *http.Client) *visionClient {
	cfg := config.AppConfig
	return &visionClient{
		http:    &http.Client{Timeout: ocrTimeout},
		apiKey:  cfg.DoubaoAPIKey,
		model:   cfg.DoubaoModel,
		baseURL: strings.TrimRight(cfg.DoubaoBaseURL, "/"),
	}
}

// transcribe 从公告 HTML 中提取图片，切片后分批送视觉模型转写，按顺序拼接。
func (v *visionClient) transcribe(rawHTML string) (string, error) {
	imgURLs := extractImageURLs(rawHTML)
	if len(imgURLs) == 0 {
		return "", nil
	}

	dl := &http.Client{Timeout: downloadTimeout}
	var allSegs []string
	for _, u := range imgURLs {
		dataURLs, err := fetchImageSegments(dl, u)
		if err != nil {
			return "", fmt.Errorf("下载图片 %s 失败: %w", u, err)
		}
		allSegs = append(allSegs, dataURLs...)
		if len(allSegs) > maxTotalSegs {
			return "", fmt.Errorf("图片切片超过 %d 张，疑似异常巨型图文，已中止", maxTotalSegs)
		}
	}
	if len(allSegs) == 0 {
		return "", nil
	}

	// 分批识别：每批 segsPerBatch 张，避免单次请求 token 超限、图片被过度压缩
	var sections []string
	for start := 0; start < len(allSegs); start += segsPerBatch {
		end := min(start+segsPerBatch, len(allSegs))
		parts := make([]map[string]any, 0, 1+end-start)
		parts = append(parts, map[string]any{"type": "text", "text": ocrPrompt})
		for _, du := range allSegs[start:end] {
			parts = append(parts, map[string]any{
				"type":      "image_url",
				"image_url": map[string]string{"url": du},
			})
		}
		reqBody := map[string]any{
			"model":       v.model,
			"messages":    []map[string]any{{"role": "user", "content": parts}},
			"max_tokens":  ocrMaxTokens,
			"temperature": 0.1,
		}

		var text string
		var truncated bool
		var err error
		for attempt := 1; attempt <= ocrMaxRetries; attempt++ {
			text, truncated, err = v.chatOnce(reqBody)
			if err == nil {
				break
			}
			if attempt < ocrMaxRetries {
				time.Sleep(time.Duration(attempt*5) * time.Second)
			}
		}
		if err != nil {
			return "", fmt.Errorf("第%d/%d批转写失败: %w", start/segsPerBatch+1,
				(len(allSegs)+segsPerBatch-1)/segsPerBatch, err)
		}
		if strings.TrimSpace(text) != "[NO_TEXT]" {
			sections = append(sections, text)
		}
		if truncated {
			log.Printf("[crawler]   警告：第 %d 批因输出长度上限被截断，尾部内容可能缺失", start/segsPerBatch+1)
		}
		if end < len(allSegs) {
			time.Sleep(batchDelay)
		}
	}
	return strings.Join(sections, "\n\n"), nil
}

func (v *visionClient) chatOnce(reqBody map[string]any) (text string, truncated bool, err error) {
	raw, err := json.Marshal(reqBody)
	if err != nil {
		return "", false, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), ocrTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, v.baseURL+chatCompletionsPath, bytes.NewReader(raw))
	if err != nil {
		return "", false, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+v.apiKey)

	resp, err := v.http.Do(req)
	if err != nil {
		return "", false, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))

	if resp.StatusCode != http.StatusOK {
		return "", false, fmt.Errorf("视觉接口 HTTP %d: %s", resp.StatusCode, truncate(string(body), 200))
	}

	var parsed struct {
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", false, fmt.Errorf("解析视觉响应失败: %w; 原文: %s", err, truncate(string(body), 200))
	}
	if parsed.Error != nil {
		return "", false, fmt.Errorf("视觉接口错误 %s: %s", parsed.Error.Code, parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return "", false, fmt.Errorf("视觉响应无 choices: %s", truncate(string(body), 200))
	}
	c := parsed.Choices[0]
	return c.Message.Content, c.FinishReason == "length", nil
}

// extractImageURLs 从公告 HTML 中按顺序提取去重后的图片地址，
// 兼容协议相对地址（//static.gametalk.qq.com/...）。
func extractImageURLs(htmlText string) []string {
	seen := map[string]bool{}
	var urls []string
	for _, m := range imgSrcRe.FindAllStringSubmatch(htmlText, -1) {
		u := strings.TrimSpace(m[1])
		if strings.HasPrefix(u, "//") {
			u = "https:" + u
		}
		if !strings.HasPrefix(u, "http") || seen[u] {
			continue
		}
		seen[u] = true
		urls = append(urls, u)
	}
	return urls
}

// fetchImageSegments 下载一张图，必要时纵向切片，统一转成 JPEG data URL。
func fetchImageSegments(client *http.Client, imgURL string) ([]string, error) {
	req, err := http.NewRequest(http.MethodGet, imgURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Referer", "https://df.qq.com/")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) "+
		"AppleWebKit/537.36 (KHTML, like Gecko) Chrome/138.0.0.0 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("不支持的图片格式（png/jpg/gif 以外，如 webp）: %w", err)
	}

	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if h <= maxSegHeight {
		du, err := encodeDataURL(img)
		if err != nil {
			return nil, err
		}
		return []string{du}, nil
	}

	// 纵向重叠切片
	var durls []string
	for y0 := 0; y0 < h; y0 += maxSegHeight - segOverlap {
		y1 := y0 + maxSegHeight
		if y1 > h {
			y1 = h
		}
		seg := image.NewRGBA(image.Rect(0, 0, w, y1-y0))
		draw.Draw(seg, seg.Bounds(), img, image.Pt(bounds.Min.X, bounds.Min.Y+y0), draw.Src)
		du, err := encodeDataURL(seg)
		if err != nil {
			return nil, err
		}
		durls = append(durls, du)
		if y1 == h {
			break
		}
	}
	return durls, nil
}

// encodeDataURL 编码为 JPEG data URL（海报转 JPEG 体积小，模型识别无差异）
func encodeDataURL(img image.Image) (string, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return "", err
	}
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

// isBanListTitle 判断是否为处罚名单类公告（整篇玩家ID列表，无攻略价值）
func isBanListTitle(title string) bool {
	return banTitleRe.MatchString(title)
}

// cleanOCRText 清洗模型输出：去 HTML 标签、装饰行、重复大标题、多余空行。
func cleanOCRText(text, title string) string {
	text = strings.TrimSpace(text)
	if text == "[NO_TEXT]" {
		return ""
	}

	var kept []string
	titleNorm := normalizeTitle(title)
	skippedLeadingTitle := false

	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimRight(line, " \t")
		trimmed := strings.TrimSpace(line)

		if trimmed == "" {
			kept = append(kept, "")
			continue
		}
		// 去掉残留 HTML 标签行/行内标签
		if htmlTagRe.MatchString(trimmed) && strings.TrimSpace(htmlTagRe.ReplaceAllString(trimmed, "")) == "" {
			continue
		}
		line = htmlTagRe.ReplaceAllString(line, "")
		trimmed = strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if partTagRe.MatchString(trimmed) {
			continue
		}
		// 去掉开头重复的公告大标题（# 标题 或与标题高度相似的行），只处理前几行
		if !skippedLeadingTitle && len(kept) < 6 {
			plain := strings.TrimLeft(trimmed, "# ")
			if normalizeTitle(plain) == titleNorm || strings.Contains(plain, "更新公告") && len([]rune(plain)) <= 40 {
				skippedLeadingTitle = true
				continue
			}
		}
		kept = append(kept, line)
	}

	// 折叠相邻的重复行（长图切片重叠处，模型常会把同一行转写两遍）
	var deduped []string
	prev := ""
	for _, l := range kept {
		if strings.TrimSpace(l) != "" && l == prev {
			continue
		}
		deduped = append(deduped, l)
		prev = l
	}

	out := strings.Join(deduped, "\n")
	out = blankLineRe.ReplaceAllString(out, "\n\n")
	return strings.TrimSpace(out)
}

func normalizeTitle(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimLeft(s, "# ")
	// 去掉日期和常见标点后比较，容忍"9月30日 更新公告"这类海报写法
	for _, p := range []string{" ", "　", "丨", "|", "｜", "：", ":"} {
		s = strings.ReplaceAll(s, p, "")
	}
	return s
}
