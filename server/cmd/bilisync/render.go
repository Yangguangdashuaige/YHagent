// cmd/bilisync/render.go
//
// 字幕 -> Markdown 的无依赖转换层：
//  1. 把逐行碎片字幕按"时间间隔/句末标点/长度上限"合并成自然段；
//  2. 用官方 AI 大纲的大章节（带秒级时间戳）把段落归入 ## 分节；
//  3. 每个自然段保留段首 [mm:ss] 时间戳锚点，回答时可定位原片位置；
//  4. 渲染成与 data/knowledge/ 现有文档（官方公告-*.md）风格一致的 Markdown，
//     直接交给 internal/knowledge/chunk.go 切片入库。
//
// 官方大纲的细要点（part_outline）稀疏且时间点滞后，不适合做硬切分边界，
// 因此只作为"参考要点"传给 LLM；### 小标题由 LLM 按字幕实际主题切分，
// 避免"威龙内容挂在风衣标题下"这类张冠李戴（见 structure.go）。
package main

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	// paraGapSeconds 相邻字幕行间隔超过该值视为自然段边界
	paraGapSeconds = 1.5
	// paraMaxRunes 自然段兜底长度上限，避免无标点长独白产出超大段落
	paraMaxRunes = 240
)

// Paragraph 合并后的自然段：Start 为段首字幕时间（秒）
type Paragraph struct {
	Start float64
	Text  string
}

// Block 一个大章节块：H2 标题 + 归属该节的段落。
// Hints 为官方大纲给的节内参考要点（稀疏、可能滞后，仅喂给 LLM 参考，不做硬边界）。
// Polished 为 LLM 润色后的 Markdown 正文（可含 ### 小标题）；为空时渲染原文。
type Block struct {
	H2       string
	Hints    []OutlinePoint
	Paras    []Paragraph
	Polished string
}

func (b *Block) rawText() string {
	var sb strings.Builder
	for i, p := range b.Paras {
		if i > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString("[" + fmtTime(p.Start) + "] ")
		sb.WriteString(p.Text)
	}
	return sb.String()
}

// MergeParagraphs 把碎片字幕合并成带时间戳的自然段
func MergeParagraphs(cues []Cue) []Paragraph {
	paras := make([]Paragraph, 0, len(cues)/4)
	var sb strings.Builder
	start := 0.0

	flush := func() {
		text := strings.TrimSpace(sb.String())
		if text != "" {
			paras = append(paras, Paragraph{Start: start, Text: text})
		}
		sb.Reset()
	}

	for i, cue := range cues {
		text := strings.TrimSpace(cue.Content)
		if text == "" {
			continue
		}
		if sb.Len() > 0 {
			gap := cue.From - cues[i-1].To
			last, _ := lastRuneOf(sb.String())
			switch {
			case gap > paraGapSeconds:
				flush()
			case isSentenceEnd(last):
				flush()
			case utf8.RuneCountInString(sb.String()) >= paraMaxRunes:
				flush()
			}
		}
		if sb.Len() == 0 {
			start = cue.From
		} else if needSpaceBetween(sb.String(), text) {
			sb.WriteByte(' ')
		}
		sb.WriteString(text)
	}
	flush()
	return paras
}

// BuildBlocks 用官方大纲的大章节时间戳把段落分节。
// 段落按段首时间归入 [本节起点, 下一节起点)；开场早于首个章节的段落并入首节。
// conclusion 为 nil（视频无 AI 总结）时退化为单一 "视频正文" 节。
func BuildBlocks(paras []Paragraph, concl *Conclusion) []Block {
	if concl == nil || len(concl.Outline) == 0 {
		return []Block{{H2: "视频正文", Paras: paras}}
	}

	blocks := make([]Block, 0, len(concl.Outline))
	for _, sec := range concl.Outline {
		var hints []OutlinePoint
		for _, p := range sec.PartOutline {
			if t := strings.TrimSpace(p.Content); t != "" {
				hints = append(hints, OutlinePoint{Timestamp: p.Timestamp, Content: t})
			}
		}
		blocks = append(blocks, Block{H2: strings.TrimSpace(sec.Title), Hints: hints})
	}

	idx := 0
	for _, p := range paras {

		// 找到该段落所属的最后一个"起点不晚于段首"的章节
		for idx+1 < len(blocks) && float64(concl.Outline[idx+1].Timestamp) <= p.Start {
			idx++
		}
		blocks[idx].Paras = append(blocks[idx].Paras, p)
	}

	// 极端情况下首节可能没分到内容（首个章节时间戳晚于全部段落），过滤空节
	out := blocks[:0]
	for _, b := range blocks {
		if len(b.Paras) > 0 {
			out = append(out, b)
		}
	}
	return out
}

// RenderMarkdown 生成最终知识文档
func RenderMarkdown(info *VideoInfo, page VideoPage, blocks []Block, summary, pageURL string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "# %s\n\n", strings.TrimSpace(info.Title))
	fmt.Fprintf(&sb, "> B站视频 | UP主：%s | 发布时间：%s | 时长：%s\n",
		info.Owner, info.PubDate.Format("2006-01-02"), fmtTime(float64(page.Duration)))
	fmt.Fprintf(&sb, "> 来源：%s\n", pageURL)
	fmt.Fprintf(&sb, "> BV号：%s | 分P：%d/%d（cid %d）\n",
		info.BVID, page.Page, len(info.Pages), page.CID)
	sb.WriteString("> 字幕来源：B站AI自动字幕，经分段与结构化整理；正文中 [mm:ss] 为原片时间戳\n\n")

	if summary != "" {
		sb.WriteString("## 内容摘要\n\n")
		sb.WriteString(strings.TrimSpace(summary))
		sb.WriteString("\n\n")
	}

	for _, b := range blocks {
		if len(b.Paras) == 0 {
			continue
		}
		sb.WriteString("## " + strings.TrimRight(b.H2, "。.") + "\n\n")
		body := b.Polished
		if strings.TrimSpace(body) == "" {
			body = b.rawText()
		}
		sb.WriteString(strings.TrimSpace(body))
		sb.WriteString("\n\n")
	}
	return sb.String()
}

var illegalFileChars = regexp.MustCompile(`[\\/:*?"<>|\r\n\t]+`)

// OutputFilename 视频-BV-标题.md；多 P 视频带 P 序号，重跑同名覆盖（幂等）
func OutputFilename(bvid string, pages int, page VideoPage, title string) string {
	name := strings.TrimSpace(title)
	name = illegalFileChars.ReplaceAllString(name, "_")
	name = strings.Trim(name, " .")
	name = truncateRunes(name, 40)
	if pages > 1 {
		part := truncateRunes(illegalFileChars.ReplaceAllString(strings.TrimSpace(page.Part), "_"), 24)
		if part == "" {
			part = fmt.Sprintf("P%d", page.Page)
		}
		return fmt.Sprintf("视频-%s-P%d-%s.md", bvid, page.Page, part)
	}
	return fmt.Sprintf("视频-%s-%s.md", bvid, name)
}

// ---- 小工具 ----

func isSentenceEnd(r rune) bool {
	switch r {
	case '。', '！', '？', '!', '?', '…', '；', ';':
		return true
	}
	return false
}

// needSpaceBetween 中英混排时仅在两侧都是 ASCII 字母/数字时补空格
func needSpaceBetween(prev, next string) bool {
	r1, _ := lastRuneOf(prev)
	r2, _ := utf8.DecodeRuneInString(next)
	return isASCIIAlnum(r1) && isASCIIAlnum(r2)
}

func isASCIIAlnum(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}

func lastRuneOf(s string) (rune, int) {
	return utf8.DecodeLastRuneInString(s)
}

// fmtTime 秒 -> mm:ss（超过 1 小时用 hh:mm:ss）
func fmtTime(sec float64) string {
	s := int(sec + 0.5)
	h := s / 3600
	m := (s % 3600) / 60
	s = s % 60
	if h > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%02d:%02d", m, s)
}

func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}
