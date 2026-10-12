// cmd/crawler/html2md.go
//
// 面向官网公告 CMS 正文的极简 HTML -> Markdown 转换器（零三方依赖）。
// 公告正文的标签集合很固定：p / span / strong / br / ul / li / img，
// 历史长公告偶发标题与表格，这里一并支持。
//
// 转换规则与 internal/knowledge/chunk.go 的切片策略对齐：
//   - 正文小标题（h1~h6、独立成行的【...】）转成 ##/###，切片器会把它们
//     纳入标题面包屑，显著提升检索精度；
//   - 表格转 Markdown 表格（切片器把整张表当作不可拆原子块）；
//   - 图片直接丢弃（图片无法向量化，纯图片海报由上层按有效字数过滤）。
package main

import (
	"html"
	"regexp"
	"strings"
)

var (
	tokenRe       = regexp.MustCompile(`(?is)<!--.*?-->|<!DOCTYPE[^>]*>|<[^>]+>|[^<]+`)
	tagRe         = regexp.MustCompile(`^<\s*(/?)([a-zA-Z0-9]+)[^>]*?(/?)\s*>$`)
	spaceRe       = regexp.MustCompile(`[ \t\x{00a0}]+`)
	bracketHeadRe = regexp.MustCompile(`^\**【\s*(.+?)\s*】\**$`)
)

// htmlToMarkdown 把公告正文 HTML 转成 Markdown 文本。
func htmlToMarkdown(src string) string {
	b := &mdBuilder{}
	skipTag := "" // 命中 <script>/<style> 时跳过到对应闭合标签

	for _, tok := range tokenRe.FindAllString(src, -1) {
		if strings.HasPrefix(tok, "<") {
			if strings.HasPrefix(tok, "<!--") || strings.HasPrefix(strings.ToLower(tok), "<!doctype") {
				continue
			}
			m := tagRe.FindStringSubmatch(tok)
			if m == nil {
				continue
			}
			closing, name, selfClose := m[1] == "/", strings.ToLower(m[2]), m[3] == "/"
			if skipTag != "" {
				if closing && name == skipTag {
					skipTag = ""
				}
				continue
			}
			if closing {
				b.closeTag(name)
				continue
			}
			if name == "script" || name == "style" {
				skipTag = name
				continue
			}
			b.openTag(name)
			if selfClose && name == "br" {
				b.lineBreak()
			}
		} else if skipTag == "" {
			b.text(tok)
		}
	}
	b.flushBlock("")
	return b.finalize()
}

type listState struct {
	ordered bool
	n       int
}

type mdBuilder struct {
	lines []string
	cur   strings.Builder

	list []listState

	// 表格状态
	inTable bool
	rows    [][]string
	row     []string
	cell    strings.Builder
	inCell  bool

	// 当前块是否为标题块（由 h1~h6 触发）
	heading int
}

func (b *mdBuilder) openTag(name string) {
	switch name {
	case "br":
		b.lineBreak()
	case "p", "div":
		// 块开始，无特殊动作；闭合时换行
	case "h1", "h2":
		b.heading = 2
	case "h3":
		b.heading = 3
	case "h4", "h5", "h6":
		b.heading = 4
	case "strong", "b":
		b.write("**")
	case "em", "i":
		b.write("*")
	case "ul", "ol":
		b.flushBlock("")
		b.list = append(b.list, listState{ordered: name == "ol"})
	case "li":
		b.flushBlock("li")
		st := &b.list[len(b.list)-1]
		st.n++
		indent := strings.Repeat("  ", len(b.list)-1)
		if st.ordered {
			b.write(indent + itoa(st.n) + ". ")
		} else {
			b.write(indent + "- ")
		}
	case "table":
		b.flushBlock("")
		b.inTable = true
		b.rows = nil
	case "tr":
		b.row = nil
	case "td", "th":
		b.inCell = true
		b.cell.Reset()
	case "img":
		// 图片丢弃
	case "a", "span", "font", "section", "article", "figure", "figcaption":
		// 透明内联/容器标签，保留内部文本
	default:
		// 未识别标签按透明处理
	}
}

func (b *mdBuilder) closeTag(name string) {
	switch name {
	case "p", "div":
		b.flushBlock("")
	case "h1", "h2", "h3", "h4", "h5", "h6":
		b.flushBlock("heading")
		b.heading = 0
	case "strong", "b":
		b.write("**")
	case "em", "i":
		b.write("*")
	case "li":
		b.flushBlock("li")
	case "ul", "ol":
		b.flushBlock("")
		if len(b.list) > 0 {
			b.list = b.list[:len(b.list)-1]
		}
	case "td", "th":
		if b.inCell {
			b.row = append(b.row, cleanInline(b.cell.String()))
			b.inCell = false
		}
	case "tr":
		if len(b.row) > 0 {
			b.rows = append(b.rows, b.row)
		}
		b.row = nil
	case "table":
		b.renderTable()
		b.inTable = false
	}
}

// text 追加文本节点（表格单元格内写入 cell，其余写入当前行）
func (b *mdBuilder) text(s string) {
	s = html.UnescapeString(s)
	s = strings.ReplaceAll(s, "\u00a0", " ")
	if b.inCell {
		b.cell.WriteString(s)
		return
	}
	b.write(s)
}

func (b *mdBuilder) write(s string) { b.cur.WriteString(s) }

func (b *mdBuilder) lineBreak() {
	if b.inCell {
		b.cell.WriteString(" ")
		return
	}
	b.flushBlock("li") // 列表内换行不丢前缀层级，普通段落就是拆成两行
}

// flushBlock 结束当前行/块。kind: ""=普通段落, "heading"=标题, "li"=列表项内换行
func (b *mdBuilder) flushBlock(kind string) {
	line := cleanInline(b.cur.String())
	b.cur.Reset()
	if line == "" {
		if kind == "" && len(b.lines) > 0 && b.lines[len(b.lines)-1] != "" {
			b.lines = append(b.lines, "")
		}
		return
	}

	// 独立成行的【小标题】提升为 ## 标题（切片器面包屑依赖标题分节）
	if b.heading == 0 && kind != "li" {
		if m := bracketHeadRe.FindStringSubmatch(line); m != nil {
			b.lines = append(b.lines, "## "+m[1])
			return
		}
	}

	if b.heading > 0 {
		b.lines = append(b.lines, strings.Repeat("#", b.heading)+" "+line)
		return
	}
	b.lines = append(b.lines, line)
}

func (b *mdBuilder) renderTable() {
	if len(b.rows) == 0 {
		return
	}
	width := 0
	for _, r := range b.rows {
		if len(r) > width {
			width = len(r)
		}
	}
	norm := func(r []string) []string {
		for len(r) < width {
			r = append(r, "")
		}
		return r
	}
	header := norm(b.rows[0])
	b.lines = append(b.lines, "| "+strings.Join(header, " | ")+" |")
	b.lines = append(b.lines, "|"+strings.Repeat(" --- |", width))
	for _, r := range b.rows[1:] {
		r = norm(r)
		b.lines = append(b.lines, "| "+strings.Join(r, " | ")+" |")
	}
	b.lines = append(b.lines, "")
}

// finalize 汇总行并做空白归一化
func (b *mdBuilder) finalize() string {
	var out []string
	blank := false
	for _, l := range b.lines {
		l = strings.TrimRight(l, " \t")
		if l == "" {
			if blank || len(out) == 0 {
				continue
			}
			blank = true
		} else {
			blank = false
		}
		out = append(out, l)
	}
	s := strings.Join(out, "\n")
	s = strings.ReplaceAll(s, "****", "")
	return strings.TrimSpace(s) + "\n"
}

// cleanInline 清洗一行内的空白与常见噪声
func cleanInline(s string) string {
	s = html.UnescapeString(s)
	s = strings.ReplaceAll(s, "\u00a0", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	s = spaceRe.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
