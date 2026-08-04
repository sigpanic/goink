package imp

import (
	"strings"
	"testing"
)

// ── cleanLLMPatternOutput: (?m) 多行模式前缀补全 ─────────────
// LLM 可能漏掉 (?m) flag，导致 ^ 只匹配全文开头而漏匹配章节标题。
// cleanLLMPatternOutput 负责在清理后补全 (?m) 前缀。

func TestCleanLLMPatternOutput_AddMultilineFlag(t *testing.T) {
	// LLM 漏掉 (?m)，应自动补上
	got := cleanLLMPatternOutput(`^第\d+章`)
	want := `(?m)^第\d+章`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCleanLLMPatternOutput_KeepMultilineFlag(t *testing.T) {
	// 已有 (?m)，不重复补
	got := cleanLLMPatternOutput(`(?m)^第\d+章`)
	want := `(?m)^第\d+章`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCleanLLMPatternOutput_OtherFlagsPreserved(t *testing.T) {
	// 含其他 flag（如 (?i)），(?m) 前置，原 flag 保留
	got := cleanLLMPatternOutput(`(?i)^Chapter`)
	want := `(?m)(?i)^Chapter`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCleanLLMPatternOutput_MergedFlagsPreserved(t *testing.T) {
	// LLM 输出合并 flag 形式 (?mi)，(?m) 前置后仍合法（Go regexp 接受重复 flag）
	got := cleanLLMPatternOutput(`(?mi)^Chapter`)
	// (?m) 补到前面，原 (?mi) 保留，m 重复设置无害
	if !strings.HasPrefix(got, `(?m)`) {
		t.Errorf("expected (?m) prefix, got %q", got)
	}
	if !strings.Contains(got, `(?mi)`) {
		t.Errorf("expected original (?mi) preserved, got %q", got)
	}
}

func TestCleanLLMPatternOutput_MarkdownStripped(t *testing.T) {
	// markdown 代码块标记去除 + (?m) 补全
	input := "```go\n^第\\d+章\n```"
	got := cleanLLMPatternOutput(input)
	want := `(?m)^第\d+章`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCleanLLMPatternOutput_QuotesStripped(t *testing.T) {
	// 各类引号去除 + (?m) 补全
	tests := []struct {
		input, want string
	}{
		{"`^第\\d+章`", `(?m)^第\d+章`},
		{`"^第\d+章"`, `(?m)^第\d+章`},
		{`'^第\d+章'`, `(?m)^第\d+章`},
	}
	for _, tc := range tests {
		got := cleanLLMPatternOutput(tc.input)
		if got != tc.want {
			t.Errorf("input %q: got %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestCleanLLMPatternOutput_NoAnchor(t *testing.T) {
	// 正则不含 ^（直接匹配内容），补 (?m) 也无害
	got := cleanLLMPatternOutput(`第\d+章`)
	want := `(?m)第\d+章`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// ── ParseWithLLMPattern: (?m) 补全后的分割行为 ──────────────

func TestParseWithLLMPattern_MultilineAutoAdded(t *testing.T) {
	// 开头不是章节标题，多行章节标题分散在文中
	// 若 (?m) 未生效，^ 只匹配全文开头，会匹配不到任何章节
	content := "书名：测试小说\n简介。\n\n" +
		"第1章 开始\n\n正文一。\n\n" +
		"第2章 发展\n\n正文二。\n\n" +
		"第3章 高潮\n\n正文三。\n"
	path := writeTemp(t, "llm_multiline.txt", content)

	// 模拟 AnalyzeWithLLM 真实流程：LLM 输出原始正则 → cleanLLMPatternOutput 补 (?m) → ParseWithLLMPattern
	rawPattern := `^第\d+章`
	cleanedPattern := cleanLLMPatternOutput(rawPattern)
	r, err := ParseWithLLMPattern(path, cleanedPattern)
	if err != nil {
		t.Fatalf("ParseWithLLMPattern failed: %v", err)
	}
	if len(r.Chapters) != 3 {
		t.Fatalf("expected 3 chapters after (?m) auto-added, got %d", len(r.Chapters))
	}
	// 验证 ^ 按行匹配：第一章不应包含开头的前言
	if strings.Contains(r.Chapters[0].Content, "书名：测试小说") {
		t.Errorf("first chapter should not contain preamble, got: %q", r.Chapters[0].Content)
	}
}

func TestParseWithLLMPattern_AlreadyHasMultiline(t *testing.T) {
	// LLM 返回的正则已有 (?m)，正常分割
	content := "序言。\n\n" +
		"第1章 开始\n\n正文一。\n\n" +
		"第2章 结束\n\n正文二。\n"
	path := writeTemp(t, "llm_has_m.txt", content)

	r, err := ParseWithLLMPattern(path, `(?m)^第\d+章`)
	if err != nil {
		t.Fatalf("ParseWithLLMPattern failed: %v", err)
	}
	if len(r.Chapters) != 2 {
		t.Fatalf("expected 2 chapters, got %d", len(r.Chapters))
	}
}
