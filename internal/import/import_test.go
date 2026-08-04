package imp

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/text/encoding/simplifiedchinese"
)

// ── Parse 路由 ─────────────────────────────────────────────

func TestParse_UnknownExtension(t *testing.T) {
	_, err := Parse("test.pdf", nil)
	if err == nil {
		t.Fatal("expected error for .pdf")
	}
	_, err = Parse("test.doc", nil)
	if err == nil {
		t.Fatal("expected error for .doc")
	}
}

// ── TXT 解析 ───────────────────────────────────────────────

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeTempBytes(t *testing.T, name string, content []byte) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestParseTxt_StandardChapters(t *testing.T) {
	content := "第1章 楔子\n\n这是第一章的内容。\n\n" +
		"第2章 启程\n\n主角离开了村庄。\n\n" +
		"第3章 遭遇\n\n遇到了第一个敌人。\n"
	path := writeTemp(t, "novel.txt", content)
	r, err := parseTxt(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Chapters) != 3 {
		t.Fatalf("expected 3 chapters, got %d", len(r.Chapters))
	}
	if r.Title != "novel" {
		t.Fatalf("expected title 'novel', got %q", r.Title)
	}
	if !strings.Contains(r.Chapters[0].Content, "楔子") {
		t.Error("chapter 1 should contain 楔子")
	}
	if !strings.Contains(r.Chapters[2].Content, "敌人") {
		t.Error("chapter 3 should contain 敌人")
	}
}

func TestParseTxt_ChineseNumberChapters(t *testing.T) {
	content := "第一章 开篇\n\n内容一。\n\n第十章 高潮\n\n内容十。\n"
	path := writeTemp(t, "novel.txt", content)
	r, err := parseTxt(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Chapters) != 2 {
		t.Fatalf("expected 2 chapters, got %d", len(r.Chapters))
	}
}

func TestParseTxt_PaddedNumbers(t *testing.T) {
	content := "第001章 开始\n\n正文一。\n\n第010章 中期\n\n正文十。\n"
	path := writeTemp(t, "novel.txt", content)
	r, err := parseTxt(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Chapters) != 2 {
		t.Fatalf("expected 2 chapters, got %d", len(r.Chapters))
	}
}

func TestParseTxt_ChapterInBodyNotConfused(t *testing.T) {
	content := "第1章 开始\n\n他想起之前那段情节，觉得故事写得很好。\n\n" +
		"第2章 继续\n\n第二段。\n"
	path := writeTemp(t, "novel.txt", content)
	r, err := parseTxt(path)
	if err != nil {
		t.Fatal(err)
	}
	// 正文不应被识别为章节头
	if len(r.Chapters) != 2 {
		t.Fatalf("expected 2 chapters (body refs ignored), got %d", len(r.Chapters))
	}
}

func TestParseTxt_LongBodyLineFiltered(t *testing.T) {
	// 长行中提及章节号应被过滤（超出 maxChapterTitleLen）
	longLine := "第三章的内容非常精彩，主角在这一章节中展现了超凡的实力，打败了所有的敌人。"
	for len([]rune(longLine)) < maxChapterTitleLen+10 {
		longLine += "继续填充内容使其超过长度阈值。"
	}
	content := "第1章 开篇\n\n正文。\n\n" + longLine + "\n\n第2章 继续\n\n正文。\n"
	path := writeTemp(t, "long.txt", content)
	r, err := parseTxt(path)
	if err != nil {
		t.Fatal(err)
	}
	// 长行「第三章...」应被长度过滤器排除
	if len(r.Chapters) != 2 {
		t.Fatalf("expected 2 chapters (long body line filtered), got %d", len(r.Chapters))
	}
}

func TestParseTxt_NoChapters(t *testing.T) {
	content := "纯文本，没有任何章节标记。\n多行内容。\n"
	path := writeTemp(t, "novel.txt", content)
	r, err := parseTxt(path)
	if err != nil {
		t.Fatal(err)
	}
	// 无章节标记 → 不导入单章，提示 AI 分析
	if !r.NeedsLLM {
		t.Errorf("expected NeedsLLM=true for no chapter markers, got NeedsLLM=%v chapters=%d", r.NeedsLLM, len(r.Chapters))
	}
}

func TestParseTxt_EmptyFile(t *testing.T) {
	path := writeTemp(t, "empty.txt", "")
	_, err := parseTxt(path)
	if err == nil {
		t.Fatal("expected error for empty file")
	}
}

func TestParseTxt_WhitespaceOnly(t *testing.T) {
	path := writeTemp(t, "ws.txt", "\n  \n\t\n")
	_, err := parseTxt(path)
	if err == nil {
		t.Fatal("expected error for whitespace-only file")
	}
}

func TestParseTxt_IndentedChapter(t *testing.T) {
	content := "　　第1章 缩进章节\n\n正文。\n\n  第2章 半角缩进\n\n正文二。\n"
	path := writeTemp(t, "indented.txt", content)
	r, err := parseTxt(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Chapters) != 2 {
		t.Fatalf("expected 2 chapters, got %d", len(r.Chapters))
	}
}

func TestParseTxt_CRLF(t *testing.T) {
	content := "第1章 测试\r\n\r\n正文。\r\n\r\n第2章 继续\r\n\r\n二章正文。\r\n"
	path := writeTemp(t, "crlf.txt", content)
	r, err := parseTxt(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Chapters) != 2 {
		t.Fatalf("expected 2 chapters, got %d", len(r.Chapters))
	}
}

func TestParseTxt_GB18030(t *testing.T) {
	content := "第1章 雪夜\n\n她推开窗，看见满城灯火。\n\n第2章 归途\n\n马蹄声穿过长街。\n"
	data, err := simplifiedchinese.GB18030.NewEncoder().Bytes([]byte(content))
	if err != nil {
		t.Fatal(err)
	}
	path := writeTempBytes(t, "gb18030.txt", data)

	r, err := parseTxt(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Chapters) != 2 {
		t.Fatalf("expected 2 chapters, got %d", len(r.Chapters))
	}
	if r.Chapters[0].Title != "雪夜" {
		t.Fatalf("title decoded incorrectly: %q", r.Chapters[0].Title)
	}
	if !strings.Contains(r.Chapters[1].Content, "马蹄声") {
		t.Fatalf("content decoded incorrectly: %q", r.Chapters[1].Content)
	}
}

func TestParseTxt_MarkdownFile(t *testing.T) {
	content := "# 第1章 楔子\n\n正文一。\n\n## 第2章 启程\n\n正文二。\n"
	path := writeTemp(t, "novel.md", content)
	r, err := parseTxt(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Chapters) != 2 {
		t.Fatalf("expected 2 chapters, got %d", len(r.Chapters))
	}
	// Markdown # 前缀应在标题中去掉
	if strings.HasPrefix(r.Chapters[0].Title, "#") {
		t.Errorf("chapter title should not start with #: %q", r.Chapters[0].Title)
	}
}

func TestParseTxt_TitleFromFilename(t *testing.T) {
	tests := []struct {
		filename, expected string
	}{
		{"盘龙.txt", "盘龙"},
		{"my_novel.md", "my_novel"},
		{"a.b.c.txt", "a.b.c"},
		{"/path/to/小说名.txt", "小说名"},
		{"noext", "noext"},
	}
	for _, tc := range tests {
		got := inferTitle(tc.filename)
		if got != tc.expected {
			t.Errorf("inferTitle(%q) = %q, want %q", tc.filename, got, tc.expected)
		}
	}
}

// ── TXT 脏格式：网文真实场景 ──────────────────────────────

func TestParseTxt_PrologueEpilogue(t *testing.T) {
	content := "楔子\n\n这是楔子的内容。\n\n第1章 开始\n\n正文。\n\n尾声\n\n这是尾声。\n"
	path := writeTemp(t, "novel.txt", content)
	r, err := parseTxt(path)
	if err != nil {
		t.Fatal(err)
	}
	// 楔子和尾声都不是「第X章」格式，Regex 不会匹配
	// 楔子作为正文开头的一部分并入第1章（因为没有章标识别为独立章）
	if len(r.Chapters) < 1 {
		t.Fatal("expected at least 1 chapter")
	}
}

func TestParseTxt_MixedChineseArabicChapter(t *testing.T) {
	content := "第1章 开始\n\n正文一。\n\n第二章 发展\n\n正文二。\n\n第3章 高潮\n\n正文三。\n"
	path := writeTemp(t, "mixed.txt", content)
	r, err := parseTxt(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Chapters) != 3 {
		t.Fatalf("expected 3 chapters, got %d", len(r.Chapters))
	}
}

func TestParseTxt_LargeChineseNumber(t *testing.T) {
	content := "第十一章 转折\n\n内容十一。\n\n第二十三章 决战\n\n内容二十三。\n"
	path := writeTemp(t, "large_cn.txt", content)
	r, err := parseTxt(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Chapters) != 2 {
		t.Fatalf("expected 2 chapters, got %d", len(r.Chapters))
	}
}

func TestParseTxt_ThousandChapter(t *testing.T) {
	content := "第一千章 新世界\n\n内容一千。\n\n第两千零一章 归来\n\n内容。\n"
	path := writeTemp(t, "thousand.txt", content)
	r, err := parseTxt(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Chapters) != 2 {
		t.Fatalf("expected 2 chapters, got %d", len(r.Chapters))
	}
}

func TestParseTxt_BareChapter(t *testing.T) {
	content := "第1章\n\n正文。\n\n第2章\n\n正文二。\n"
	path := writeTemp(t, "bare.txt", content)
	r, err := parseTxt(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Chapters) != 2 {
		t.Fatalf("expected 2 chapters, got %d", len(r.Chapters))
	}
}

func TestParseTxt_PunctuationVariations(t *testing.T) {
	content := "第1章：楔子\n\n正文。\n\n第2章。启程\n\n正文。\n\n第3章——转折\n\n正文。\n"
	path := writeTemp(t, "punct.txt", content)
	r, err := parseTxt(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Chapters) != 3 {
		t.Fatalf("expected 3 chapters, got %d", len(r.Chapters))
	}
}

func TestParseTxt_VolumeMarkers(t *testing.T) {
	content := "第一卷 初入江湖\n\n第1章 山村少年\n\n正文。\n\n第2章 初遇\n\n正文。\n\n" +
		"第二卷 风云际会\n\n第3章 下山\n\n正文。\n"
	path := writeTemp(t, "volume.txt", content)
	r, err := parseTxt(path)
	if err != nil {
		t.Fatal(err)
	}
	// 卷标记也会被 reg 匹配到，这是可接受的（卷头之后第一个章标会分割）
	if len(r.Chapters) < 3 {
		t.Fatalf("expected at least 3 entries, got %d", len(r.Chapters))
	}
}

func TestParseTxt_UT8BOM(t *testing.T) {
	// UTF-8 BOM: EF BB BF —— BOM 应在 parseTxt 中自动去除
	content := "\xEF\xBB\xBF第1章 开始\n\n正文。\n\n第2章 继续\n\n正文。\n"
	path := writeTemp(t, "bom.txt", content)
	r, err := parseTxt(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Chapters) != 2 {
		t.Fatalf("expected 2 chapters after BOM strip, got %d", len(r.Chapters))
	}
}

func TestParseTxt_SectionMarkers(t *testing.T) {
	content := "第一节 初识\n\n内容。\n\n第二节 深入\n\n内容。\n"
	path := writeTemp(t, "section.txt", content)
	r, err := parseTxt(path)
	if err != nil {
		t.Fatal(err)
	}
	// "第N节" 格式现在被 strict_line_start 识别为章节
	if len(r.Chapters) != 2 {
		t.Fatalf("expected 2 chapters (section markers recognized), got %d", len(r.Chapters))
	}
}

func TestParseTxt_VeryShortChapter(t *testing.T) {
	content := "第1章\n\n短。\n\n第2章 标题\n\n还是很短。\n"
	path := writeTemp(t, "short.txt", content)
	r, err := parseTxt(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Chapters) != 2 {
		t.Fatalf("expected 2 chapters, got %d", len(r.Chapters))
	}
}

func TestParseTxt_BlankLinesAroundHeader(t *testing.T) {
	content := "\n\n\n第1章 开始\n\n\n正文。\n\n\n\n第2章 继续\n\n正文。\n"
	path := writeTemp(t, "blanks.txt", content)
	r, err := parseTxt(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Chapters) != 2 {
		t.Fatalf("expected 2 chapters, got %d", len(r.Chapters))
	}
}

func TestParseTxt_SubtitleAfterChapter(t *testing.T) {
	// 章节头下面紧跟副标题或引用
	content := "第1章 初遇\n——副标题说明——\n\n正文开始。\n\n第2章\n（本章约3000字）\n\n正文。\n"
	path := writeTemp(t, "subtitle.txt", content)
	r, err := parseTxt(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Chapters) != 2 {
		t.Fatalf("expected 2 chapters, got %d", len(r.Chapters))
	}
}

func TestParseTxt_100Chapters(t *testing.T) {
	var sb strings.Builder
	for i := 1; i <= 100; i++ {
		sb.WriteString("第")
		sb.WriteString(itoa(i))
		sb.WriteString("章 章节标题\n\n")
		sb.WriteString("这是本章节的正文内容，模拟真实小说的长度。\n\n")
	}
	path := writeTemp(t, "100chapters.txt", sb.String())
	r, err := parseTxt(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Chapters) != 100 {
		t.Fatalf("expected 100 chapters, got %d", len(r.Chapters))
	}
}

func TestParseTxt_TabIndent(t *testing.T) {
	content := "\t第1章 测试\n\n正文。\n"
	path := writeTemp(t, "tab.txt", content)
	r, err := parseTxt(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Chapters) != 1 {
		t.Fatalf("expected 1 chapter, got %d", len(r.Chapters))
	}
}

// ── EPUB 解析 ───────────────────────────────────────────────

// makeMinimalEpub 创建一个最简 EPUB 文件用于测试
func makeMinimalEpub(t *testing.T, dir string, chapters []struct{ title, body string }) string {
	t.Helper()

	path := filepath.Join(dir, "test.epub")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}

	w := zip.NewWriter(f)

	// mimetype（必须无压缩且第一个条目）
	mw, _ := w.CreateHeader(&zip.FileHeader{Name: "mimetype", Method: zip.Store})
	mw.Write([]byte("application/epub+zip"))

	// META-INF/container.xml
	container := `<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`
	cw, _ := w.Create("META-INF/container.xml")
	cw.Write([]byte(container))

	// OEBPS/content.opf
	manifestItems := ""
	spineItems := ""
	for i := range chapters {
		fn := "chapter" + pad(i+1) + ".xhtml"
		id := "ch" + pad(i+1)
		manifestItems += `<item id="` + id + `" href="` + fn + `" media-type="application/xhtml+xml"/>` + "\n"
		spineItems += `<itemref idref="` + id + `"/>` + "\n"
	}
	opf := `<?xml version="1.0" encoding="UTF-8"?>
<package version="2.0" unique-identifier="bookid" xmlns="http://www.idpf.org/2007/opf">
  <metadata>
    <dc:title xmlns:dc="http://purl.org/dc/elements/1.1/">测试小说</dc:title>
  </metadata>
  <manifest>
    <item id="ncx" href="toc.ncx" media-type="application/x-dtbncx+xml"/>
` + manifestItems + `
  </manifest>
  <spine toc="ncx">
` + spineItems + `
  </spine>
</package>`
	ow, _ := w.Create("OEBPS/content.opf")
	ow.Write([]byte(opf))

	// 章节 HTML
	for i, ch := range chapters {
		fn := "OEBPS/chapter" + pad(i+1) + ".xhtml"
		html := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>` + ch.title + `</title></head>
<body>
<h1>` + ch.title + `</h1>
<p>` + ch.body + `</p>
</body>
</html>`
		hw, _ := w.Create(fn)
		hw.Write([]byte(html))
	}

	w.Close()
	f.Close()
	return path
}

func pad(n int) string { return string(rune('0'+n/10)) + string(rune('0'+n%10)) }

func TestParseEpub_Minimal(t *testing.T) {
	dir := t.TempDir()
	path := makeMinimalEpub(t, dir, []struct{ title, body string }{
		{"第一章", "这是第一章的正文内容。"},
		{"第二章", "第二章发生了重要转折。"},
		{"第三章", "第三章把故事推向高潮。"},
	})

	r, err := Parse(path, nil)
	if err != nil {
		t.Fatalf("parse epub: %v", err)
	}
	if r.Title != "测试小说" {
		t.Errorf("expected title '测试小说', got %q", r.Title)
	}
	if len(r.Chapters) != 3 {
		t.Fatalf("expected 3 chapters, got %d", len(r.Chapters))
	}
	if !strings.Contains(r.Chapters[0].Content, "第一章的正文") {
		t.Errorf("chapter 1 content mismatch: %q", r.Chapters[0].Content[:50])
	}
}

func TestParseEpub_SingleChapter(t *testing.T) {
	dir := t.TempDir()
	path := makeMinimalEpub(t, dir, []struct{ title, body string }{
		{"序章", "一篇短文，只有一章。"},
	})

	r, err := Parse(path, nil)
	if err != nil {
		t.Fatalf("parse epub: %v", err)
	}
	if len(r.Chapters) != 1 {
		t.Fatalf("expected 1 chapter, got %d", len(r.Chapters))
	}
}

func TestParseEpub_ChaptersWithOnlyTitle(t *testing.T) {
	dir := t.TempDir()
	// 章节有 h1 标题但 body 为空——标题也算有效内容，章节保留
	path := makeMinimalEpub(t, dir, []struct{ title, body string }{
		{"第一章", "有内容的章节。"},
		{"第二章", ""},
		{"第三章", "第三章有内容。"},
	})

	r, err := Parse(path, nil)
	if err != nil {
		t.Fatalf("parse epub: %v", err)
	}
	// 第二章有标题无正文，保留（标题即内容）
	if len(r.Chapters) != 3 {
		t.Fatalf("expected 3 chapters (title-only kept), got %d", len(r.Chapters))
	}
	if !strings.Contains(r.Chapters[1].Content, "第二章") {
		t.Error("chapter 2 should contain its h1 title")
	}
}

func TestParseEpub_NestedContentPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested.epub")
	f, _ := os.Create(path)
	w := zip.NewWriter(f)

	mw, _ := w.CreateHeader(&zip.FileHeader{Name: "mimetype", Method: zip.Store})
	mw.Write([]byte("application/epub+zip"))

	container := `<?xml version="1.0"?><container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
<rootfiles><rootfile full-path="OEBPS/Text/content.opf" media-type="application/oebps-package+xml"/></rootfiles></container>`
	cw, _ := w.Create("META-INF/container.xml")
	cw.Write([]byte(container))

	opf := `<?xml version="1.0"?><package version="2.0" xmlns="http://www.idpf.org/2007/opf">
<metadata><dc:title xmlns:dc="http://purl.org/dc/elements/1.1/">嵌套测试</dc:title></metadata>
<manifest><item id="ch1" href="../Content/chap1.xhtml" media-type="application/xhtml+xml"/></manifest>
<spine><itemref idref="ch1"/></spine></package>`
	ow, _ := w.Create("OEBPS/Text/content.opf")
	ow.Write([]byte(opf))

	html := `<?xml version="1.0"?><!DOCTYPE html><html xmlns="http://www.w3.org/1999/xhtml">
<head><title>第一章</title></head><body><h1>第一章</h1><p>正文。</p></body></html>`
	hw, _ := w.Create("OEBPS/Content/chap1.xhtml")
	hw.Write([]byte(html))

	w.Close()
	f.Close()

	r, err := Parse(path, nil)
	if err != nil {
		t.Fatalf("parse epub: %v", err)
	}
	if len(r.Chapters) != 1 {
		t.Fatalf("expected 1 chapter, got %d", len(r.Chapters))
	}
	if !strings.Contains(r.Chapters[0].Content, "正文") {
		t.Error("content missing")
	}
}

func TestParseEpub_TitleAndEscapedPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "escaped.epub")
	f, _ := os.Create(path)
	w := zip.NewWriter(f)

	mw, _ := w.CreateHeader(&zip.FileHeader{Name: "mimetype", Method: zip.Store})
	mw.Write([]byte("application/epub+zip"))

	container := `<?xml version="1.0"?><container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
<rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles></container>`
	cw, _ := w.Create("META-INF/container.xml")
	cw.Write([]byte(container))

	opf := `<?xml version="1.0"?><package version="2.0" xmlns="http://www.idpf.org/2007/opf">
<metadata><dc:title xmlns:dc="http://purl.org/dc/elements/1.1/">路径测试</dc:title></metadata>
<manifest><item id="ch1" href="%E7%AC%AC%E4%B8%80%E7%AB%A0%5B%E5%BA%8F%5D.xhtml" media-type="application/xhtml+xml"/></manifest>
<spine><itemref idref="ch1"/></spine></package>`
	ow, _ := w.Create("OEBPS/content.opf")
	ow.Write([]byte(opf))

	html := `<?xml version="1.0"?><!DOCTYPE html><html><head><title>第一章</title></head><body><h1>第一章[序]</h1><p>正文。</p></body></html>`
	hw, _ := w.Create("OEBPS/第一章[序].xhtml")
	hw.Write([]byte(html))

	w.Close()
	f.Close()

	r, err := Parse(path, nil)
	if err != nil {
		t.Fatalf("parse epub: %v", err)
	}
	if len(r.Chapters) != 1 {
		t.Fatalf("expected 1 chapter, got %d", len(r.Chapters))
	}
	if r.Chapters[0].Title != "第一章[序]" {
		t.Fatalf("title: got %q, want %q", r.Chapters[0].Title, "第一章[序]")
	}
}

func TestParseEpub_NoMetadataTitle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notitle.epub")
	f, _ := os.Create(path)
	w := zip.NewWriter(f)

	mw, _ := w.CreateHeader(&zip.FileHeader{Name: "mimetype", Method: zip.Store})
	mw.Write([]byte("application/epub+zip"))

	container := `<?xml version="1.0"?><container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
<rootfiles><rootfile full-path="content.opf" media-type="application/oebps-package+xml"/></rootfiles></container>`
	cw, _ := w.Create("META-INF/container.xml")
	cw.Write([]byte(container))

	opf := `<?xml version="1.0"?><package version="2.0" xmlns="http://www.idpf.org/2007/opf">
<metadata><dc:title xmlns:dc="http://purl.org/dc/elements/1.1/"></dc:title></metadata>
<manifest><item id="ch1" href="ch1.xhtml" media-type="application/xhtml+xml"/></manifest>
<spine><itemref idref="ch1"/></spine></package>`
	ow, _ := w.Create("content.opf")
	ow.Write([]byte(opf))

	html := `<?xml version="1.0"?><!DOCTYPE html><html><head><title>ch1</title></head><body><p>正文。</p></body></html>`
	hw, _ := w.Create("ch1.xhtml")
	hw.Write([]byte(html))

	w.Close()
	f.Close()

	r, err := Parse(path, nil)
	if err != nil {
		t.Fatalf("parse epub: %v", err)
	}
	// 空标题直接返回空，由 ImportNovel 层兜底为「未命名」
	if r.Title != "" {
		t.Errorf("expected empty title from empty OPF, got %q", r.Title)
	}
}

func TestParseEpub_InvalidZip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.epub")
	os.WriteFile(path, []byte("not a zip file"), 0644)

	_, err := Parse(path, nil)
	if err == nil {
		t.Fatal("expected error for invalid epub")
	}
}

// ── HTML 文本提取 ──────────────────────────────────────────

func TestExtractHTMLText_Normal(t *testing.T) {
	// 创建临时 epub 配合 extractHTMLText 测试
	dir := t.TempDir()
	path := makeMinimalEpub(t, dir, []struct{ title, body string }{
		{"第一章", "这是测试内容，包含标点符号。"},
	})

	// 直接打开 epub 测试 extractHTMLText
	r, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	text, err := extractHTMLText(r, "OEBPS/chapter01.xhtml")
	if err != nil {
		t.Fatalf("extractHTMLText: %v", err)
	}
	if !strings.Contains(text, "测试内容") {
		t.Errorf("expected '测试内容' in text, got: %s", text)
	}
	if !strings.Contains(text, "标点符号") {
		t.Errorf("expected '标点符号' in text")
	}
}

func TestExtractHTMLText_CaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	path := makeMinimalEpub(t, dir, []struct{ title, body string }{
		{"第一章", "内容。"},
	})

	r, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	// 大小写不同应仍能找到
	text, err := extractHTMLText(r, "oebps/chapter01.xhtml")
	if err != nil {
		t.Fatalf("case-insensitive lookup failed: %v", err)
	}
	if !strings.Contains(text, "内容") {
		t.Error("content mismatch")
	}
}

// ── 章节正则边界 ────────────────────────────────────────────

func TestChapterSepRe(t *testing.T) {
	// 使用 chapterPatterns[0] (strict_line_start) 替代已移除的 chapterMarkerRe
	p := chapterPatterns[0].pattern
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
		// Markdown 前缀
		{"# 第1章 标题", true},
		{"## 第一章 开篇", true},
		// 不应匹配的
		{"这是第5章的内容", false}, // 行首不是「第」
		{"", false},
	}

	for _, tc := range tests {
		got := p.MatchString(tc.line)
		if got != tc.matches {
			t.Errorf("strict_line_start.MatchString(%q) = %v, want %v", tc.line, got, tc.matches)
		}
	}
}

// ── 集成测试：Parse 路由到正确解析器 ────────────────────────

func TestParse_TXTRoute(t *testing.T) {
	path := writeTemp(t, "test.txt", "第1章\n\n内容。\n")
	r, err := Parse(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.Title != "test" {
		t.Errorf("expected title 'test', got %q", r.Title)
	}
}

func TestParse_MDRoute(t *testing.T) {
	path := writeTemp(t, "test.md", "# 第1章\n\n内容。\n")
	r, err := Parse(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.Title != "test" {
		t.Errorf("expected title 'test', got %q", r.Title)
	}
}

func TestParseEpub_EndToEnd(t *testing.T) {
	dir := t.TempDir()
	path := makeMinimalEpub(t, dir, []struct{ title, body string }{
		{"第一章 启程", "少年背上行囊，离开了居住十六年的村庄。"},
		{"第二章 遭遇", "森林深处传来一声低沉的咆哮。"},
	})

	r, err := Parse(path, nil)
	if err != nil {
		t.Fatalf("Parse epub: %v", err)
	}
	if r.Title != "测试小说" {
		t.Errorf("title: got %q, want '测试小说'", r.Title)
	}
	if len(r.Chapters) != 2 {
		t.Fatalf("expected 2 chapters, got %d", len(r.Chapters))
	}
	if !strings.Contains(r.Chapters[0].Content, "少年") {
		t.Error("chapter 1 missing content")
	}
	if !strings.Contains(r.Chapters[1].Content, "咆哮") {
		t.Error("chapter 2 missing content")
	}
}

// ── Benchmark ───────────────────────────────────────────────

func BenchmarkParseTxt(b *testing.B) {
	// 生成 50 章的文本
	var sb strings.Builder
	for i := 1; i <= 50; i++ {
		sb.WriteString("第" + strings.Repeat("0", 3-len(itoa(i))) + itoa(i) + "章 标题\n\n")
		for j := 0; j < 200; j++ {
			sb.WriteString("这是正文内容，用来模拟真实的小说章节。")
		}
		sb.WriteString("\n\n")
	}
	content := sb.String()
	dir := b.TempDir()
	path := filepath.Join(dir, "bench.txt")
	os.WriteFile(path, []byte(content), 0644)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parseTxt(path)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
