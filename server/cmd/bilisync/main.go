// cmd/bilisync/main.go
//
// B 站视频 -> 静态知识库 Markdown 的离线同步工具（只在你自己的开发机上运行，
// 不部署到服务器），与 cmd/crawler、cmd/itemsync 同级：
//
//  1. 登录态（BILI_SESSDATA）调 B 站接口：视频信息、AI 字幕（ai-zh 带时间轴）、
//     官方 AI 总结（摘要 + 秒级章节大纲）；
//  2. 碎片字幕按停顿/句读合并成自然段，按官方大纲归入大章节，保留 [mm:ss]；
//  3. 默认调豆包把每节口语稿润色成可检索书面语（-raw 可跳过，零 API 费用）；
//  4. 平铺写入 data/knowledge/视频-{BV}-{标题}.md，随后跑 cmd/indexer 增量建库。
//
// 用法（在 server/ 目录下）：
//
//	go run ./cmd/bilisync BV1kBfVYhEAs                       # 单个视频（全分P）
//	go run ./cmd/bilisync BVaaa BVbbb https://.../BVccc      # 批量，逐个同步
//	go run ./cmd/bilisync BVxxxx -raw                        # 不调豆包，只做时间戳分段
//	go run ./cmd/bilisync BVaaa BVbbb -scan                  # 只探测标题/时长/字幕轨
//
// 环境变量（server/.env）：
//
//	BILI_SESSDATA  必填，浏览器登录 B 站后的 Cookie 值（HttpOnly，从开发者工具
//	               Application -> Cookies 复制）；AI 字幕必须登录态可见
//	ARK_API_KEY    润色步骤需要（-raw / -scan 模式不需要）
package main

import (
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"yhagent-server/internal/ai"
	"yhagent-server/internal/config"
)

var bvidRe = regexp.MustCompile(`BV[0-9A-Za-z]{10}`)

// target 一个待同步目标：BV 号 + 可选分 P 序号（0 表示全部分 P）
type target struct {
	bvid     string
	onlyPage int
}

type pageResult struct {
	bvid  string
	title string
	page  int
	file  string
	err   error
}

func main() {
	videoURL := flag.String("url", "", "B 站视频链接或 BV 号（也可直接给多个位置参数）")
	outDir := flag.String("out", "data/knowledge", "Markdown 输出目录")
	rawOnly := flag.Bool("raw", false, "只做字幕分段，不调用豆包润色（零 API 费用）")
	scanOnly := flag.Bool("scan", false, "只探测视频标题/时长/分P/字幕轨，不下载不润色")
	flag.Parse()

	args := flag.Args()
	if *videoURL != "" {
		args = append([]string{*videoURL}, args...)
	}
	targets := parseTargets(args)
	if len(targets) == 0 {
		log.Fatal("未从参数中解析到 BV 号，用法：go run ./cmd/bilisync BV1xxx BV2yyy ...（也支持视频链接，可带 ?p=N）")
	}
	log.Printf("[bilisync] 共 %d 个待处理视频（scan=%v raw=%v）", len(targets), *scanOnly, *rawOnly)

	if err := config.Load(); err != nil {
		log.Fatalf("配置加载失败: %v", err)
	}
	sessdata := os.Getenv("BILI_SESSDATA")
	if sessdata == "" {
		log.Fatal("缺少 BILI_SESSDATA：请在 server/.env 配置浏览器登录 B 站后的 SESSDATA Cookie")
	}
	var llm *ai.SeedClient
	if !*rawOnly && !*scanOnly {
		if config.AppConfig.DoubaoAPIKey == "" {
			log.Fatal("缺少 ARK_API_KEY（字幕润色需要）；如只想分段请加 -raw，探测请加 -scan")
		}
		llm = ai.NewSeedClient()
	}

	cli := NewBiliClient(sessdata)
	if err := cli.Warmup(); err != nil {
		log.Fatalf("初始化 B 站会话失败: %v", err)
	}

	var results []pageResult
	batchStart := time.Now()
	for i, t := range targets {
		log.Printf("[bilisync] ===== [%d/%d] %s 开始 =====", i+1, len(targets), t.bvid)
		vStart := time.Now()
		results = append(results, syncOneVideo(cli, llm, t, *outDir, *rawOnly, *scanOnly)...)
		log.Printf("[bilisync] ===== [%d/%d] %s 结束，耗时 %s =====",
			i+1, len(targets), t.bvid, time.Since(vStart).Round(time.Second))
	}

	summary(results, len(targets), *scanOnly)
	if !*scanOnly {
		log.Printf("[bilisync] 全部完成，总耗时 %s；记得执行 go run ./cmd/indexer 增量更新向量索引",
			time.Since(batchStart).Round(time.Second))
	}
}

// parseTargets 从命令行参数提取去重后的 BV 目标（保留 ?p=N）
func parseTargets(args []string) []target {
	seen := map[string]bool{}
	var out []target
	for _, a := range args {
		bvid := bvidRe.FindString(a)
		if bvid == "" || seen[bvid] {
			continue
		}
		seen[bvid] = true
		onlyPage := 0
		if u, err := url.Parse(a); err == nil && u.RawQuery != "" {
			if p := u.Query().Get("p"); p != "" {
				onlyPage, _ = strconv.Atoi(p)
			}
		}
		out = append(out, target{bvid: bvid, onlyPage: onlyPage})
	}
	return out
}

// syncOneVideo 同步单个视频的全部分 P（或指定 P），返回每个 P 的结果
func syncOneVideo(cli *BiliClient, llm *ai.SeedClient, t target, outDir string, rawOnly, scanOnly bool) []pageResult {
	info, err := cli.View(t.bvid)
	if err != nil {
		return []pageResult{{bvid: t.bvid, err: fmt.Errorf("获取视频信息失败: %w", err)}}
	}
	log.Printf("[bilisync] 《%s》 UP=%s 分P=%d 时长=%s 发布=%s",
		info.Title, info.Owner, len(info.Pages), fmtDur(info.Pages), info.PubDate.Format("2006-01-02"))

	pages := info.Pages
	if t.onlyPage > 0 {
		var picked []VideoPage
		for _, p := range pages {
			if p.Page == t.onlyPage {
				picked = append(picked, p)
			}
		}
		if len(picked) == 0 {
			return []pageResult{{bvid: t.bvid, title: info.Title, err: fmt.Errorf("视频没有第 %d P", t.onlyPage)}}
		}
		pages = picked
	}

	var results []pageResult
	for _, page := range pages {
		if scanOnly {
			err := scanOnePage(cli, info, page)
			results = append(results, pageResult{
				bvid: t.bvid, title: info.Title, page: page.Page,
				err: err,
			})
			continue
		}
		file, err := syncOnePage(cli, llm, info, page, outDir, rawOnly)
		results = append(results, pageResult{
			bvid: t.bvid, title: info.Title, page: page.Page,
			file: file, err: err,
		})
	}
	return results
}

// scanOnePage 仅探测字幕轨可用性，不下载字幕、不调 LLM
func scanOnePage(cli *BiliClient, info *VideoInfo, page VideoPage) error {
	tracks, err := cli.SubtitleTracks(info.BVID, page.CID)
	if err != nil {
		return fmt.Errorf("取字幕轨: %w", err)
	}
	track := PickSubtitleTrack(tracks)
	if track == nil {
		return fmt.Errorf("P%d 无中文字幕轨（UP未传字幕且无AI字幕），需走 ASR 路线", page.Page)
	}
	log.Printf("[bilisync][scan] P%d 时长=%s 可用字幕轨：%s（lan=%s）",
		page.Page, fmtTime(float64(page.Duration)), track.LanDoc, track.Lan)
	return nil
}

func syncOnePage(cli *BiliClient, llm *ai.SeedClient, info *VideoInfo, page VideoPage, outDir string, rawOnly bool) (string, error) {
	tracks, err := cli.SubtitleTracks(info.BVID, page.CID)
	if err != nil {
		return "", fmt.Errorf("取字幕轨: %w", err)
	}
	track := PickSubtitleTrack(tracks)
	if track == nil {
		return "", fmt.Errorf("P%d 没有任何中文字幕轨（UP未传字幕且无AI字幕），需改走 ASR 路线", page.Page)
	}
	cues, err := cli.DownloadSubtitle(track.SubURL)
	if err != nil {
		return "", fmt.Errorf("下载字幕: %w", err)
	}
	if len(cues) == 0 {
		return "", fmt.Errorf("P%d 字幕内容为空", page.Page)
	}
	log.Printf("[bilisync] P%d 命中字幕轨：%s（%d 行）", page.Page, track.LanDoc, len(cues))

	// AI 总结是增强项：拿不到就按无章节降级，不影响主流程
	concl, err := cli.Conclusion(info.BVID, page.CID, info.UpMid)
	if err != nil {
		log.Printf("[bilisync] P%d AI 大纲获取失败，按无章节降级: %v", page.Page, err)
	}

	paras := MergeParagraphs(cues)
	blocks := BuildBlocks(paras, concl)
	log.Printf("[bilisync] P%d 合并为 %d 个自然段、%d 个章节块", page.Page, len(paras), len(blocks))

	summary := ""
	if concl != nil {
		summary = concl.Summary
	}
	if !rawOnly {
		n, err := StructureBlocks(llm, info.Title, blocks)
		if err != nil {
			log.Printf("[bilisync] 润色流程异常: %v", err)
		}
		log.Printf("[bilisync] P%d 豆包结构化完成：%d/%d 节", page.Page, n, len(blocks))
	}

	pageURL := fmt.Sprintf("https://www.bilibili.com/video/%s", info.BVID)
	if len(info.Pages) > 1 {
		pageURL += fmt.Sprintf("?p=%d", page.Page)
	}
	md := RenderMarkdown(info, page, blocks, summary, pageURL)
	name := OutputFilename(info.BVID, len(info.Pages), page, info.Title)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", fmt.Errorf("创建输出目录: %w", err)
	}
	path := filepath.Join(outDir, name)
	if err := os.WriteFile(path, []byte(md), 0o644); err != nil {
		return "", fmt.Errorf("写入文件: %w", err)
	}
	log.Printf("[bilisync] P%d 已写入 %s（%d 字符）", page.Page, path, len([]rune(md)))
	return path, nil
}

// summary 批量任务收尾汇总：成功/失败清单一目了然
func summary(results []pageResult, nVideos int, scanOnly bool) {
	mode := "同步"
	if scanOnly {
		mode = "探测"
	}
	ok, fail := 0, 0
	for _, r := range results {
		if r.err != nil {
			fail++
			log.Printf("[bilisync][%s失败] %s P%d 《%s》: %v", mode, r.bvid, r.page, r.title, r.err)
		} else {
			ok++
		}
	}
	log.Printf("[bilisync] %s汇总：%d 个视频，%d 个分P成功，%d 个失败", mode, nVideos, ok, fail)
}

// fmtDur 多 P 视频给总时长，单 P 时在调用处直接用 fmtTime，这里仅汇总用
func fmtDur(pages []VideoPage) string {
	total := 0
	for _, p := range pages {
		total += p.Duration
	}
	return fmtTime(float64(total))
}
