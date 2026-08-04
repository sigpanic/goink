package imp

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
)

// chapterPatterns 定义多种章节检测正则，按优先级排列（平局时优先选排前的）。
var chapterPatterns = []struct {
	name    string
	pattern *regexp.Regexp
	loose   bool // loose 模式下在匹配后按行长度过滤
}{
	{
		name:    "strict_line_start",
		pattern: regexp.MustCompile(`(?m)^(?:#{1,6}\s+)?(?:[ 　\t]*)第[零〇一二三四五六七八九十百千两\d]+[章卷部回节]`),
	},
	{
		name:    "loose_inline",
		pattern: regexp.MustCompile(`第[零〇一二三四五六七八九十百千两\d]+[章卷部回节]`),
		loose:   true,
	},
	{
		name:    "special_markers",
		pattern: regexp.MustCompile(`(?m)^(?:#{1,6}\s+)?(?:[ 　\t]*)(?:序章|楔子|引子|番外[一二三四五六七八九十零\d]*|终章|后记|序言|序)(?:[ 　\t：:——\-—].*)?$`),
	},
	{
		name:    "english",
		pattern: regexp.MustCompile(`(?im)^Chapter\s+\d+`),
	},
	{
		name:    "juan_line_start",
		pattern: regexp.MustCompile(`(?m)^(?:#{1,6}\s+)?(?:[ 　\t]*)卷[零〇一二三四五六七八九十百千两\d]+`),
	},
	{
		name: "celestial_cycle_marker",
		// 兼容干支纪年式章节编目，常见于部分网络文学中以六十甲子序号替代数字的轮转章节标记。
		// 60 甲子全枚举可确保精确匹配，避免天干地支字符在正文语境中误触发。
		pattern: regexp.MustCompile(
			`(?m)^(?:#{1,6}\s+)?(?:[ 　\t])*(?:` +
				`甲子|乙丑|丙寅|丁卯|戊辰|己巳|庚午|辛未|壬申|癸酉|` +
				`甲戌|乙亥|丙子|丁丑|戊寅|己卯|庚辰|辛巳|壬午|癸未|` +
				`甲申|乙酉|丙戌|丁亥|戊子|己丑|庚寅|辛卯|壬辰|癸巳|` +
				`甲午|乙未|丙申|丁酉|戊戌|己亥|庚子|辛丑|壬寅|癸卯|` +
				`甲辰|乙巳|丙午|丁未|戊申|己酉|庚戌|辛亥|壬子|癸丑|` +
				`甲寅|乙卯|丙辰|丁巳|戊午|己未|庚申|辛酉|壬戌|癸亥` +
				`)章`),
	},
	{
		name:    "numeric_line",
		pattern: regexp.MustCompile(`(?m)^(?:#{1,6}\s+)?(?:[ 　\t]*)\d{1,5}$`),
	},
}

// maxChapterTitleLen 章节标题行最大字符数，超过此长度的视为正文引用而非章节头。
const maxChapterTitleLen = 30

// matchLine 记录一个章节标记匹配所在的行信息。
type matchLine struct {
	start int    // 在 content 中的字节偏移
	line  string // 匹配行文本
}

func parseTxt(filePath string) (*Result, error) {
	content, err := readFileContent(filePath)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(content) == "" {
		return nil, fmt.Errorf("文件内容为空")
	}

	title := inferTitle(filePath)

	// 对每个正则模式，找出所有匹配行并按行长度过滤
	type patternResult struct {
		idx     int
		matches []matchLine
	}
	var results []patternResult

	for pi, cp := range chapterPatterns {
		allIdx := cp.pattern.FindAllStringIndex(content, -1)
		var matches []matchLine
		seen := make(map[int]bool) // 按行起始位置去重，避免同一行多匹配产生重复分割点
		for _, loc := range allIdx {
			// 找到该匹配所在完整行的范围
			lineStart := loc[0]
			lineEnd := loc[1]
			// 向左找到行首
			for lineStart > 0 && content[lineStart-1] != '\n' {
				lineStart--
			}
			// 向右找到行尾
			for lineEnd < len(content) && content[lineEnd-1] != '\n' {
				lineEnd++
			}
			line := strings.TrimRight(content[lineStart:lineEnd], "\n")

			// 同一行去重
			if seen[lineStart] {
				continue
			}

			// 行长度过滤
			if len([]rune(line)) > maxChapterTitleLen {
				continue
			}
			seen[lineStart] = true
			matches = append(matches, matchLine{start: lineStart, line: line})
		}
		results = append(results, patternResult{idx: pi, matches: matches})
	}

	// 选择匹配数最多的模式；平局时优先选排前的（即 idx 更小的）
	// 加权规则：如果获胜模式是 loose_inline (idx=1)，检查所有非 loose 行首模式。
	// 当存在某个非 loose 模式 ≥2 匹配，且 loose 匹配数不超过其 1.5 倍时，
	// 优先选择该行首模式（行首匹配一定是真阳性，loose 可能是正文引用）。
	bestIdx := -1
	bestCount := 0
	// 先按匹配数选最优
	for _, r := range results {
		if len(r.matches) > bestCount || (len(r.matches) == bestCount && (bestIdx == -1 || r.idx < bestIdx)) {
			bestCount = len(r.matches)
			bestIdx = r.idx
		}
	}
	// 非 loose 模式优先提升
	// 当 loose_inline 获胜时，检查是否有行首模式的匹配数和 loose 接近，
	// 且 loose 的额外匹配数很少（不超过行首模式匹配数的 50%）。
	// 此时 loose 的额外匹配更可能是正文引用，应优先选行首模式。
	// 特别地，当 loose 和某个行首模式匹配数完全相同时（extra=0），
	// 说明 loose 的匹配和行首模式完全重叠，行首模式更可靠。
	if bestIdx == 1 {
		var looseCount int
		for _, lr := range results {
			if lr.idx == 1 {
				looseCount = len(lr.matches)
				break
			}
		}
		for _, r := range results {
			if r.idx == 1 || len(r.matches) < 2 {
				continue
			}
			extra := looseCount - len(r.matches)
			// loose 额外匹配不超过行首模式 50%，或完全重叠（extra=0）
			if extra >= 0 && extra <= len(r.matches)/2 {
				bestIdx = r.idx
				break
			}
		}
	}

	// 使用获胜模式的匹配位置进行章节分割
	candidates := results[bestIdx].matches

	// 合并其他行首模式（非 loose）的匹配点
	// 当文件中存在多种章节格式时（如"序章" + "第N章"），统计选择只会选一种，
	// 但其他格式的匹配也可能是真实章节头，需要合并进来。
	for _, r := range results {
		if r.idx == bestIdx || chapterPatterns[r.idx].loose {
			continue
		}
		candidateStarts := make(map[int]bool)
		for _, c := range candidates {
			candidateStarts[c.start] = true
		}
		for _, m := range r.matches {
			if !candidateStarts[m.start] {
				candidates = append(candidates, m)
			}
		}
	}
	// 按位置排序（合并后需要重新排序）
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].start < candidates[j].start
	})

	// 间距验证：过滤掉间距过小的假阳性匹配
	candidates = filterByGap(candidates, content)

	// 统一按匹配点切分：candidates==0 返回空切片，==1 切 1 章（首个匹配点之前的内容按前言丢弃，与多匹配点行为一致），>=2 正常切分
	chapters := splitByPositions(content, candidates)

	// 最后统一 guard：章节数与文件大小不匹配（含 0 章）→ 提示前端走 LLM
	totalChars := utf8.RuneCountInString(content)
	if !isReasonableChapterCount(len(chapters), totalChars) {
		return &Result{
			Title:    title,
			Chapters: chapters,
			NeedsLLM: true,
		}, nil
	}

	return &Result{
		Title:    title,
		Chapters: chapters,
	}, nil
}

// readFileContent 读取文件并解码为 UTF-8 字符串，统一换行符为 \n。
func readFileContent(filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("读取文件失败: %w", err)
	}
	content, err := DecodeText(data)
	if err != nil {
		return "", err
	}
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	return content, nil
}

// DecodeText 将字节数据解码为 UTF-8 字符串，支持 GB18030 回退。
func DecodeText(data []byte) (string, error) {
	data = trimUTF8BOM(data)
	if utf8.Valid(data) {
		return string(data), nil
	}
	decoded, err := simplifiedchinese.GB18030.NewDecoder().Bytes(data)
	if err != nil {
		return "", fmt.Errorf("文本编码不是 UTF-8，按 GB18030 解码也失败: %w", err)
	}
	return string(decoded), nil
}

func trimUTF8BOM(data []byte) []byte {
	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		return data[3:]
	}
	return data
}

func inferTitle(filePath string) string {
	name := filePath
	// 去掉路径
	if idx := strings.LastIndex(name, "/"); idx >= 0 {
		name = name[idx+1:]
	}
	if idx := strings.LastIndex(name, "\\"); idx >= 0 {
		name = name[idx+1:]
	}
	// 去掉扩展名
	if idx := strings.LastIndex(name, "."); idx >= 0 {
		name = name[:idx]
	}
	// 去掉常见后缀
	name = strings.TrimSuffix(name, "_")

	if name == "" {
		name = "未命名"
	}
	return name
}

// filterByGap 根据匹配点之间的间距过滤假阳性。
// 正文章节之间的间距通常较大且均匀，而正文引用"第X章"往往紧跟在章节标题之后，
// 间距远小于正常章节间距，通过中位数间距的 10% 作为阈值来过滤。
func filterByGap(matches []matchLine, content string) []matchLine {
	if len(matches) <= 2 {
		return matches
	}
	// 计算相邻匹配之间的字符间距（rune 数）
	gaps := make([]int, len(matches)-1)
	for i := 0; i < len(matches)-1; i++ {
		gaps[i] = utf8.RuneCountInString(content[matches[i].start:matches[i+1].start])
	}
	// 取前 10 个间距的中位数
	sorted := make([]int, len(gaps))
	copy(sorted, gaps)
	sort.Ints(sorted)
	n := len(sorted)
	if n > 10 {
		n = 10
	}
	medianGap := sorted[n/2]

	// 如果中位数间距太小（<200），说明章节很短或文件很短，
	// 此时间距验证不可靠，直接返回原匹配列表
	if medianGap < 200 {
		return matches
	}

	// 过滤：与前一个匹配间距 < 中位数 10% 的视为假阳性
	threshold := medianGap / 10
	if threshold < 50 {
		threshold = 50 // 最小阈值 50 字符
	}

	var filtered []matchLine
	filtered = append(filtered, matches[0]) // 始终保留第一个
	for i := 1; i < len(matches); i++ {
		gap := utf8.RuneCountInString(content[matches[i-1].start:matches[i].start])
		if gap >= threshold {
			filtered = append(filtered, matches[i])
		}
	}
	return filtered
}

// isReasonableChapterCount 检验章节数是否合理。
// 中文网络小说平均每章 2000-5000 字，取 3000 字估算，
// 若实际章节数不足估算的 10%，说明分割可能失败。
func isReasonableChapterCount(chapters int, totalChars int) bool {
	if chapters == 0 {
		return false // 0 章永远不合理（识别不出章节格式）
	}
	if totalChars < 3000 {
		return true // 文件太短，无法估算
	}
	estimated := totalChars / 3000
	if estimated < 1 {
		estimated = 1
	}
	// 实际章节数不足估算的 10%，说明分割有问题
	return chapters >= estimated/10
}

// stripChapterPrefix 去掉标题中的章节号前缀，避免与前端渲染的"第N章"重复。
// 例如："第1章 开始" → "开始"，"第一章：开端" → "开端"，
// "Chapter 1: The Beginning" → "The Beginning"，
// "序章 黎明" → "黎明"，"楔子 风起" → "风起"。
// 如果去掉前缀后为空，则保留原标题。
var chapterPrefixRe = regexp.MustCompile(
	`^(?:第[零〇一二三四五六七八九十百千两\d]+[章卷部回节]|` +
		`Chapter\s+\d+|` +
		`序章|楔子|引子|番外[一二三四五六七八九十零\d]*|终章|后记|序言|序)` +
		`[\s：:——\-—]*`)

func stripChapterPrefix(title string) string {
	stripped := chapterPrefixRe.ReplaceAllString(title, "")
	stripped = strings.TrimSpace(stripped)
	if stripped == "" {
		return title // 去掉前缀后为空（如"第一章"无副标题），保留原文
	}
	return stripped
}

// splitByPositions 根据位置列表分割 content 为章节。
// positions 中每个元素代表一个章节的起始位置。
// 提取标题时去除 Markdown 标记和章节号前缀。
func splitByPositions(content string, positions []matchLine) []Chapter {
	var chapters []Chapter
	for i, c := range positions {
		var end int
		if i+1 < len(positions) {
			end = positions[i+1].start
		} else {
			end = len(content)
		}

		chapContent := strings.TrimSpace(content[c.start:end])

		// 提取标题：取第一行，去除 Markdown 标记
		lines := strings.SplitN(chapContent, "\n", 2)
		titleLine := strings.TrimSpace(lines[0])
		for strings.HasPrefix(titleLine, "#") {
			titleLine = strings.TrimPrefix(titleLine, "#")
			titleLine = strings.TrimSpace(titleLine)
		}
		chapTitle := titleLine

		// 去掉章节号前缀，避免与前端渲染的"第N章"重复
		chapTitle = stripChapterPrefix(chapTitle)

		if chapTitle == "" {
			chapTitle = fmt.Sprintf("第%d章", i+1)
		}

		chapters = append(chapters, Chapter{
			Title:   chapTitle,
			Content: chapContent,
		})
	}
	return chapters
}
