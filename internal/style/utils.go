package style

import (
	"math"
	"strings"

	"github.com/sigpanic/goink/internal/text"
)

const PreviewMaxRunes = 120

// ComputeStats 对多段素材文本进行确定性统计。
func ComputeStats(samples []Sample) Stats {
	var combined strings.Builder
	for _, sample := range samples {
		combined.WriteString(sample.Content)
		combined.WriteString("\n")
	}
	content := combined.String()

	stats := text.ComputeStats(content)

	// 句子长度分布
	sentences := splitSentences(content)
	total := len(sentences)
	if total == 0 {
		return Stats{}
	}
	short, mid, long := 0, 0, 0
	totalLen := 0
	lens := make([]int, len(sentences))
	for i, sent := range sentences {
		l := len([]rune(sent))
		lens[i] = l
		totalLen += l
		if l < 15 {
			short++
		} else if l <= 30 {
			mid++
		} else {
			long++
		}
	}

	// 标点密度
	chars := len([]rune(content))
	if chars == 0 {
		chars = 1
	}
	commas := strings.Count(content, "，") + strings.Count(content, ",")
	periods := strings.Count(content, "。") + strings.Count(content, ".")
	exclaims := strings.Count(content, "！") + strings.Count(content, "!")
	questions := strings.Count(content, "？") + strings.Count(content, "?")
	quotes := strings.Count(content, "「") + strings.Count(content, "」") +
		strings.Count(content, "\"") + strings.Count(content, "\u201c") + strings.Count(content, "\u201d")

	// 段落统计
	paragraphs := strings.Split(strings.TrimSpace(content), "\n\n")
	paraCount := 0
	paraTotalLen := 0
	for _, p := range paragraphs {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		paraCount++
		paraTotalLen += len([]rune(p))
	}
	avgParaLen := 0.0
	if paraCount > 0 {
		avgParaLen = float64(paraTotalLen) / float64(paraCount)
	}

	avgSentLen := float64(totalLen) / float64(total)
	varSentLen := 0.0
	for _, l := range lens {
		diff := float64(l) - avgSentLen
		varSentLen += diff * diff
	}
	sentLenStdDev := math.Sqrt(varSentLen / float64(total))

	return Stats{
		TotalChars:      chars,
		TotalWords:      stats.WordCount,
		SentenceCount:   total,
		ShortSentPct:    float64(short) * 100 / float64(total),
		MidSentPct:      float64(mid) * 100 / float64(total),
		LongSentPct:     float64(long) * 100 / float64(total),
		AvgSentLen:      avgSentLen,
		SentLenStdDev:   sentLenStdDev,
		CommaDensity:    float64(commas) * 100 / float64(chars),
		PeriodDensity:   float64(periods) * 100 / float64(chars),
		ExclaimDensity:  float64(exclaims) * 100 / float64(chars),
		QuestionDensity: float64(questions) * 100 / float64(chars),
		QuoteDensity:    float64(quotes) * 100 / float64(chars),
		ParagraphCount:  paraCount,
		AvgParaLen:      avgParaLen,
	}
}

// TruncatePreview 截取前 PreviewMaxRunes 个字符作为预览文本。
func TruncatePreview(content string) string {
	runes := []rune(content)
	if len(runes) <= PreviewMaxRunes {
		return content
	}
	return string(runes[:PreviewMaxRunes]) + "…"
}

func splitSentences(content string) []string {
	var sentences []string
	var b strings.Builder
	for _, r := range content {
		b.WriteRune(r)
		if r == '。' || r == '！' || r == '？' || r == '\n' || r == '.' || r == '!' || r == '?' {
			s := strings.TrimSpace(b.String())
			if s != "" {
				sentences = append(sentences, s)
			}
			b.Reset()
		}
	}
	s := strings.TrimSpace(b.String())
	if s != "" {
		sentences = append(sentences, s)
	}
	return sentences
}
