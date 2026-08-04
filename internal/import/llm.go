package imp

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// GenerateTextFunc 是调用 LLM 生成文本的函数签名。
// import 包通过此接口解耦对 llm 包的直接依赖。
type GenerateTextFunc func(ctx context.Context, providerName string, messages []map[string]any, model string) (string, error)

// chapterAnalysisPrompt 是 LLM 分析章节格式的系统提示。
const chapterAnalysisPrompt = `你是一个小说章节格式识别专家。用户将提供一部小说的前 150 行文本，请分析其中的章节标题格式，并输出一个 Go 正则表达式用于匹配所有章节标题行。

要求：
1. 只输出正则字符串本身，不要添加任何解释、代码块标记或引号
2. 使用 (?m) 开头启用多行模式
3. 使用 ^ 匹配行首
4. 正则应匹配章节标题行的行首部分
5. 示例输出：(?m)^(?:#{1,6}\s+)?(?:[ 　\t]*)第[零〇一二三四五六七八九十百千两\d]+章
6. 如果无法识别任何章节格式，输出 NONE

常见章节格式：
- "第X章 标题"、"第X卷 标题"（中文数字或阿拉伯数字）
- "序章"、"楔子"、"引子"、"番外"、"终章"、"后记"
- "Chapter N"、"Part N"
- 纯数字行（如单独一行的 "1"、"2"）
- "卷X"、"第X节"、"第X回"
- 特殊符号开头的章节标记（如 "☆"、"★"、"◆" 等）`

// AnalyzeWithLLM 使用 LLM 分析小说文件的章节格式。
// 读取文件前 150 行，让 LLM 生成正则，编译后全文匹配分割。
// ctx 用于取消 LLM 调用（如用户关闭窗口）。
func AnalyzeWithLLM(ctx context.Context, filePath string, providerName string, modelID string, generateFn GenerateTextFunc) (*Result, error) {
	// 1. 读取文件 + 解码
	content, err := readFileContent(filePath)
	if err != nil {
		return nil, err
	}

	// 2. 取前 150 行
	lines := strings.Split(content, "\n")
	if len(lines) > 150 {
		lines = lines[:150]
	}
	preview := strings.Join(lines, "\n")

	// 3. 调用 LLM
	messages := []map[string]any{
		{"role": "system", "content": chapterAnalysisPrompt},
		{"role": "user", "content": preview},
	}

	llmOutput, err := generateFn(ctx, providerName, messages, modelID)
	if err != nil {
		return nil, fmt.Errorf("AI 分析失败: %w", err)
	}

	// 4. 清理 LLM 输出
	llmPattern := cleanLLMPatternOutput(llmOutput)
	if llmPattern == "" || llmPattern == "NONE" {
		return nil, fmt.Errorf("AI 无法识别章节格式")
	}

	// 5. 用 LLM 正则分割全文
	result, err := ParseWithLLMPattern(filePath, llmPattern)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// cleanLLMPatternOutput 清理 LLM 输出的正则字符串，
// 去除可能的 markdown 代码块标记、引号等，并确保以 (?m) 多行模式开头。
func cleanLLMPatternOutput(output string) string {
	s := strings.TrimSpace(output)

	// 去除 markdown 代码块
	if strings.HasPrefix(s, "```") {
		firstNewline := strings.Index(s, "\n")
		if firstNewline >= 0 {
			s = s[firstNewline+1:]
		}
		s = strings.TrimSuffix(strings.TrimSpace(s), "```")
		s = strings.TrimSpace(s)
	}

	// 去除前后引号
	s = strings.Trim(s, "`\"'")
	s = strings.TrimSpace(s)

	// 确保以 (?m) 多行模式开头，使 ^ 匹配每行首而非全文首
	if !strings.HasPrefix(s, "(?m)") {
		s = "(?m)" + s
	}
	return s
}

// ParseWithLLMPattern 使用 LLM 返回的正则表达式对全文进行章节分割。
// llmPattern 是 LLM 根据前 150 行分析后输出的 Go 正则字符串。
// 如果编译失败、匹配数不足或分割结果不合理，返回 error。
func ParseWithLLMPattern(filePath string, llmPattern string) (*Result, error) {
	content, err := readFileContent(filePath)
	if err != nil {
		return nil, err
	}

	title := inferTitle(filePath)

	// 1. 编译 LLM 返回的正则
	re, err := regexp.Compile(llmPattern)
	if err != nil {
		return nil, fmt.Errorf("LLM 正则编译失败: %w", err)
	}

	// 2. 全文匹配
	allIdx := re.FindAllStringIndex(content, -1)
	var candidates []matchLine
	seen := make(map[int]bool)
	for _, loc := range allIdx {
		// 找到匹配所在完整行的范围
		lineStart := loc[0]
		lineEnd := loc[1]
		for lineStart > 0 && content[lineStart-1] != '\n' {
			lineStart--
		}
		for lineEnd < len(content) && content[lineEnd-1] != '\n' {
			lineEnd++
		}
		line := strings.TrimRight(content[lineStart:lineEnd], "\n")

		if seen[lineStart] {
			continue
		}
		if len([]rune(line)) > maxChapterTitleLen {
			continue
		}
		seen[lineStart] = true
		candidates = append(candidates, matchLine{start: lineStart, line: line})
	}

	// 3. 匹配数不足
	if len(candidates) <= 1 {
		return nil, fmt.Errorf("LLM 正则仅匹配到 %d 个章节标题，无法有效分割", len(candidates))
	}

	// 4. 间距验证
	candidates = filterByGap(candidates, content)
	if len(candidates) <= 1 {
		return nil, fmt.Errorf("LLM 正则间距过滤后仅剩 %d 个匹配点，无法有效分割", len(candidates))
	}

	// 5. 分割
	chapters := splitByPositions(content, candidates)
	if len(chapters) == 0 {
		return nil, fmt.Errorf("LLM 正则分割未能生成章节")
	}

	// 6. 文件大小估算验证
	totalChars := utf8.RuneCountInString(content)
	if !isReasonableChapterCount(len(chapters), totalChars) {
		return nil, fmt.Errorf("LLM 正则分割结果不合理：找到 %d 章，但文件约 %d 字，估算应约 %d 章",
			len(chapters), totalChars, totalChars/3000)
	}

	return &Result{
		Title:    title,
		Chapters: chapters,
	}, nil
}
