package style

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sigpanic/goink/internal/llm"
	"github.com/sigpanic/goink/internal/mcp_tools"
	"github.com/sigpanic/goink/internal/skill"
)

// Extract 分析样本文字的写作风格，生成仿写 skill。
// 取消由调用方（app 层）通过 ctx 管理。
func Extract(ctx context.Context, llmClient *llm.Client,
	samples []Sample, providerName, modelID, reasoningEffort string) (*ExtractResult, error) {

	// 计算 stats + 拼接全文
	stats := ComputeStats(samples)
	var combined strings.Builder
	combined.WriteString(formatStatsForLLM(stats))
	for _, sample := range samples {
		combined.WriteString("\n\n---\n\n")
		combined.WriteString(sample.Content)
	}

	// 组装工具：output_style_skill 返回结构化的风格仿写技能
	tools := []map[string]any{{
		"type": "function",
		"function": map[string]any{
			"name":        "output_style_skill",
			"description": "返回风格仿写技能的结构化输出。",
			"parameters":  mcp_tools.SchemaOf(StyleSkillOutput{}),
		},
	}}

	opts := &llm.CallToolOptions{
		Attempts:  2,
		MaxTokens: 64000,
	}
	if reasoningEffort != "" {
		opts.ReasoningEffort = reasoningEffort
	}

	raw, err := llmClient.CallTool(ctx, providerName, modelID,
		buildExtractMessages(combined.String()), tools, "output_style_skill", opts)
	if err != nil {
		return nil, fmt.Errorf("LLM 调用失败: %w", err)
	}

	var out StyleSkillOutput
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("解析风格技能失败: %w", err)
	}
	if strings.TrimSpace(out.Name) == "" || strings.TrimSpace(out.Content) == "" {
		return nil, fmt.Errorf("LLM 返回的风格技能内容不完整")
	}

	safeName := skill.SanitizeFileName(out.Name)
	return &ExtractResult{
		Name:        out.Name,
		Description: out.Description,
		RawContent:  buildStyleSkillMarkdown(out.Name, out.Description, out.Content),
		FilePath:    fmt.Sprintf("~/.goink/skills/%s.md", safeName),
	}, nil
}

// buildStyleSkillMarkdown 组装完整的风格仿写 skill markdown：固定 frontmatter + 正文。
func buildStyleSkillMarkdown(name, description, content string) string {
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "name: %s\n", name)
	fmt.Fprintf(&b, "description: %s\n", description)
	b.WriteString("category: 风格仿写\n")
	b.WriteString("mode: auto\n")
	b.WriteString("author: ai\n")
	b.WriteString("version: 1\n")
	b.WriteString("---\n\n")
	b.WriteString(strings.TrimSpace(content))
	b.WriteString("\n")
	return b.String()
}

func buildExtractMessages(sample string) []map[string]any {
	return []map[string]any{
		{"role": "system", "content": extractSystemPrompt},
		{"role": "user", "content": fmt.Sprintf("文本开头为【文本统计信息】（确定性测量数据），之后为原文样本。请基于统计数据分析风格，并调用 output_style_skill 工具提交结果：\n\n%s", sample)},
	}
}

// formatStatsForLLM 将 Stats 格式化为中文统计文本供 LLM 参考。
func formatStatsForLLM(stats Stats) string {
	return fmt.Sprintf(`【文本统计信息】
总字数：%d  总字符数：%d
句子数：%d  平均句长：%.1f 字  句长标准差：%.1f
句长分布：短句（<15字）%.1f%%  中句（15-30字）%.1f%%  长句（>30字）%.1f%%
标点密度：逗号 %.2f%%  句号 %.2f%%  感叹号 %.2f%%  问号 %.2f%%  引号 %.2f%%
段落数：%d  平均段长：%.1f 字
`,
		stats.TotalWords, stats.TotalChars,
		stats.SentenceCount, stats.AvgSentLen, stats.SentLenStdDev,
		stats.ShortSentPct, stats.MidSentPct, stats.LongSentPct,
		stats.CommaDensity, stats.PeriodDensity, stats.ExclaimDensity, stats.QuestionDensity, stats.QuoteDensity,
		stats.ParagraphCount, stats.AvgParaLen)
}

const extractSystemPrompt = `你是一位专业的写作风格分析师。文本开头包含【文本统计信息】——句长分布、标准差、标点密度等均为代码确定性计算，请直接引用这些数值作为分析依据。

从以下六个维度拆解写作风格：

1. **句式特征**：结合统计数据中的句长分布和标准差，分析长短句搭配模式与节奏特征
2. **用词习惯**：词汇量级、口语/书面语倾向、高频词类型
3. **修辞手法**：常用修辞类型及其使用频率
4. **节奏控制**：结合标点密度数据分析段落组织与断句节奏
5. **叙事视角与距离**：人称选择、叙事者与内容的距离感
6. **氛围与语调**：情绪基调、语言温度

请调用 output_style_skill 工具提交分析结果，各字段要求：
- name：为这个风格起一个贴切的中文名称
- description：一句话描述适合什么写作场景，什么时候调用
- content：风格仿写技能正文 markdown（不含 frontmatter），包含以下章节：
# {风格名称}

## 风格概述
一句话概括整体特点。

## 句式特征
基于统计数据的句长分布和标准差，分析句式特点。

## 用词习惯
详细分析用词偏好、词汇选择倾向。

## 修辞手法
详细分析使用的修辞手法及其特点。

## 节奏控制
结合标点密度数据分析段落组织、断句节奏。

## 叙事视角与距离
详细分析叙事者位置、与内容的距离感。

## 氛围与语调
详细分析情绪基调、语言温度。

## 仿写要点
提炼 3-5 条可操作的仿写指导原则，每一条应具体、可执行。

## 原文锚点
从原文中挑选 4-6 个最能体现该风格的片段作为仿写范例。每个片段 80-150 字，逐字引用原文，标注该片段示范了什么技法。格式：

### 锚点 N
> [原文片段]

示范：[一句话说明这段体现了什么技法、仿写时如何运用]`
