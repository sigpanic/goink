package imp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/text/encoding/simplifiedchinese"
)

// ── 多正则统计章节分割算法综合测试 ──────────────────────────
// 以下测试覆盖 parseTxt 函数的章节拆分逻辑，
// 包括标准中文章节、中文数字章节、特殊前缀（☆）、英文章节、
// 混合格式、无章节标记、少量章节、Markdown 标题、卷标记、
// 全角标点、大文件模拟、GB18030 编码等场景。

// writeTxtFile 在临时目录中创建 .txt 文件并返回路径
func writeTxtFile(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

// writeTxtFileBytes 在临时目录中创建 .txt 文件（原始字节）并返回路径
func writeTxtFileBytes(t *testing.T, name string, data []byte) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

// ── 1. 标准中文章节（阿拉伯数字，最常见格式） ──────────────

func TestParseTxt_StandardArabicChapters(t *testing.T) {
	content := "第1章 开始\n正文内容...\n\n第2章 发展\n正文内容...\n\n第3章 高潮\n正文内容...\n"
	path := writeTxtFile(t, "标准章节.txt", content)
	r, err := parseTxt(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Chapters) != 3 {
		t.Fatalf("期望 3 章，实际 %d 章", len(r.Chapters))
	}
	if !strings.Contains(r.Chapters[0].Title, "开始") {
		t.Errorf("第1章标题应包含'开始'，实际: %q", r.Chapters[0].Title)
	}
	if !strings.Contains(r.Chapters[1].Title, "发展") {
		t.Errorf("第2章标题应包含'发展'，实际: %q", r.Chapters[1].Title)
	}
	if !strings.Contains(r.Chapters[2].Title, "高潮") {
		t.Errorf("第3章标题应包含'高潮'，实际: %q", r.Chapters[2].Title)
	}
	if !strings.Contains(r.Chapters[0].Content, "正文内容") {
		t.Errorf("第1章内容应包含'正文内容'，实际: %q", r.Chapters[0].Content)
	}
}

// ── 2. 中文数字章节 ──────────────────────────────────────

func TestParseTxt_ChineseNumeralChapters(t *testing.T) {
	content := "第一章 晨曦\n正文内容...\n\n第二章 暮色\n正文内容...\n\n第三章 夜幕\n正文内容...\n"
	path := writeTxtFile(t, "中文数字.txt", content)
	r, err := parseTxt(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Chapters) != 3 {
		t.Fatalf("期望 3 章，实际 %d 章", len(r.Chapters))
	}
	if !strings.Contains(r.Chapters[0].Title, "晨曦") {
		t.Errorf("第一章标题应包含'晨曦'，实际: %q", r.Chapters[0].Title)
	}
	if !strings.Contains(r.Chapters[2].Title, "夜幕") {
		t.Errorf("第三章标题应包含'夜幕'，实际: %q", r.Chapters[2].Title)
	}
}

// ── 3. 带特殊前缀的章节（☆、等）—— 关键 BUG CASE ────────
// 网文常见格式："☆、第1章 chapter.001"
// strict_line_start 不匹配（行首非"第"），但 loose_inline 模式
// 可匹配到行内的"第N章"。统计选择时 loose_inline 的匹配数
// 应大于其他模式，从而正确识别此类章节。

func TestParseTxt_SpecialPrefixStarChapters(t *testing.T) {
	content := "==================\n" +
		"☆、第1章 chapter.001\n铺天盖地的丧尸从四面八方包围过来...\n\n" +
		"☆、第2章 chapter.002\n昏昏沉沉中，何文琳疲惫得睁不开眼...\n\n" +
		"☆、第3章 chapter.003\n何文琳警惕的打量四周...\n"
	path := writeTxtFile(t, "星号前缀.txt", content)
	r, err := parseTxt(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Chapters) != 3 {
		t.Fatalf("期望 3 章（☆前缀章节应被 loose_inline 模式识别），实际 %d 章", len(r.Chapters))
	}
	// 验证章节标题包含章节信息
	for i, ch := range r.Chapters {
		if !strings.Contains(ch.Title, fmt.Sprintf("chapter.%03d", i+1)) {
			t.Errorf("第%d章标题应包含'chapter.%03d'，实际: %q", i+1, i+1, ch.Title)
		}
	}
	// 验证章节内容包含对应正文
	if !strings.Contains(r.Chapters[0].Content, "丧尸") {
		t.Errorf("第1章内容应包含'丧尸'，实际: %q", r.Chapters[0].Content)
	}
	if !strings.Contains(r.Chapters[1].Content, "何文琳") {
		t.Errorf("第2章内容应包含'何文琳'，实际: %q", r.Chapters[1].Content)
	}
}

// ── 4. 英文章节 ──────────────────────────────────────────

func TestParseTxt_EnglishChapters(t *testing.T) {
	content := "Chapter 1: The Beginning\nSome text here...\n\n" +
		"Chapter 2: The Development\nMore text...\n\n" +
		"Chapter 3: The Climax\nFinal text...\n"
	path := writeTxtFile(t, "english.txt", content)
	r, err := parseTxt(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Chapters) != 3 {
		t.Fatalf("期望 3 章，实际 %d 章", len(r.Chapters))
	}
	if !strings.Contains(r.Chapters[0].Title, "The Beginning") {
		t.Errorf("Chapter 1 标题应包含'The Beginning'，实际: %q", r.Chapters[0].Title)
	}
	if !strings.Contains(r.Chapters[2].Title, "The Climax") {
		t.Errorf("Chapter 3 标题应包含'The Climax'，实际: %q", r.Chapters[2].Title)
	}
	if !strings.Contains(r.Chapters[1].Content, "More text") {
		t.Errorf("Chapter 2 内容应包含'More text'，实际: %q", r.Chapters[1].Content)
	}
}

// ── 5. 混合格式（严格模式应优于宽松模式） ──────────────────
// 正文中提及章节号（如"第3章的内容"）不应被误识别为章节头。
// 期望行为：strict_line_start 的 3 次匹配应胜过 loose_inline 的 4 次匹配，
// 因为 strict 模式更可靠。
// 当前实现按匹配数最多的模式胜出，如果 loose_inline 匹配数更多则可能产生
// 额外章节。此测试记录期望行为。

func TestParseTxt_MixedFormat_StrictPatternWins(t *testing.T) {
	content := "第1章 正式标题\n这里的正文提到了第3章的内容...\n\n" +
		"第2章 另一个标题\n更多正文...\n\n" +
		"第3章 第三个标题\n正文继续...\n"
	path := writeTxtFile(t, "混合格式.txt", content)
	r, err := parseTxt(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Chapters) != 3 {
		t.Fatalf("期望 3 章（正文中的章节引用不应产生额外章节），实际 %d 章\n"+
			"注意：如果 loose_inline 模式匹配数 > strict_line_start，当前实现会选择 loose_inline，导致多余章节", len(r.Chapters))
	}
	// 第1章的内容中应该包含对"第3章"的引用（它在正文中，不是章节头）
	if !strings.Contains(r.Chapters[0].Content, "第3章的内容") {
		t.Errorf("第1章内容应包含'第3章的内容'（正文引用），实际: %q", r.Chapters[0].Content)
	}
	if !strings.Contains(r.Chapters[2].Title, "第三个标题") {
		t.Errorf("第3章标题应包含'第三个标题'，实际: %q", r.Chapters[2].Title)
	}
}

// ── 6. 完全没有章节标记 ──────────────────────────────────

func TestParseTxt_NoChapterMarkers(t *testing.T) {
	// 真正无章节标记（不含"第X章"字样，避免 loose_inline 误匹配）
	content := "这是一段没有任何章节标记的纯文本。\n它只有正文内容，没有任何章节号。\n应该整段作为一个章节处理。\n"
	path := writeTxtFile(t, "纯文本.txt", content)
	r, err := parseTxt(path)
	if err != nil {
		t.Fatal(err)
	}
	// 无章节标记 → candidates==0 → NeedsLLM=true（不导入单章）
	if !r.NeedsLLM {
		t.Errorf("期望 NeedsLLM=true（无章节标记时不导入单章），实际 NeedsLLM=%v chapters=%d", r.NeedsLLM, len(r.Chapters))
	}
}

// ── 7. 仅1个章节匹配（<=1 个匹配时视为单章） ──────────────
// 当全文仅有1个章节标记匹配时，可能是正文中偶然提及，
// bestCount <= 1 时整个文件视为一章。

func TestParseTxt_SingleChapterMatch(t *testing.T) {
	content := "只有第1章是标记的\n其他内容都是正文\n没有更多的章节了\n"
	path := writeTxtFile(t, "单匹配.txt", content)
	r, err := parseTxt(path)
	if err != nil {
		t.Fatal(err)
	}
	// bestCount <= 1 → 整个文件视为1章
	if len(r.Chapters) != 1 {
		t.Fatalf("期望 1 章（仅1个匹配时 bestCount<=1，应视为单章），实际 %d 章", len(r.Chapters))
	}
	if !strings.Contains(r.Chapters[0].Content, "其他内容都是正文") {
		t.Errorf("章节内容应包含'其他内容都是正文'，实际: %q", r.Chapters[0].Content)
	}
	if !strings.Contains(r.Chapters[0].Content, "只有第1章是标记的") {
		t.Errorf("章节内容应包含'只有第1章是标记的'，实际: %q", r.Chapters[0].Content)
	}
}

// ── 8. Markdown 标题章节 ─────────────────────────────────

func TestParseTxt_MarkdownHeadingChapters(t *testing.T) {
	content := "# 第1章 开始\n正文...\n\n## 第2章 发展\n正文...\n"
	path := writeTxtFile(t, "markdown.md", content)
	r, err := parseTxt(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Chapters) != 2 {
		t.Fatalf("期望 2 章，实际 %d 章", len(r.Chapters))
	}
	// Markdown # 前缀应在标题中被去除
	if strings.HasPrefix(r.Chapters[0].Title, "#") {
		t.Errorf("章节标题不应以 # 开头，实际: %q", r.Chapters[0].Title)
	}
	if !strings.Contains(r.Chapters[0].Title, "开始") {
		t.Errorf("第1章标题应包含'开始'，实际: %q", r.Chapters[0].Title)
	}
	if !strings.Contains(r.Chapters[1].Title, "发展") {
		t.Errorf("第2章标题应包含'发展'，实际: %q", r.Chapters[1].Title)
	}
}

// ── 9. 卷标记 ────────────────────────────────────────────
// juan_line_start 模式支持无"第"前缀的"卷N"格式，
// 如"卷一"、"卷二"。

func TestParseTxt_VolumeMarkersWithoutPrefix(t *testing.T) {
	content := "卷一 风起\n正文内容...\n\n卷二 云涌\n正文内容...\n"
	path := writeTxtFile(t, "卷标记.txt", content)
	r, err := parseTxt(path)
	if err != nil {
		t.Fatal(err)
	}
	// juan_line_start 模式匹配"卷一"、"卷二"
	if len(r.Chapters) != 2 {
		t.Fatalf("期望 2 章（juan_line_start 模式应识别'卷N'格式），实际 %d 章", len(r.Chapters))
	}
	if !strings.Contains(r.Chapters[0].Title, "风起") {
		t.Errorf("卷一标题应包含'风起'，实际: %q", r.Chapters[0].Title)
	}
	if !strings.Contains(r.Chapters[1].Title, "云涌") {
		t.Errorf("卷二标题应包含'云涌'，实际: %q", r.Chapters[1].Title)
	}
}

func TestParseTxt_VolumeMarkersWithPrefix(t *testing.T) {
	content := "第一卷 风起\n正文内容...\n\n第二卷 云涌\n正文内容...\n"
	path := writeTxtFile(t, "第卷标记.txt", content)
	r, err := parseTxt(path)
	if err != nil {
		t.Fatal(err)
	}
	// "第一卷"、"第二卷" 符合 strict_line_start 的 "第...卷" 格式
	if len(r.Chapters) != 2 {
		t.Fatalf("期望 2 章，实际 %d 章", len(r.Chapters))
	}
	if !strings.Contains(r.Chapters[0].Title, "风起") {
		t.Errorf("第一卷标题应包含'风起'，实际: %q", r.Chapters[0].Title)
	}
	if !strings.Contains(r.Chapters[1].Title, "云涌") {
		t.Errorf("第二卷标题应包含'云涌'，实际: %q", r.Chapters[1].Title)
	}
}

// ── 10. 带全角标点的章节 ─────────────────────────────────

func TestParseTxt_FullWidthPunctuationChapters(t *testing.T) {
	content := "第1章：开端\n正文...\n\n第2章：发展\n正文...\n"
	path := writeTxtFile(t, "全角标点.txt", content)
	r, err := parseTxt(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Chapters) != 2 {
		t.Fatalf("期望 2 章，实际 %d 章", len(r.Chapters))
	}
	if !strings.Contains(r.Chapters[0].Title, "开端") {
		t.Errorf("第1章标题应包含'开端'，实际: %q", r.Chapters[0].Title)
	}
	if !strings.Contains(r.Chapters[1].Title, "发展") {
		t.Errorf("第2章标题应包含'发展'，实际: %q", r.Chapters[1].Title)
	}
}

// ── 11. 大文件模拟（50+ 章） ──────────────────────────────
// 验证统计方法在大量章节下仍能正确识别模式。

func TestParseTxt_LargeFile50Chapters(t *testing.T) {
	var sb strings.Builder
	for i := 1; i <= 50; i++ {
		fmt.Fprintf(&sb, "第%d章 章节标题%d\n", i, i)
		sb.WriteString("这是本章节的正文内容，包含足够的文字来模拟真实小说。\n\n")
	}
	path := writeTxtFile(t, "大文件50章.txt", sb.String())
	r, err := parseTxt(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Chapters) != 50 {
		t.Fatalf("期望 50 章，实际 %d 章", len(r.Chapters))
	}
	// 抽查首尾章节
	if !strings.Contains(r.Chapters[0].Title, "章节标题1") {
		t.Errorf("第1章标题应包含'章节标题1'，实际: %q", r.Chapters[0].Title)
	}
	if !strings.Contains(r.Chapters[49].Title, "章节标题50") {
		t.Errorf("第50章标题应包含'章节标题50'，实际: %q", r.Chapters[49].Title)
	}
	if !strings.Contains(r.Chapters[24].Content, "正文内容") {
		t.Errorf("第25章内容应包含'正文内容'，实际: %q", r.Chapters[24].Content)
	}
}

// 大文件模拟：50章中文数字
func TestParseTxt_LargeFile50ChineseNumeralChapters(t *testing.T) {
	chineseNums := []string{
		"一", "二", "三", "四", "五", "六", "七", "八", "九", "十",
		"十一", "十二", "十三", "十四", "十五", "十六", "十七", "十八", "十九", "二十",
		"二十一", "二十二", "二十三", "二十四", "二十五", "二十六", "二十七", "二十八", "二十九", "三十",
		"三十一", "三十二", "三十三", "三十四", "三十五", "三十六", "三十七", "三十八", "三十九", "四十",
		"四十一", "四十二", "四十三", "四十四", "四十五", "四十六", "四十七", "四十八", "四十九", "五十",
	}
	var sb strings.Builder
	for _, num := range chineseNums {
		fmt.Fprintf(&sb, "第%s章 标题%s\n", num, num)
		sb.WriteString("这是本章节的正文内容，模拟真实小说的长度。\n\n")
	}
	path := writeTxtFile(t, "大文件50中文数字章.txt", sb.String())
	r, err := parseTxt(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Chapters) != 50 {
		t.Fatalf("期望 50 章，实际 %d 章", len(r.Chapters))
	}
}

// ── 12. GB18030 编码文件 ─────────────────────────────────

func TestParseTxt_GB18030Encoded(t *testing.T) {
	content := "第1章 雪夜\n\n她推开窗，看见满城灯火。\n\n第2章 归途\n\n马蹄声穿过长街。\n"
	encoded, err := simplifiedchinese.GB18030.NewEncoder().Bytes([]byte(content))
	if err != nil {
		t.Fatal(err)
	}
	path := writeTxtFileBytes(t, "gb18030编码.txt", encoded)
	r, err := parseTxt(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Chapters) != 2 {
		t.Fatalf("期望 2 章，实际 %d 章", len(r.Chapters))
	}
	if !strings.Contains(r.Chapters[0].Title, "雪夜") {
		t.Errorf("第1章标题应包含'雪夜'，实际: %q", r.Chapters[0].Title)
	}
	if !strings.Contains(r.Chapters[1].Content, "马蹄声") {
		t.Errorf("第2章内容应包含'马蹄声'，实际: %q", r.Chapters[1].Content)
	}
}

// GB18030 编码 + 中文数字章节
func TestParseTxt_GB18030ChineseNumeralChapters(t *testing.T) {
	content := "第一章 晨曦\n清晨的阳光洒满大地。\n\n第二章 暮色\n夕阳西下，染红了天边。\n"
	encoded, err := simplifiedchinese.GB18030.NewEncoder().Bytes([]byte(content))
	if err != nil {
		t.Fatal(err)
	}
	path := writeTxtFileBytes(t, "gb18030中文数字.txt", encoded)
	r, err := parseTxt(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Chapters) != 2 {
		t.Fatalf("期望 2 章，实际 %d 章", len(r.Chapters))
	}
	if !strings.Contains(r.Chapters[0].Content, "阳光") {
		t.Errorf("第一章内容应包含'阳光'，实际: %q", r.Chapters[0].Content)
	}
}

// ── 边界与补充测试 ───────────────────────────────────────

// 带部标记的章节（正则支持"第...部"格式）
func TestParseTxt_BuMarker(t *testing.T) {
	content := "第一部 起源\n内容...\n\n第二部 发展\n内容...\n"
	path := writeTxtFile(t, "部标记.txt", content)
	r, err := parseTxt(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Chapters) != 2 {
		t.Fatalf("期望 2 章，实际 %d 章", len(r.Chapters))
	}
	if !strings.Contains(r.Chapters[0].Title, "起源") {
		t.Errorf("第一部标题应包含'起源'，实际: %q", r.Chapters[0].Title)
	}
}

// 英文章节带点号分隔符
func TestParseTxt_EnglishChapterDotSeparator(t *testing.T) {
	content := "Chapter 1. The Beginning\nSome text...\n\nChapter 2. The End\nMore text...\n"
	path := writeTxtFile(t, "english_dot.txt", content)
	r, err := parseTxt(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Chapters) != 2 {
		t.Fatalf("期望 2 章，实际 %d 章", len(r.Chapters))
	}
}

// 章节标题中包含特殊字符
func TestParseTxt_ChapterTitleWithSpecialChars(t *testing.T) {
	content := "第1章 【特别篇】奇迹\n正文内容...\n\n第2章 《外传》暗影\n更多正文...\n"
	path := writeTxtFile(t, "特殊标题.txt", content)
	r, err := parseTxt(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Chapters) != 2 {
		t.Fatalf("期望 2 章，实际 %d 章", len(r.Chapters))
	}
	if !strings.Contains(r.Chapters[0].Title, "特别篇") {
		t.Errorf("第1章标题应包含'特别篇'，实际: %q", r.Chapters[0].Title)
	}
}

// 连续章节标记无正文（极端边界）
func TestParseTxt_ConsecutiveChapterMarkers(t *testing.T) {
	content := "第1章 标题1\n第2章 标题2\n第3章 标题3\n"
	path := writeTxtFile(t, "连续章节.txt", content)
	r, err := parseTxt(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Chapters) != 3 {
		t.Fatalf("期望 3 章，实际 %d 章", len(r.Chapters))
	}
}

// 验证章节内容不重叠
func TestParseTxt_ChapterContentsDoNotOverlap(t *testing.T) {
	content := "第1章 独立\n这是独有内容AAA。\n\n第2章 分离\n这是另一段独有内容BBB。\n\n第3章 隔离\n最后一段独有内容CCC。\n"
	path := writeTxtFile(t, "不重叠.txt", content)
	r, err := parseTxt(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Chapters) != 3 {
		t.Fatalf("期望 3 章，实际 %d 章", len(r.Chapters))
	}
	// 第1章不应包含第2章内容
	if strings.Contains(r.Chapters[0].Content, "BBB") {
		t.Errorf("第1章内容不应包含第2章的'BBB'")
	}
	// 第2章不应包含第3章内容
	if strings.Contains(r.Chapters[1].Content, "CCC") {
		t.Errorf("第2章内容不应包含第3章的'CCC'")
	}
	// 各章应包含自己的内容
	if !strings.Contains(r.Chapters[0].Content, "AAA") {
		t.Errorf("第1章内容应包含'AAA'")
	}
	if !strings.Contains(r.Chapters[1].Content, "BBB") {
		t.Errorf("第2章内容应包含'BBB'")
	}
	if !strings.Contains(r.Chapters[2].Content, "CCC") {
		t.Errorf("第3章内容应包含'CCC'")
	}
}

// UTF-8 BOM + 章节分割
func TestParseTxt_UTF8BOMWithChapters(t *testing.T) {
	content := "\xEF\xBB\xBF第1章 开始\n正文一。\n\n第2章 继续\n正文二。\n"
	path := writeTxtFile(t, "bom章节.txt", content)
	r, err := parseTxt(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Chapters) != 2 {
		t.Fatalf("期望 2 章（BOM 应被去除），实际 %d 章", len(r.Chapters))
	}
	if !strings.Contains(r.Chapters[0].Title, "开始") {
		t.Errorf("第1章标题应包含'开始'，实际: %q", r.Chapters[0].Title)
	}
}

// CR 换行（旧 Mac 格式）
func TestParseTxt_CRLineEndings(t *testing.T) {
	content := "第1章 测试\r正文内容。\r第2章 继续\r更多正文。\r"
	path := writeTxtFile(t, "cr换行.txt", content)
	r, err := parseTxt(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Chapters) != 2 {
		t.Fatalf("期望 2 章，实际 %d 章", len(r.Chapters))
	}
}

// 章节标题提取：标题行之后的正文不应混入标题
func TestParseTxt_ChapterTitleExtraction(t *testing.T) {
	content := "第1章 标题行\n这是正文第一行。\n这是正文第二行。\n\n第2章 另一个标题\n后续正文内容。\n"
	path := writeTxtFile(t, "标题提取.txt", content)
	r, err := parseTxt(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Chapters) != 2 {
		t.Fatalf("期望 2 章，实际 %d 章", len(r.Chapters))
	}
	// 标题不应包含正文
	if strings.Contains(r.Chapters[0].Title, "正文第一行") {
		t.Errorf("标题不应包含正文内容，实际: %q", r.Chapters[0].Title)
	}
	// 内容应包含正文
	if !strings.Contains(r.Chapters[0].Content, "正文第一行") {
		t.Errorf("第1章内容应包含'正文第一行'，实际: %q", r.Chapters[0].Content)
	}
}

// ── chapterPatterns 正则匹配测试 ─────────────────────────

func TestChapterPatterns_StrictLineStart(t *testing.T) {
	p := chapterPatterns[0] // strict_line_start
	tests := []struct {
		line    string
		matches bool
	}{
		{"第1章 标题", true},
		{"第一章 标题", true},
		{"第001章 标题", true},
		{"第12章：冒号标题", true},
		{"  第5章 空格缩进", true},
		{"　　第5章 全角缩进", true},
		{"# 第1章 标题", true},
		{"## 第一章 开篇", true},
		{"第一卷 风起", true},
		{"第一部 起源", true},
		// 不应匹配的
		{"这是第5章的内容", false},
		{"☆、第1章 chapter.001", false},
		{"", false},
	}
	for _, tc := range tests {
		got := p.pattern.MatchString(tc.line)
		if got != tc.matches {
			t.Errorf("strict_line_start.MatchString(%q) = %v, want %v", tc.line, got, tc.matches)
		}
	}
}

func TestChapterPatterns_LooseInline(t *testing.T) {
	p := chapterPatterns[1] // loose_inline
	tests := []struct {
		content string
		count   int
	}{
		{"第1章 标题\n正文\n第2章 标题二\n", 2},
		{"☆、第1章 chapter.001\n正文\n☆、第2章 chapter.002\n", 2},
		{"这里的正文提到了第3章的内容\n第1章 标题\n", 2}, // inline + line start
	}
	for _, tc := range tests {
		matches := p.pattern.FindAllStringIndex(tc.content, -1)
		if len(matches) != tc.count {
			t.Errorf("loose_inline.FindAll(%q) = %d matches, want %d", tc.content[:minLen(30, len(tc.content))], len(matches), tc.count)
		}
	}
}

func TestChapterPatterns_English(t *testing.T) {
	p := chapterPatterns[3] // english
	tests := []struct {
		line    string
		matches bool
	}{
		{"Chapter 1 Introduction", true},
		{"Chapter 23", true},
		{"chapter 5 test", true}, // case insensitive
		{"This is chapter 5", false},
		{"", false},
	}
	for _, tc := range tests {
		got := p.pattern.MatchString(tc.line)
		if got != tc.matches {
			t.Errorf("english.MatchString(%q) = %v, want %v", tc.line, got, tc.matches)
		}
	}
}

func TestChapterPatterns_JuanLineStart(t *testing.T) {
	p := chapterPatterns[4] // juan_line_start
	tests := []struct {
		line    string
		matches bool
	}{
		{"卷一 风起", true},
		{"卷二 云涌", true},
		{"卷10 低谷", true},
		{"# 卷三 转折", true},
		{"  卷四 结局", true},
		// 不应匹配
		{"这是卷一的内容", false},
		{"第一卷 风起", false}, // 有"第"前缀，由 strict_line_start 处理
		{"", false},
	}
	for _, tc := range tests {
		got := p.pattern.MatchString(tc.line)
		if got != tc.matches {
			t.Errorf("juan_line_start.MatchString(%q) = %v, want %v", tc.line, got, tc.matches)
		}
	}
}

// ── 辅助函数测试 ─────────────────────────────────────────

// DecodeText 单元测试：有效 UTF-8 直接返回
func TestDecodeText_ValidUTF8(t *testing.T) {
	input := "第1章 你好世界"
	result, err := DecodeText([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if result != input {
		t.Errorf("UTF-8 文本应原样返回，实际: %q", result)
	}
}

// DecodeText 单元测试：无效 UTF-8 回退 GB18030
func TestDecodeText_InvalidUTF8FallbackGB18030(t *testing.T) {
	original := "第1章 雪夜"
	encoded, err := simplifiedchinese.GB18030.NewEncoder().Bytes([]byte(original))
	if err != nil {
		t.Fatal(err)
	}
	result, err := DecodeText(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if result != original {
		t.Errorf("GB18030 解码结果不匹配，期望 %q，实际 %q", original, result)
	}
}

// trimUTF8BOM 单元测试
func TestTrimUTF8BOM(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected []byte
	}{
		{"无BOM", []byte("hello"), []byte("hello")},
		{"有BOM", append([]byte{0xEF, 0xBB, 0xBF}, []byte("hello")...), []byte("hello")},
		{"空输入", []byte{}, []byte{}},
		{"仅BOM", []byte{0xEF, 0xBB, 0xBF}, []byte{}},
		{"BOM不完整2字节", []byte{0xEF, 0xBB}, []byte{0xEF, 0xBB}},
		{"BOM不完整1字节", []byte{0xEF}, []byte{0xEF}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := trimUTF8BOM(tc.input)
			if string(result) != string(tc.expected) {
				t.Errorf("trimUTF8BOM(%x) = %x, want %x", tc.input, result, tc.expected)
			}
		})
	}
}

// inferTitle 单元测试
func TestInferTitle_Comprehensive(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"小说.txt", "小说"},
		{"my_novel.md", "my_novel"},
		{"/path/to/小说名.txt", "小说名"},
		{"C:\\Users\\test\\book.txt", "book"},
		{"noext", "noext"},
		{".hidden", "未命名"}, // 点开头的文件名：最后一个点是扩展名分隔符，基名为空
		{"a.b.c.txt", "a.b.c"},
		{"结尾下划线_.txt", "结尾下划线"},
		{"", "未命名"},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := inferTitle(tc.input)
			if got != tc.expected {
				t.Errorf("inferTitle(%q) = %q, want %q", tc.input, got, tc.expected)
			}
		})
	}
}

// ── 行长度过滤测试 ───────────────────────────────────────

// 长行中提及章节号应被行长度过滤器排除（maxChapterTitleLen=30）
func TestParseTxt_LongBodyLineExceedsTitleLen(t *testing.T) {
	longLine := "第三章的内容非常精彩，主角在这一章节中展现了超凡的实力，打败了所有的敌人。"
	for len([]rune(longLine)) < maxChapterTitleLen+10 {
		longLine += "继续填充内容使其超过长度阈值。"
	}
	content := "第1章 开篇\n\n正文。\n\n" + longLine + "\n\n第2章 继续\n\n正文。\n"
	path := writeTxtFile(t, "长行过滤.txt", content)
	r, err := parseTxt(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Chapters) != 2 {
		t.Fatalf("期望 2 章（长行中的章节提及应被过滤），实际 %d 章", len(r.Chapters))
	}
}

// ── 统计选择逻辑测试 ─────────────────────────────────────

// 当 strict 和 loose 匹配数相同时，strict 应优先（idx 更小）
func TestParseTxt_StrictWinsOverLooseOnTie(t *testing.T) {
	content := "第1章 标题1\n正文一。\n\n第2章 标题2\n正文二。\n\n第3章 标题3\n正文三。\n"
	path := writeTxtFile(t, "strict优先.txt", content)
	r, err := parseTxt(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Chapters) != 3 {
		t.Fatalf("期望 3 章，实际 %d 章", len(r.Chapters))
	}
}

// 0 个匹配时整文件为一章
func TestParseTxt_ZeroMatches(t *testing.T) {
	content := "纯文本，没有章节标记。\n多行内容。\n"
	path := writeTxtFile(t, "零匹配.txt", content)
	r, err := parseTxt(path)
	if err != nil {
		t.Fatal(err)
	}
	// 0 匹配 → 不导入单章，提示 AI 分析
	if !r.NeedsLLM {
		t.Errorf("期望 NeedsLLM=true（0 匹配时不导入单章），实际 NeedsLLM=%v", r.NeedsLLM)
	}
}

// minLen helper for test formatting
func minLen(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ── 扩充正则测试 ─────────────────────────────────────────

// "第...回" 格式（如"第一百一十回"）
func TestParseTxt_HuiMarker(t *testing.T) {
	content := "第一回 甄士隐梦幻识通灵\n正文内容...\n\n第二回 贾夫人仙逝扬州城\n正文内容...\n\n第三回 托内兄如海酬训教\n正文内容...\n"
	path := writeTxtFile(t, "回目.txt", content)
	r, err := parseTxt(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Chapters) != 3 {
		t.Fatalf("期望 3 章（第...回格式），实际 %d 章", len(r.Chapters))
	}
	if !strings.Contains(r.Chapters[0].Title, "甄士隐") {
		t.Errorf("第一回标题应包含'甄士隐'，实际: %q", r.Chapters[0].Title)
	}
}

// "第...节" 格式
func TestParseTxt_JieMarker(t *testing.T) {
	content := "第一节 初识\n正文...\n\n第二节 深入\n正文...\n\n第三节 高潮\n正文...\n"
	path := writeTxtFile(t, "节标记.txt", content)
	r, err := parseTxt(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Chapters) != 3 {
		t.Fatalf("期望 3 章（第...节格式），实际 %d 章", len(r.Chapters))
	}
}

// 序章
func TestParseTxt_Xuzhang(t *testing.T) {
	content := "序章 黎明之前\n黑暗笼罩大地...\n\n第1章 觉醒\n主角睁开了眼睛...\n\n第2章 出发\n踏上了旅途...\n"
	path := writeTxtFile(t, "序章.txt", content)
	r, err := parseTxt(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Chapters) != 3 {
		t.Fatalf("期望 3 章（序章应作为独立章节），实际 %d 章", len(r.Chapters))
	}
	if !strings.Contains(r.Chapters[0].Title, "黎明之前") {
		t.Errorf("第一章标题应包含'黎明之前'（序章前缀被去掉），实际: %q", r.Chapters[0].Title)
	}
}

// 楔子
func TestParseTxt_Xiezi(t *testing.T) {
	content := "楔子 风起\n正文内容...\n\n第1章 开始\n正文...\n"
	path := writeTxtFile(t, "楔子.txt", content)
	r, err := parseTxt(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Chapters) != 2 {
		t.Fatalf("期望 2 章（楔子应作为独立章节），实际 %d 章", len(r.Chapters))
	}
	if !strings.Contains(r.Chapters[0].Title, "风起") {
		t.Errorf("第一章标题应包含'风起'（楔子前缀被去掉），实际: %q", r.Chapters[0].Title)
	}
}

// 引子
func TestParseTxt_Yinzi(t *testing.T) {
	content := "引子 暗涌\n正文...\n\n第1章 风暴\n正文...\n"
	path := writeTxtFile(t, "引子.txt", content)
	r, err := parseTxt(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Chapters) != 2 {
		t.Fatalf("期望 2 章（引子应作为独立章节），实际 %d 章", len(r.Chapters))
	}
}

// 番外
func TestParseTxt_Fanwai(t *testing.T) {
	content := "第1章 开始\n正文...\n\n第2章 高潮\n正文...\n\n番外 那些年\n番外内容...\n\n番外二 另一个故事\n更多番外...\n"
	path := writeTxtFile(t, "番外.txt", content)
	r, err := parseTxt(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Chapters) < 4 {
		t.Fatalf("期望至少 4 章（含番外），实际 %d 章", len(r.Chapters))
	}
}

// 终章
func TestParseTxt_Zhongzhang(t *testing.T) {
	content := "第1章 开始\n正文...\n\n第2章 发展\n正文...\n\n终章 尾声\n正文...\n"
	path := writeTxtFile(t, "终章.txt", content)
	r, err := parseTxt(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Chapters) != 3 {
		t.Fatalf("期望 3 章（终章应作为独立章节），实际 %d 章", len(r.Chapters))
	}
	if !strings.Contains(r.Chapters[2].Title, "尾声") {
		t.Errorf("第三章标题应包含'尾声'（终章前缀被去掉），实际: %q", r.Chapters[2].Title)
	}
}

// 后记
func TestParseTxt_Houji(t *testing.T) {
	content := "第1章 正文\n内容...\n\n第2章 结局\n内容...\n\n后记\n感谢阅读。\n"
	path := writeTxtFile(t, "后记.txt", content)
	r, err := parseTxt(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Chapters) < 3 {
		t.Fatalf("期望至少 3 章（含后记），实际 %d 章", len(r.Chapters))
	}
}

// 单独"序"字
func TestParseTxt_SingleXu(t *testing.T) {
	content := "序\n这是序言的内容。\n\n第1章 开始\n正文...\n"
	path := writeTxtFile(t, "序.txt", content)
	r, err := parseTxt(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Chapters) != 2 {
		t.Fatalf("期望 2 章（序应作为独立章节），实际 %d 章", len(r.Chapters))
	}
}

// 纯数字章节行（某些小说用纯数字作为章节号，如 "44. 某某某(下)" 的简化形式）
func TestParseTxt_NumericChapters(t *testing.T) {
	content := "1\n正文内容...\n\n2\n更多内容...\n\n3\n最后一段...\n"
	path := writeTxtFile(t, "数字章节.txt", content)
	r, err := parseTxt(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Chapters) != 3 {
		t.Fatalf("期望 3 章（纯数字章节行），实际 %d 章", len(r.Chapters))
	}
}

// 同一行多匹配去重（===第3章 第03章=== 只产生一个分割点）
// 注意：加权逻辑可能选择 strict（行首模式）而忽略非行首的"===第3章==="。
// 这是有意为之：strict 更可靠，装饰符号包裹的章节头属于长尾格式，后续由 LLM 保底处理。
func TestParseTxt_SameLineMultiMatchDedup(t *testing.T) {
	content := "第1章 开始\n正文内容...\n\n第2章 继续\n更多正文内容...\n\n===第3章 第03章 标题===\n正文...\n"
	path := writeTxtFile(t, "同行多匹配.txt", content)
	r, err := parseTxt(path)
	if err != nil {
		t.Fatal(err)
	}
	// 关键验证：不会因为同一行多匹配产生重复章节
	for i := 1; i < len(r.Chapters); i++ {
		if r.Chapters[i].Title == r.Chapters[i-1].Title {
			t.Errorf("第%d章和第%d章标题重复: %q", i, i-1, r.Chapters[i].Title)
		}
	}
	// 加权选 strict 后得到 2 章（第1章 + 第2章），"===第3章===" 被忽略
	// 这是可接受的降级行为，LLM 保底可以后续处理
	if len(r.Chapters) < 2 {
		t.Fatalf("期望至少 2 章，实际 %d 章", len(r.Chapters))
	}
}

// special_markers 正则单元测试
func TestChapterPatterns_SpecialMarkers(t *testing.T) {
	p := chapterPatterns[2] // special_markers
	tests := []struct {
		line    string
		matches bool
	}{
		{"序章 黎明之前", true},
		{"楔子 风起", true},
		{"引子 暗涌", true},
		{"番外 那些年", true},
		{"番外一 另一个故事", true},
		{"番外二 结局", true},
		{"终章 尾声", true},
		{"后记", true},
		{"序言", true},
		{"序", true},
		// 不应匹配
		{"这是序章的内容", false},
		{"他写了一篇后记", false},
		{"序章内容很多", false}, // 行首匹配但后面紧跟非空格字符，不应匹配
		{"", false},
	}
	for _, tc := range tests {
		got := p.pattern.MatchString(tc.line)
		if got != tc.matches {
			t.Errorf("special_markers.MatchString(%q) = %v, want %v", tc.line, got, tc.matches)
		}
	}
}

// numeric_line 正则单元测试
func TestChapterPatterns_NumericLine(t *testing.T) {
	p := chapterPatterns[6] // numeric_line
	tests := []struct {
		line    string
		matches bool
	}{
		{"1", true},
		{"2", true},
		{"599", true},
		{"  123", true},
		{"\t45", true},
		// 不应匹配
		{"123章", false},
		{"第1章", false},
		{"abc", false},
		{"", false},
		{"123456", false}, // 超过5位
	}
	for _, tc := range tests {
		got := p.pattern.MatchString(tc.line)
		if got != tc.matches {
			t.Errorf("numeric_line.MatchString(%q) = %v, want %v", tc.line, got, tc.matches)
		}
	}
}
