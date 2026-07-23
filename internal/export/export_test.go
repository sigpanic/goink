package export

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/sigpanic/goink/internal/chapter"
	"github.com/sigpanic/goink/internal/novel"
)

func testNovel() *novel.Novel {
	return &novel.Novel{
		ID:          1,
		Title:       "测试书名",
		Genre:       "玄幻",
		Description: "一部测试小说",
	}
}

func testChapters() []ChapterWithContent {
	return []ChapterWithContent{
		{Chapter: chapter.Chapter{ChapterNumber: 1, Title: "开头"}, Content: "这是第一章的正文内容。\n\n第二段。"},
		{Chapter: chapter.Chapter{ChapterNumber: 2, Title: "发展"}, Content: "第二章内容。\n\n继续推进剧情。"},
	}
}

func TestExportNovel_UnknownFormat(t *testing.T) {
	_, _, err := ExportNovel(testNovel(), testChapters(), "pdf", "")
	if err == nil {
		t.Fatal("expected error for unknown format")
	}
}

// ── Markdown ──────────────────────────────────────────────

func TestExportMarkdown(t *testing.T) {
	data, filename, err := ExportNovel(testNovel(), testChapters(), "markdown", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(filename, ".md") {
		t.Errorf("expected .md suffix, got %s", filename)
	}

	s := string(data)
	if !strings.Contains(s, "# 测试书名") {
		t.Error("missing title heading")
	}
	if !strings.Contains(s, "## 目录") {
		t.Error("missing TOC heading")
	}
	if !strings.Contains(s, "第1章 开头") {
		t.Error("missing chapter 1")
	}
	if !strings.Contains(s, "第2章 发展") {
		t.Error("missing chapter 2")
	}
	if !strings.Contains(s, "这是第一章的正文内容") {
		t.Error("missing chapter 1 content")
	}
	if !strings.Contains(s, "**类型**: 玄幻") {
		t.Error("missing genre")
	}
}

func TestExportMarkdown_ChapterWithoutTitle(t *testing.T) {
	chs := []ChapterWithContent{
		{Chapter: chapter.Chapter{ChapterNumber: 3, Title: ""}, Content: "第三章内容。"},
	}
	data, _, err := ExportNovel(testNovel(), chs, "markdown", "")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if !strings.Contains(s, "第3章 第3章") {
		t.Error("expected fallback title '第3章'")
	}
}

// ── TXT ───────────────────────────────────────────────────

func TestExportTxt(t *testing.T) {
	data, filename, err := ExportNovel(testNovel(), testChapters(), "txt", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(filename, ".txt") {
		t.Errorf("expected .txt suffix, got %s", filename)
	}

	s := string(data)
	if !strings.Contains(s, "测试书名") {
		t.Error("missing title")
	}
	if !strings.Contains(s, "第1章 开头") {
		t.Error("missing chapter 1 heading")
	}
	if !strings.Contains(s, "这是第一章的正文内容") {
		t.Error("missing chapter 1 content")
	}
	// 不应包含 markdown 标记（但原样输出，不管这个）
}

// ── EPUB ──────────────────────────────────────────────────

func TestExportEpub(t *testing.T) {
	data, filename, err := ExportNovel(testNovel(), testChapters(), "epub", "测试作者")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(filename, ".epub") {
		t.Errorf("expected .epub suffix, got %s", filename)
	}

	// EPUB 本质是 ZIP 文件，验证结构
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("epub is not valid zip: %v", err)
	}

	var hasMimetype, hasContainer, hasPackageOpf bool
	for _, f := range zr.File {
		switch f.Name {
		case "mimetype":
			hasMimetype = true
		case "META-INF/container.xml":
			hasContainer = true
		case "EPUB/package.opf":
			hasPackageOpf = true
		}
	}

	if !hasMimetype {
		t.Error("missing mimetype entry")
	}
	if !hasContainer {
		t.Error("missing META-INF/container.xml")
	}
	if !hasPackageOpf {
		t.Error("missing EPUB/package.opf")
	}

	// 遍历找到 package.opf 检查内容
	for _, f := range zr.File {
		if f.Name == "EPUB/package.opf" {
			rc, _ := f.Open()
			buf := new(bytes.Buffer)
			buf.ReadFrom(rc)
			rc.Close()
			s := buf.String()
			if !strings.Contains(s, "测试书名") {
				t.Error("package.opf missing book title")
			}
			if !strings.Contains(s, "测试作者") {
				t.Error("package.opf missing author")
			}
		}
	}
}

// TestExportEpub_Content 拆开 EPUB 校验各 section 的 HTML 内容和格式。
func TestExportEpub_Content(t *testing.T) {
	n := testNovel()
	chs := testChapters()
	data, _, err := ExportNovel(n, chs, "epub", "测试作者")
	if err != nil {
		t.Fatal(err)
	}

	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}

	// 收集所有 section XHTML 文件内容
	sections := make(map[string]string) // filename → content
	for _, f := range zr.File {
		if strings.HasPrefix(f.Name, "EPUB/xhtml/") {
			rc, err := f.Open()
			if err != nil {
				t.Fatalf("open %s: %v", f.Name, err)
			}
			buf := new(bytes.Buffer)
			io.Copy(buf, rc)
			rc.Close()
			sections[f.Name] = buf.String()
		}
	}

	if len(sections) != 2 {
		t.Errorf("expected 2 sections, got %d", len(sections))
	}

	for _, html := range sections {
		// 必须有 <style> 内联 CSS
		if !strings.Contains(html, "<style>") {
			t.Error("section missing <style>")
		}
		if !strings.Contains(html, "font-family") {
			t.Error("section CSS missing font-family")
		}
		if !strings.Contains(html, "text-indent") {
			t.Error("section CSS missing text-indent")
		}

		// 章节标题渲染为 <h1>
		if !strings.Contains(html, "<h1>") {
			t.Error("section missing <h1>")
		}

		// 正文应有 <p> 标签（goldmark 转换后的段落）
		if !strings.Contains(html, "<p>") {
			t.Error("section missing <p> tags — goldmark may not have converted markdown correctly")
		}

		// 不应残留原始 markdown 标记
		if strings.Contains(html, "**") || strings.Contains(html, "##") {
			t.Error("section contains raw markdown syntax — goldmark should have converted it")
		}
	}

	// 校验第1章的具体内容
	ch1Name := fmt.Sprintf("EPUB/xhtml/%s", "第1章 开头")
	ch1, ok := sections[ch1Name]
	if !ok {
		t.Fatalf("missing section %s", ch1Name)
	}
	if !strings.Contains(ch1, "<h1>第1章 开头</h1>") {
		t.Error("chapter 1 heading missing or incorrect")
	}
	if !strings.Contains(ch1, "这是第一章的正文内容") {
		t.Error("chapter 1 body text missing")
	}
}

func TestExportEpub_FallbackAuthor(t *testing.T) {
	data, _, err := ExportNovel(testNovel(), testChapters()[:1], "epub", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range zipReader(t, data).File {
		if f.Name == "EPUB/package.opf" {
			rc, _ := f.Open()
			buf := new(bytes.Buffer)
			buf.ReadFrom(rc)
			rc.Close()
			if !strings.Contains(buf.String(), "Goink") {
				t.Error("expected fallback author Goink")
			}
		}
	}
}

func zipReader(t *testing.T, data []byte) *zip.Reader {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	return zr
}

// ── safeFilename ──────────────────────────────────────────

func TestSafeFilename(t *testing.T) {
	tests := []struct{ in, want string }{
		{"正常书名", "正常书名"},
		{"书:名", "书_名"},
		{"a<b>c", "a_b_c"},
		{"test/name", "test_name"},
		{"  spaces  ", "spaces"},
	}
	for _, tc := range tests {
		got := safeFilename(tc.in)
		if got != tc.want {
			t.Errorf("safeFilename(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
