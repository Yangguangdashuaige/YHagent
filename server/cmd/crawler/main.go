// cmd/crawler/main.go
//
// 三角洲行动官网公告离线爬虫（只在你自己的开发机上运行，不部署到服务器）。
//
// 数据来自官网新闻页 https://df.qq.com/cp/a20240906main/newslist.html
// 背后的腾讯 AMS 内容接口（POST https://comm.ams.game.qq.com/ide/）：
//
//   - 列表：iChartId/iSubChartId=329849，sIdeToken=o3Iw53，
//     参数 chanid（6896=公告 / 6895=最新 / 6898=新闻 / 7037=赛事）、
//     typeids=1（资讯）、limit、start（偏移量）
//   - 详情：iChartId/iSubChartId=329870，sIdeToken=vDQriw，参数 id=docid，
//     正文在 data[0].sContent（HTML）
//
// 抓到的每篇公告转成 Markdown（图片丢弃——向量检索用不上，且 chunk.go
// 只切文本），平铺写入 data/knowledge/ 目录，随后跑 cmd/indexer 即可建库。
//
// 用法（在 server/ 目录下）：
//
//	go run ./cmd/crawler                 # 增量抓取"公告"分类全部
//	go run ./cmd/crawler -chan 6898      # 抓"新闻"分类
//	go run ./cmd/crawler -maxdocs 10     # 只抓最新10篇（试跑用）
//	go run ./cmd/crawler -ocr -ids=123,456 # 只补跑指定文档ID（需配合 -ocr）
//
// 增量：已写入的文档ID记录在 Markdown 头部，重跑时自动跳过。
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"yhagent-server/internal/config"
)

const (
	ideURL = "https://comm.ams.game.qq.com/ide/"

	listChart = "329849"
	listToken = "o3Iw53"
	infoChart = "329870"
	infoToken = "vDQriw"

	pageSize  = 50 // 列表单页条数（接口验证可用）
	pageDelay = 600 * time.Millisecond
	infoDelay = 1200 * time.Millisecond // 礼貌限速，降低对官网的压力
)

// chanNames 资讯频道 id -> 中文名（写入 Markdown 头部，便于阅读）
var chanNames = map[string]string{
	"6896": "公告",
	"6895": "最新",
	"6898": "新闻",
	"7037": "赛事",
}

// listItem 列表项（只取需要的字段；接口存在重复键 sTagIds/sTagids，
// encoding/json 对重复键按最后一个解析，不影响这些字段）
type listItem struct {
	DocID   string `json:"iDocID"`
	Title   string `json:"sTitle"`
	IdxTime string `json:"sIdxTime"`
}

type listResp struct {
	IRet  int    `json:"iRet"`
	SMsg  string `json:"sMsg"`
	JData struct {
		Data struct {
			Total int        `json:"total"`
			Items []listItem `json:"items"`
		} `json:"data"`
	} `json:"jData"`
}

type infoResp struct {
	IRet  int    `json:"iRet"`
	SMsg  string `json:"sMsg"`
	JData struct {
		Data []struct {
			Title   string `json:"sTitle"`
			IdxTime string `json:"sIdxTime"`
			Content string `json:"sContent"`
		} `json:"data"`
	} `json:"jData"`
}

func main() {
	chanID := flag.String("chan", "6896", "资讯频道：6896=公告 6895=最新 6898=新闻 7037=赛事")
	outDir := flag.String("out", "data/knowledge", "Markdown 输出目录（与 indexer 的 -src 一致）")
	maxDocs := flag.Int("maxdocs", 0, "最多抓取多少篇（0=全部，试跑可设 10）")
	ocr := flag.Bool("ocr", false, "对纯图片公告调用豆包视觉模型转写成文字（需要 server/.env 中的 ARK 配置，按篇产生少量 token 费用）")
	idsFlag := flag.String("ids", "", "只抓指定文档ID（逗号分隔，补跑特定公告用）")
	flag.Parse()

	if _, ok := chanNames[*chanID]; !ok {
		log.Fatalf("未知频道 %q，可选 6896/6895/6898/7037", *chanID)
	}
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		log.Fatalf("创建输出目录失败: %v", err)
	}

	client := &http.Client{Timeout: 30 * time.Second}

	// OCR 模式：加载 server/.env 中的豆包配置，初始化视觉转写客户端
	var vision *visionClient
	if *ocr {
		if err := config.Load(); err != nil {
			log.Fatalf("OCR 初始化失败: %v", err)
		}
		if config.AppConfig.DoubaoAPIKey == "" || config.AppConfig.DoubaoModel == "" {
			log.Fatal("OCR 需要在 server/.env 配置 ARK_API_KEY 和 ARK_MODEL_ENDPOINT")
		}
		vision = newVisionClient(client)
		log.Printf("[crawler] OCR 已开启：纯图片公告将通过模型接入点 %s 转写", vision.model)
	}

	// 1. 增量去重：扫描 data/knowledge/ 里已有的"文档ID：xxx"标记
	have, err := scanExistingDocs(*outDir)
	if err != nil {
		log.Fatalf("扫描已有文档失败: %v", err)
	}
	log.Printf("[crawler] 知识库中已有 %d 篇官网文档，将自动跳过", len(have))

	// 2. 翻页拉列表
	var all []listItem
	for start := 0; ; start += pageSize {
		page, total, err := fetchList(client, *chanID, start)
		if err != nil {
			log.Fatalf("拉取列表 start=%d 失败: %v", start, err)
		}
		if start == 0 {
			log.Printf("[crawler] 频道「%s」共 %d 篇", chanNames[*chanID], total)
		}
		all = append(all, page...)
		log.Printf("[crawler] 列表 %d/%d", min(len(all), total), total)
		if len(page) == 0 || len(all) >= total {
			break
		}
		time.Sleep(pageDelay)
	}
	// 按 -ids 白名单过滤（补跑特定公告，省去全量扫描与无关费用）
	if *idsFlag != "" {
		want := map[string]bool{}
		for _, s := range strings.Split(*idsFlag, ",") {
			if s = strings.TrimSpace(s); s != "" {
				want[s] = true
			}
		}
		picked := all[:0]
		for _, it := range all {
			if want[it.DocID] {
				picked = append(picked, it)
				delete(want, it.DocID)
			}
		}
		all = picked
		// 列表里没有的 ID（已被挤出列表窗口等）：直接合成条目，
		// 标题/时间在抓详情时以详情接口返回为准
		for id := range want {
			all = append(all, listItem{DocID: id})
			log.Printf("[crawler] 文档ID %s 不在频道列表中，将直接按 ID 抓详情", id)
		}
		log.Printf("[crawler] 按 -ids 过滤后共 %d 篇待处理", len(all))
	}
	if *maxDocs > 0 && len(all) > *maxDocs {
		all = all[:*maxDocs]
	}

	// 3. 逐篇抓详情并落盘
	var saved, ocrSaved, skipped, ocrSkipped, failed int
	for i, it := range all {
		if have[it.DocID] {
			skipped++
			continue
		}
		detail, err := fetchInfo(client, it.DocID)
		if err != nil {
			log.Printf("[crawler] (%d/%d) 详情失败 %s: %v", i+1, len(all), it.DocID, err)
			failed++
			time.Sleep(infoDelay)
			continue
		}

		origBody := htmlToMarkdown(detail.Content)
		body := origBody
		fromOCR := false

		// 正文无有效文字时：有图走视觉转写，无图则是简短文字公告，直接保留
		if meaningfulRunes(body) < 100 {
			imgURLs := extractImageURLs(detail.Content)

			if len(imgURLs) == 0 {
				// 无图：纯文字短公告（如"安全组件升级""服务器异常"），
				// 哪怕只有几十字也有检索价值；只有完全空白才丢弃
				if meaningfulRunes(body) < 15 {
					ocrSkipped++
					log.Printf("[crawler] (%d/%d) 空白内容：%s", i+1, len(all), detail.Title)
					time.Sleep(infoDelay)
					continue
				}
			} else {
				if !*ocr {
					skipped++
					log.Printf("[crawler] (%d/%d) 跳过图片公告（加 -ocr 可转写）：%s", i+1, len(all), detail.Title)
					time.Sleep(infoDelay)
					continue
				}
				if isBanListTitle(detail.Title) {
					ocrSkipped++
					log.Printf("[crawler] (%d/%d) 跳过处罚名单海报（无检索价值）：%s", i+1, len(all), detail.Title)
					time.Sleep(infoDelay)
					continue
				}
				text, oerr := vision.transcribe(detail.Content)
				if oerr != nil {
					log.Printf("[crawler] (%d/%d) OCR 失败 %s: %v", i+1, len(all), it.DocID, oerr)
					failed++
					time.Sleep(infoDelay)
					continue
				}
				body = cleanOCRText(text, detail.Title)
				if meaningfulRunes(body) < 100 {
					// 图片没认出文字：若原文带少量说明文字则保留原文，否则丢弃
					if meaningfulRunes(origBody) >= 15 {
						body = origBody
					} else {
						ocrSkipped++
						log.Printf("[crawler] (%d/%d) OCR 后仍无有效文字（装饰封面）：%s", i+1, len(all), detail.Title)
						time.Sleep(infoDelay)
						continue
					}
				}
				fromOCR = true
			}
		}

		fname, err := renderDoc(*outDir, *chanID, detail.Title, detail.IdxTime, it.DocID, body)
		if err != nil {
			log.Printf("[crawler] (%d/%d) 写入失败 %s: %v", i+1, len(all), it.DocID, err)
			failed++
		} else if fromOCR {
			ocrSaved++
			log.Printf("[crawler] (%d/%d) 已OCR转写: %s", i+1, len(all), filepath.Base(fname))
		} else {
			saved++
			log.Printf("[crawler] (%d/%d) 已保存: %s", i+1, len(all), filepath.Base(fname))
		}
		time.Sleep(infoDelay)
	}
	log.Printf("[crawler] 完成：文字 %d 篇，OCR转写 %d 篇，跳过 %d 篇（其中无价值海报 %d），失败 %d 篇",
		saved, ocrSaved, skipped, ocrSkipped, failed)
	if failed > 0 {
		log.Printf("[crawler] 有失败项，重跑一次本命令即可增量补齐（已成功的不会重复抓）")
	}
}

// ---------- 接口调用 ----------

// postIDE 以 x-www-form-urlencoded POST 调用 AMS ide 接口，带退避重试。
func postIDE(client *http.Client, form url.Values, out any) error {
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		req, err := http.NewRequest(http.MethodPost, ideURL, strings.NewReader(form.Encode()))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Referer", "https://df.qq.com/cp/a20240906main/newslist.html")
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) "+
			"AppleWebKit/537.36 (KHTML, like Gecko) Chrome/138.0.0.0 Safari/537.36")

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
		} else {
			body, rerr := io.ReadAll(resp.Body)
			resp.Body.Close()
			if rerr != nil {
				lastErr = rerr
			} else if resp.StatusCode != 200 {
				lastErr = fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(body), 120))
			} else if jerr := json.Unmarshal(body, out); jerr != nil {
				lastErr = fmt.Errorf("解析响应失败: %w; 原文: %s", jerr, truncate(string(body), 120))
			} else {
				return nil
			}
		}
		if attempt < 3 {
			time.Sleep(time.Duration(attempt*3) * time.Second)
		}
	}
	return lastErr
}

func fetchList(client *http.Client, chanID string, start int) ([]listItem, int, error) {
	form := url.Values{}
	form.Set("iChartId", listChart)
	form.Set("iSubChartId", listChart)
	form.Set("sIdeToken", listToken)
	form.Set("chanid", chanID)
	form.Set("typeids", "1")
	form.Set("limit", fmt.Sprint(pageSize))
	form.Set("start", fmt.Sprint(start))

	var r listResp
	if err := postIDE(client, form, &r); err != nil {
		return nil, 0, err
	}
	if r.IRet != 0 {
		return nil, 0, fmt.Errorf("接口返回 iRet=%d: %s", r.IRet, r.SMsg)
	}
	return r.JData.Data.Items, r.JData.Data.Total, nil
}

type article struct {
	Title   string
	IdxTime string
	Content string // HTML
}

func fetchInfo(client *http.Client, docid string) (*article, error) {
	form := url.Values{}
	form.Set("iChartId", infoChart)
	form.Set("iSubChartId", infoChart)
	form.Set("sIdeToken", infoToken)
	form.Set("id", docid)

	var r infoResp
	if err := postIDE(client, form, &r); err != nil {
		return nil, err
	}
	if r.IRet != 0 {
		return nil, fmt.Errorf("iRet=%d: %s", r.IRet, r.SMsg)
	}
	if len(r.JData.Data) == 0 {
		return nil, fmt.Errorf("详情为空")
	}
	d := r.JData.Data[0]
	return &article{Title: d.Title, IdxTime: d.IdxTime, Content: d.Content}, nil
}

// ---------- 落盘 ----------

var (
	// 头部里的文档ID标记，也是增量去重的依据
	docIDRe   = regexp.MustCompile(`文档ID：([0-9]+)`)
	badCharRe = regexp.MustCompile(`[\\/:*?"<>|\r\n\t]+`)
)

// renderDoc 把已转成 Markdown 的正文写入 data/knowledge/。
// body 由 htmlToMarkdown（文字公告）或视觉转写（图片公告）提供。
func renderDoc(outDir, chanID, title, idxTime, docid, body string) (string, error) {
	date := strings.ReplaceAll(strings.Split(idxTime, " ")[0], "-", "")
	titlePart := badCharRe.ReplaceAllString(strings.TrimSpace(title), "_")
	titlePart = trimRunes(titlePart, 40)
	base := fmt.Sprintf("官方公告-%s-%s", date, titlePart)

	name := base + ".md"
	for n := 2; ; n++ {
		if _, err := os.Stat(filepath.Join(outDir, name)); os.IsNotExist(err) {
			break
		}
		name = fmt.Sprintf("%s-%d.md", base, n)
	}
	path := filepath.Join(outDir, name)

	md := fmt.Sprintf("# %s\n\n"+
		"> 官方公告 | 分类：%s | 发布时间：%s | 文档ID：%s\n"+
		"> 来源：https://df.qq.com/cp/a20240906main/newsdetail.html?id=%s\n\n"+
		"%s\n",
		strings.TrimSpace(title), chanNames[chanID], idxTime, docid, docid, body)

	if err := os.WriteFile(path, []byte(md), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// scanExistingDocs 扫描目录下所有 .md 头部的"文档ID：xxx"，返回已存在ID集合
func scanExistingDocs(dir string) (map[string]bool, error) {
	have := map[string]bool{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		m := docIDRe.FindSubmatch(b)
		if m != nil {
			have[string(m[1])] = true
		}
	}
	return have, nil
}

// ---------- 小工具 ----------

func meaningfulRunes(s string) int {
	n := 0
	for _, r := range s {
		if r >= 0x4e00 && r <= 0x9fff { // 汉字
			n++
		} else if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			n++
		}
	}
	return n
}

func trimRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return strings.TrimSpace(string(r[:max]))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
