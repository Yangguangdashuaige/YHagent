// cmd/bilisync/structure.go
//
// 调豆包对"节内逐字稿"做结构化润色：
// 大章节（##）骨架来自 B 站官方 AI 大纲（可信、不重排），LLM 在每个 ## 块内
// 按字幕实际主题切分 ### 小标题（通常一个干员/一条流言一个），把口语 ASR
// 文本整理成带时间戳锚点、可检索的书面知识。官方细要点只作参考传入——
// 其时间点稀疏且滞后，硬切分会导致面包屑张冠李戴。
//
// 长视频按自然段切批（structureMaxInput 字/批），失败自动重试一次，
// 单节最终失败时降级为原始合并稿，不阻断整个同步。
package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"
	"unicode/utf8"

	"yhagent-server/internal/ai"

	"github.com/volcengine/ark-runtime-go/arkruntime/model/chat"
)

const (
	structureMaxInput = 5000 // 单批逐字稿字符上限（约 3000 汉字，远低于对话窗口）
	// 思考型模型（如 doubao-seed 思考档）在长指令下首字延迟可达数分钟，
	// 这是离线批处理工具，给足 10 分钟；正文实际流式输出通常只需几秒。
	// 批量并发时服务端排队会进一步放大延迟，超时后单节降级为原始分段稿，
	// 可在低峰期对受影响视频重跑（输出文件同名覆盖）。
	structureTimeout = 600 * time.Second
)

const structureSystemPrompt = `你是资深游戏视频字幕编辑。用户会给你《三角洲行动》游戏视频某一章节的AI语音识别字幕，已按时间戳分段，格式为"[mm:ss] 文本"。
请把口语逐字稿整理成可检索的书面知识，严格遵守：
1. 删除口癖、重复语句、无意义填充词（嗯/啊/对吧/然后然后）以及与主题无关的闲聊；修正ASR错别字，尤其是干员名（深蓝、风衣、麦晓雯、威龙、乌鲁鲁、露娜、红狼等）与游戏术语。
2. 补全标点，合并破碎短句。按字幕实际主题切分三级小标题：每个独立主题（通常对应一个干员或一条流言鉴定）单独起一个 "### 小标题"，标题简短准确；主题内的并列要点用 Markdown 无序列表表达。不要输出一级或二级标题（# 或 ##）。
3. 必须原样保留所有 [mm:ss] 时间戳，放在对应句子或要点的最前面；不得新增、删除、改写或猜测时间戳。
4. 只能使用字幕中出现的事实、数字与结论（例如某条流言的"真/假/只对一半"鉴定结果及原因），严禁补充字幕里没有的信息。
5. 直接输出整理后的 Markdown，不要前言、解释或代码块包裹。`

// StructureBlocks 原地填充每个 Block 的 Polished 字段，返回成功润色的节数
func StructureBlocks(client *ai.SeedClient, videoTitle string, blocks []Block) (int, error) {
	polished := 0
	for i := range blocks {
		if len(blocks[i].Paras) == 0 {
			continue
		}
		batches := splitBatches(blocks[i].Paras, structureMaxInput)
		outs := make([]string, 0, len(batches))
		sectionStart := time.Now()
		log.Printf("[bilisync] 润色章节《%s》（%d 批，思考型模型首字可能等待数分钟，请勿中断）...",
			blocks[i].H2, len(batches))
		for bi, batch := range batches {
			// ### 小标题只允许在该节的第一批开头生成，续批不再重复切主题
			text, err := polishWithRetry(client, videoTitle, &blocks[i], batch, bi == 0)
			if err != nil {
				log.Printf("[bilisync] 章节《%s》润色失败，保留原始合并稿: %v", blocks[i].H2, err)
				outs = nil
				break
			}
			outs = append(outs, text)
		}
		if len(outs) > 0 {
			blocks[i].Polished = strings.Join(outs, "\n\n")
			polished++
			log.Printf("[bilisync] 章节《%s》润色完成，耗时 %s", blocks[i].H2, time.Since(sectionStart).Round(time.Second))
		}
	}
	return polished, nil
}

// splitBatches 按自然段累积，单批不超过 maxRunes 字（保证不把同一段拦腰切开）
func splitBatches(paras []Paragraph, maxRunes int) []string {
	var batches []string
	var sb strings.Builder
	flush := func() {
		if strings.TrimSpace(sb.String()) != "" {
			batches = append(batches, strings.TrimSpace(sb.String()))
		}
		sb.Reset()
	}
	for _, p := range paras {
		line := "[" + fmtTime(p.Start) + "] " + p.Text
		if utf8.RuneCountInString(sb.String())+utf8.RuneCountInString(line) > maxRunes && sb.Len() > 0 {
			flush()
		}
		sb.WriteString(line)
		sb.WriteString("\n\n")
	}
	flush()
	return batches
}

func polishWithRetry(client *ai.SeedClient, title string, block *Block, raw string, headingsAllowed bool) (string, error) {
	var lastErr error
	for attempt := 1; attempt <= 2; attempt++ {
		text, err := polishOnce(client, title, block, raw, headingsAllowed)
		if err == nil {
			return text, nil
		}
		lastErr = err
		if attempt == 1 {
			time.Sleep(3 * time.Second)
		}
	}
	return "", lastErr
}

func polishOnce(client *ai.SeedClient, title string, block *Block, raw string, headingsAllowed bool) (string, error) {
	var sbUser strings.Builder
	fmt.Fprintf(&sbUser, "视频标题：%s\n当前章节：%s\n", title, block.H2)
	if len(block.Hints) > 0 {
		sbUser.WriteString("官方给出的该章节参考要点（稀疏且可能有偏差，仅供参考，结构与结论必须以字幕实际内容为准）：\n")
		for _, h := range block.Hints {
			fmt.Fprintf(&sbUser, "- [%s] %s\n", fmtTime(float64(h.Timestamp)), h.Content)
		}
		sbUser.WriteString("\n")
	}
	if headingsAllowed {
		sbUser.WriteString("请整理下面的字幕（每行以[mm:ss]时间戳开头），按实际主题切分 ### 小标题：\n\n")
	} else {
		sbUser.WriteString("请继续整理下面的字幕（每行以[mm:ss]时间戳开头），直接承接上一批的主题，不要再输出 ### 小标题：\n\n")
	}
	sbUser.WriteString(strings.TrimSpace(raw))

	msgs := []chat.ChatCompletionRequestMessage{systemMessage(structureSystemPrompt), ai.NewUserMessage(sbUser.String())}
	var sb strings.Builder
	ctx, cancel := context.WithTimeout(context.Background(), structureTimeout)
	defer cancel()
	if err := client.ChatStream(ctx, msgs, nil, func(chunk string) {
		sb.WriteString(chunk)
	}, nil, nil); err != nil {
		return "", err
	}
	return cleanModelOutput(sb.String()), nil
}

// cleanModelOutput 防御性清理：去掉模型偶尔包上的代码块围栏与前后多余空白
func cleanModelOutput(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```markdown")
	s = strings.TrimPrefix(s, "```md")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}

// systemMessage 构造 system 消息（ai 包内的构造器未导出，这里按同一 SDK 方式组装）
func systemMessage(content string) chat.ChatCompletionRequestMessage {
	sum := chat.ChatCompletionRequestMessageSum{}
	sum.SetChatCompletionRequestSystemMessage(chat.ChatCompletionRequestSystemMessage{
		Role:    chat.ChatCompletionRequestSystemMessageRoleSystem,
		Content: chat.NewStringChatCompletionMessageContent(content),
	})
	return chat.ChatCompletionRequestMessage{OneOf: sum}
}
