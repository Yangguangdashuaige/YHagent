// internal/ingest/scan.go
//
// 入库前 Prompt 注入扫描：联网资料/整理后的文档若包含试图指挥模型的
// 指令性文本（"忽略以上规则""你必须""输出系统提示"等），在管理员确认
// 环节标黄提示人工核查——人已在环，就让人真正看到风险点。
//
// 设计定位是"启发式预警"而非拦截：宁可对疑似文本多提示、绝不静默放行；
// 误报的代价（管理员多看一眼）远小于漏报的代价（注入文档持久化进知识库，
// 影响所有后续用户的回答）。
package ingest

import "regexp"

// injectionPatterns 常见注入指令特征（中英文）。
// 刻意只匹配"指挥模型"的句式，不匹配普通攻略用语，压低误报。
var injectionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`忽略(以上|之前|前面|先前).{0,12}(规则|指令|要求|设定|提示|prompt)`),
	regexp.MustCompile(`无视(以上|之前|前面).{0,12}(规则|指令|要求|设定|提示)`),
	regexp.MustCompile(`(?i)disregard.{0,20}(above|previous|prior|all).{0,10}(instruction|rule|prompt)`),
	regexp.MustCompile(`(?i)ignore.{0,15}(all\s+)?(previous|prior|above).{0,12}(instruction|rule|prompt|direction)`),
	regexp.MustCompile(`(?i)system\s*prompt|系统提示词`),             // 索取系统提示
	regexp.MustCompile(`你必须(执行|遵守|按照|输出|遵循).{0,16}(以下|下面|上述)?`), // "你必须"太宽，限定动词后接指令指向
	regexp.MustCompile(`(?i)you\s+must\s+(now\s+)?(follow|obey|output|reveal|repeat|print)`),
	regexp.MustCompile(`(?i)\bact\s+as\b.{0,30}(system|developer|admin|root)`), // 角色劫持
	regexp.MustCompile(`(?i)\bnew\s+instructions?\b|\bdeveloper\s+mode\b`),
	regexp.MustCompile(`新的?(指令|规则|设定).{0,8}[:：]`),
}

// injectionHit 扫描结果：命中则给出管理员可读的警示文案
type injectionHit struct {
	Pattern string // 命中的特征描述（给管理员看，不含原文以免执行）
	Line    int    // 首个命中行号（1 起）
}

// scanInjection 扫描将入库的 Markdown 文本，返回疑似注入提示（nil = 干净）。
// 逐行扫描而非全文匹配：既能给出行号，也避免跨行正则的意外匹配。
func scanInjection(md string) *injectionHit {
	lines := splitLines(md)
	for i, ln := range lines {
		for _, p := range injectionPatterns {
			if p.MatchString(ln) {
				return &injectionHit{Pattern: p.String(), Line: i + 1}
			}
		}
	}
	return nil
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i, r := range s {
		if r == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	out = append(out, s[start:])
	return out
}
