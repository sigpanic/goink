package pattern

import (
	"encoding/json"
	"fmt"
	"strings"
)

func boundaryMessages(chapters []ChapterSource) []map[string]any {
	var b strings.Builder
	for _, ch := range chapters {
		title := strings.TrimSpace(ch.Title)
		if title == "" {
			title = fmt.Sprintf("第%d章（无标题）", ch.ChapterNumber)
		}
		fmt.Fprintf(&b, "第%d章：%s\n", ch.ChapterNumber, title)
	}
	return []map[string]any{
		{"role": "system", "content": "你是一个长篇小说结构分析师。根据章节标题推断可能的叙事阶段边界。只能通过调用 output_boundary_hints 返回结果。"},
		{"role": "user", "content": "请找出可能的叙事阶段边界。边界是参考线索，不是最终定论。每条提示保持简短。\n\n" + b.String()},
	}
}

func summaryMessages(chapters []ChapterSource, boundaries []BoundaryHint) []map[string]any {
	var b strings.Builder
	if len(boundaries) > 0 {
		b.WriteString("疑似阶段边界参考：\n")
		for _, h := range boundaries {
			fmt.Fprintf(&b, "- 第%d-%d章：%s\n", h.StartChapter, h.EndChapter, h.Hint)
		}
		b.WriteString("\n")
	}
	b.WriteString("章节列表：\n")
	for _, ch := range chapters {
		title := strings.TrimSpace(ch.Title)
		if title == "" {
			title = fmt.Sprintf("第%d章（无标题）", ch.ChapterNumber)
		}
		fmt.Fprintf(&b, "\n## 第%d章：%s\n", ch.ChapterNumber, title)
		if strings.TrimSpace(ch.Summary) != "" {
			fmt.Fprintf(&b, "[上下文参考，无需重新生成]\n%s\n", strings.TrimSpace(ch.Summary))
			continue
		}
		if strings.TrimSpace(ch.Content) == "" {
			b.WriteString("[需提取]\n无内容。\n")
			continue
		}
		fmt.Fprintf(&b, "[需提取]\n%s\n", ch.Content)
	}
	return []map[string]any{
		{"role": "system", "content": "你为叙事模式提取生成章节摘要。标记为[需提取]的章节需要各生成一条 80-150 字的叙事摘要，覆盖核心事件、关键人物行为与决策、情感或关系转折。保留足够信息量以供后续判断叙事阶段边界。标记为[上下文参考]的章节仅作叙事背景参考，除非同时标记了[需提取]，否则不要包含在输出中。只能通过调用 output_chapter_summaries 返回结果。"},
		{"role": "user", "content": b.String()},
	}
}

func initialChunkMessages(summaries []ChapterSummaryItem) []map[string]any {
	return []map[string]any{
		{"role": "system", "content": "你将章节级摘要压缩为叙事阶段块（chunk）。以叙事阶段转折为分界标准：当摘要显示叙事方向、情感基调或核心冲突发生明显变化时，应在该处划分边界。相邻且属于同一叙事阶段的章节应合并为一个块。每个块的 content 应概括该阶段的核心事件、关键人物行动与转折，约 100-200 字。只能通过调用 output_chunks 返回结果。"},
		{"role": "user", "content": "根据以下章节摘要生成第一轮叙事阶段块：\n\n" + marshalPretty(summaries)},
	}
}

func compressChunkMessages(chunks []Chunk, round int) []map[string]any {
	return []map[string]any{
		{"role": "system", "content": "你将多个叙事阶段块合并为更少、更大的阶段块。规则如下：\n1. 优先合并相邻且属于同一叙事阶段的块（叙事方向一致、情感基调相同）。\n2. 若一个块的内部包含两个明显不同的叙事子阶段（如铺垫→爆发），则可以拆分，但应尽量减少拆分。\n3. 每个输出块的 content 应重新概括该合并后阶段的核心事件与转折，约 100-200 字。\n4. 必须保留准确的 start_chapter 和 end_chapter。只能通过调用 output_chunks 返回结果。"},
		{"role": "user", "content": fmt.Sprintf("第 %d 轮压缩。将以下阶段块合并为更少的叙事阶段块：\n\n%s", round, marshalPretty(chunks))},
	}
}

func finalSkillMessages(chunks []Chunk) []map[string]any {
	return []map[string]any{
		{"role": "system", "content": finalSkillSystemPrompt},
		{"role": "user", "content": "根据以下叙事阶段块生成最终的可复用叙事模式技能：\n\n" + marshalPretty(chunks)},
	}
}

func marshalPretty(v any) string {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(raw)
}

const finalSkillSystemPrompt = `你是一个专业的小说结构分析师。

根据提供的叙事阶段块，创建一个可复用的叙事模式技能（套路模板）。不要仅仅描述原书内容，而要抽象出可复用的模板。

只能通过调用 output_skill 工具返回结果，工具参数要求：
- name：中文模式名称（如"逆袭打脸流"）。
- description：一句话描述何时使用此叙事模式。
- content：技能正文 markdown，不含 frontmatter（frontmatter 由系统自动生成）。必须包含以下章节：

  ## 套路概览
  总结可复用的叙事结构。
  ## 阶段拆解
  列出每个阶段，包含源文章节范围、结构功能、常见信号以及如何复用。
  ## 爽点节奏
  说明压制、释放、揭示、挫折和高潮的分布规律。
  ## 角色功能模板
  抽象主角、盟友、对手、反派、导师、工具角色以及牺牲/对比角色的功能定位。
  ## 可复用叙事规律
  给出可以指导新作品创作的具体写作规则。
  ## 使用注意
  说明哪些地方必须改编，避免成品显得雷同。

再次强调：content 中应抽象出可复用模板，不要仅仅描述原书内容；content 中不要包含 YAML frontmatter。`
