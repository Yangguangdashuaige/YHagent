// internal/knowledge/chunk.go
//
// 静态知识库的 Markdown 切片器（零三方依赖）。
// 设计目标：游戏攻略类文档（标题分节、段落、表格、列表）切成
// 几百字一片、带标题面包屑的独立知识片段，每片可单独向量化与检索。
package knowledge

import (
	"bufio"
	"strings"
)

// 切片参数：中文语境按 rune 计数。单片约 600 字，兼顾检索精度与上下文完整度。
const (
	maxChunkRunes = 600 // 单片正文目标上限
	overlapRunes  = 120 // 超长段落硬切时的首尾重叠，避免句子被拦腰截断后语义丢失
)

// Chunk 是一个知识切片。Content 为最终送入向量化模型和大模型的完整文本（含标题前缀）。
type Chunk struct {
	ID      int    `json:"id"`      // 全局序号，与向量下标一一对应
	Source  string `json:"source"`  // 来源文件名，回答时可标注出处
	Title   string `json:"title"`   // 标题面包屑，如 "武器攻略 > M14 > 压枪技巧"
	Content string `json:"content"` // 标题 + 正文
}

// EmbedInput 返回该切片送入向量化模型的输入文本：面包屑的**最深小节名**
// 单独一行 + 原始 Content（Content 开头本就带【完整面包屑】前缀）。
//
// 只加重"末段小节名"而不是整条面包屑的原因（title2x 全标题加权实测翻车）：
// 面包屑大头是文档名前缀，整条加权会让同文档所有小节的向量互相拉近，
// 顶层摘要片（标题=纯文档名）反而失去区分度（R053/R042 回归）；而末段
// 小节名是每个切片**独有**的内容标识（如「可选择行动及难度」），加重它：
//   - "能选什么难度和行动"类查询贴住答案小节（R098）；
//   - 摘要片末段=文档名（含日期/赛季名），日期查询区分度提高（R053/R042）。
//
// indexer（离线建库）与 ExtraStore.AddDocument（运行时入库）必须使用同一函数，
// 保证主索引与补充索引的向量分布一致。
func EmbedInput(c Chunk) string {
	leaf := EmbedTitleLeaf(c.Title)
	if leaf == "" {
		return c.Content
	}
	return leaf + "\n" + c.Content
}

// EmbedTitleLeaf 取面包屑最后一段（最深小节名）；无分隔符时返回整条标题。
// 用于向量化输入加权（见 EmbedInput）与 BM25 之外的轻量标题信号。
func EmbedTitleLeaf(title string) string {
	t := strings.TrimSpace(title)
	if t == "" {
		return ""
	}
	if i := strings.LastIndex(t, ">"); i >= 0 {
		return strings.TrimSpace(t[i+1:])
	}
	return t
}

// ChunkMarkdown 把一篇 Markdown 文档切成若干 Chunk。
// source 通常是文件名；startID 为本文件起始编号（多文件连续编号）。
func ChunkMarkdown(source, markdown string, startID int) []Chunk {
	sections := splitSections(markdown)
	chunks := make([]Chunk, 0, len(sections)*2)
	id := startID

	for _, sec := range sections {
		bodies := packSection(sec.body)
		for _, body := range bodies {
			body = strings.TrimSpace(body)
			if body == "" {
				continue
			}
			title := strings.TrimSpace(strings.Join(sec.path, " > "))
			full := body
			if title != "" {
				full = "【" + title + "】\n" + body
			}
			chunks = append(chunks, Chunk{
				ID:      id,
				Source:  source,
				Title:   title,
				Content: full,
			})
			id++
		}
	}
	return chunks
}

// section 是一个标题分节：标题路径 + 该节正文行
type section struct {
	path []string
	body []string
}

// splitSections 按 ATX 标题（#/##/###...）切分文档并维护标题面包屑。
// 文档开头第一个标题之前的内容归入空路径节（通常没有）。
func splitSections(markdown string) []section {
	scanner := bufio.NewScanner(strings.NewReader(markdown))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var sections []section
	cur := section{}
	// 面包屑栈：depth -> 标题文本
	crumb := make(map[int]string)
	maxDepth := 0

	flush := func() {
		if len(cur.body) > 0 {
			path := orderedPath(crumb, maxDepth)
			sections = append(sections, section{path: path, body: cur.body})
			cur.body = nil
		}
	}

	for scanner.Scan() {
		line := scanner.Text()
		level := headerLevel(line)
		if level > 0 {
			flush()
			title := strings.TrimSpace(strings.TrimLeft(line, "# "))
			crumb[level] = title
			// 进入同级或更高级标题后，清掉更深层的面包屑
			for d := level + 1; d <= maxDepth; d++ {
				delete(crumb, d)
			}
			if level > maxDepth {
				maxDepth = level
			}
			// 复制当前路径，供新节使用
			cur = section{path: orderedPath(crumb, maxDepth)}
			continue
		}
		cur.body = append(cur.body, line)
	}
	flush()
	return sections
}

// orderedPath 按层级顺序输出标题面包屑
func orderedPath(crumb map[int]string, maxDepth int) []string {
	path := make([]string, 0, maxDepth)
	for d := 1; d <= maxDepth; d++ {
		if t, ok := crumb[d]; ok && t != "" {
			path = append(path, t)
		}
	}
	return path
}

// headerLevel 判断行是否为 ATX 标题，返回层级 1~6，否则 0
func headerLevel(line string) int {
	trimmed := strings.TrimLeft(line, " \t")
	if !strings.HasPrefix(trimmed, "#") {
		return 0
	}
	n := 0
	for n < len(trimmed) && trimmed[n] == '#' {
		n++
	}
	if n > 6 || n >= len(trimmed) || trimmed[n] != ' ' {
		return 0
	}
	return n
}

// packSection 把一节的正文行打包成不超过目标长度的若干块：
// 表格、代码块、列表组作为不可拆"原子块"，普通段落可硬切并带重叠。
func packSection(lines []string) []string {
	blocks := extractBlocks(lines)

	var result []string
	var buf strings.Builder

	flushBuf := func() {
		if strings.TrimSpace(buf.String()) != "" {
			result = append(result, buf.String())
			buf.Reset()
		}
	}

	for _, block := range blocks {
		blockRunes := []rune(block)
		// 原子块（表格/代码/列表）整体超限时，独立成片，不与缓冲合并
		if len(blockRunes) > maxChunkRunes {
			flushBuf()
			for _, piece := range hardSplit(block) {
				result = append(result, piece)
			}
			continue
		}
		if len([]rune(buf.String()))+len(blockRunes)+2 > maxChunkRunes {
			flushBuf()
		}
		if buf.Len() > 0 {
			buf.WriteString("\n\n")
		}
		buf.WriteString(block)
	}
	flushBuf()
	return result
}

// extractBlocks 把正文行聚合成块：
//   - 连续的表格行（| 开头）为一个块
//   - 围栏代码块 ``` 整体为一个块
//   - 连续的列表行为一个块
//   - 空行分隔的普通段落为一个块
func extractBlocks(lines []string) []string {
	var blocks []string
	var para []string
	inCode := false
	var code []string

	flushPara := func() {
		if len(para) > 0 {
			blocks = append(blocks, strings.Join(para, "\n"))
			para = nil
		}
	}

	i := 0
	for i < len(lines) {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		// 围栏代码块：从 ``` 到下一个 ``` 整体保留
		if strings.HasPrefix(trimmed, "```") {
			flushPara()
			if !inCode {
				inCode = true
				code = []string{line}
			} else {
				code = append(code, line)
				blocks = append(blocks, strings.Join(code, "\n"))
				code = nil
				inCode = false
			}
			i++
			continue
		}
		if inCode {
			code = append(code, line)
			i++
			continue
		}

		// 表格：连续 | 开头行（含分隔行 |---|---|）
		if strings.HasPrefix(trimmed, "|") {
			flushPara()
			var table []string
			for i < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i]), "|") {
				table = append(table, lines[i])
				i++
			}
			blocks = append(blocks, strings.Join(table, "\n"))
			continue
		}

		// 列表：连续的 - / * / 数字. 开头行
		if isListStart(trimmed) {
			flushPara()
			var list []string
			for i < len(lines) {
				t := strings.TrimSpace(lines[i])
				if isListStart(t) || (t != "" && strings.HasPrefix(lines[i], "  ")) {
					list = append(list, lines[i])
					i++
					continue
				}
				break
			}
			blocks = append(blocks, strings.Join(list, "\n"))
			continue
		}

		// 普通内容：按空行分段
		if trimmed == "" {
			flushPara()
			i++
			continue
		}
		para = append(para, line)
		i++
	}
	flushPara()
	if inCode && len(code) > 0 {
		blocks = append(blocks, strings.Join(code, "\n"))
	}
	return blocks
}

// isListStart 判断是否为无序列表（-、*）或有序列表（1.）起始
func isListStart(s string) bool {
	if strings.HasPrefix(s, "- ") || strings.HasPrefix(s, "* ") || strings.HasPrefix(s, "+ ") {
		return true
	}
	if len(s) >= 2 && s[0] >= '0' && s[0] <= '9' {
		rest := strings.TrimLeft(s, "0123456789")
		return strings.HasPrefix(rest, ". ")
	}
	return false
}

// hardSplit 把超长单块（主要是长段落）按 rune 切开，相邻片段保留 overlapRunes 重叠
func hardSplit(s string) []string {
	runes := []rune(s)
	if len(runes) <= maxChunkRunes {
		return []string{s}
	}
	var pieces []string
	start := 0
	for start < len(runes) {
		end := start + maxChunkRunes
		if end > len(runes) {
			end = len(runes)
		}
		pieces = append(pieces, string(runes[start:end]))
		if end >= len(runes) {
			break
		}
		start = end - overlapRunes
	}
	return pieces
}
