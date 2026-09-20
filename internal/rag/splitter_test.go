package rag

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// testTokenizer 加载真实词表供测试使用。若 vocab.txt 不可用则跳过。
func testTokenizer(t *testing.T) *Tokenizer {
	t.Helper()
	tok, err := NewTokenizer("../../build/runtime/models/vocab.txt")
	if err != nil {
		t.Skipf("skipping: cannot load vocab.txt: %v", err)
	}
	return tok
}

// verifyPositions 检查 positions 基本性质：非负、递增、不越界。
func verifyPositions(t *testing.T, text string, positions []int, chunks []string, overlap int) {
	t.Helper()
	if len(positions) != len(chunks) {
		t.Fatalf("positions len %d != chunks len %d", len(positions), len(chunks))
	}
	contentRunes := []rune(text)
	for i := range chunks {
		pos := positions[i]
		if pos < 0 {
			t.Errorf("chunk %d: negative position %d", i, pos)
		}
		if pos > len(contentRunes) {
			t.Errorf("chunk %d: position %d beyond content length %d", i, pos, len(contentRunes))
		}
		if i > 0 && pos <= positions[i-1] {
			t.Errorf("chunk %d: position %d <= previous %d", i, pos, positions[i-1])
		}
		// 取跳过 overlap 后的 chunk 内容前 10 字，检查是否在原文 pos+overlap 附近
		chunkRunes := []rune(chunks[i])
		skip := 0
		if i > 0 {
			skip = min(overlap, len(chunkRunes))
		}
		if skip >= len(chunkRunes) {
			continue
		}
		bodyStart := skip
		bodyEnd := min(bodyStart+10, len(chunkRunes))
		body := string(chunkRunes[bodyStart:bodyEnd])
		searchPos := pos + utf8.RuneCountInString(string(chunkRunes[:skip]))
		if searchPos >= len(contentRunes) {
			continue
		}
		searchEnd := min(searchPos+len(body)+20, len(contentRunes))
		region := string(contentRunes[searchPos:searchEnd])
		if !strings.Contains(region, body) {
			t.Errorf("chunk %d: chunk body %q not found near original pos %d (search region: …%s…)", i, body, searchPos, region[:min(30, len(region))])
		}
	}
}

func TestSplitText_Short(t *testing.T) {
	tok := testTokenizer(t)
	text := "主角张三走进房间，看到李四正在等待。"
	chunks, positions := SplitText(text, 500, 50, tok)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d: %v", len(chunks), chunks)
	}
	if chunks[0] != text {
		t.Errorf("content mismatch: %q", chunks[0])
	}
	if positions[0] != 0 {
		t.Errorf("expected position 0 for single chunk, got %d", positions[0])
	}
}

func TestSplitText_LongParagraph(t *testing.T) {
	tok := testTokenizer(t)
	p1 := strings.Repeat("第一章内容。", 50)
	p2 := strings.Repeat("第二章内容。", 50)
	text := p1 + "\n" + p2

	chunks, positions := SplitText(text, 350, 50, tok)
	if len(chunks) < 2 {
		t.Fatalf("expected >=2 chunks, got %d", len(chunks))
	}
	verifyPositions(t, text, positions, chunks, 50)
}

func TestSplitText_SentenceSplit(t *testing.T) {
	tok := testTokenizer(t)
	text := strings.Repeat("这是一句很长的话反复说。", 60)

	chunks, positions := SplitText(text, 500, 50, tok)
	if len(chunks) < 2 {
		t.Fatalf("expected >=2 chunks for long sentence, got %d", len(chunks))
	}
	verifyPositions(t, text, positions, chunks, 50)
	// 有重叠的情况下，chunk 可能略超 chunkSize
	for i, c := range chunks {
		if tok.TokenCount(c) > 600 {
			t.Errorf("chunk %d unreasonably long: %d tokens", i, tok.TokenCount(c))
		}
	}
}

func TestSplitText_Empty(t *testing.T) {
	tok := testTokenizer(t)
	if chunks, positions := SplitText("", 500, 50, tok); chunks != nil || positions != nil {
		t.Errorf("expected nil,nil for empty text, got chunks=%v positions=%v", chunks, positions)
	}
}

func TestSplitText_Overlap(t *testing.T) {
	tok := testTokenizer(t)
	// 多块文本，验证重叠
	p1 := strings.Repeat("段落一内容。", 30)
	p2 := strings.Repeat("段落二内容。", 30)
	p3 := strings.Repeat("段落三内容。", 30)
	text := p1 + "\n" + p2 + "\n" + p3

	chunks, positions := SplitText(text, 500, 80, tok)
	if len(chunks) < 2 {
		t.Fatalf("expected >=2 chunks, got %d", len(chunks))
	}
	verifyPositions(t, text, positions, chunks, 80)

	// 验证第二块的开头包含第一块的尾部内容
	for i := 1; i < len(chunks); i++ {
		prev := []rune(chunks[i-1])
		curr := []rune(chunks[i])
		if len(prev) < 80 || len(curr) < 80 {
			continue
		}
		// 第一块的最后 overlap 字符应等于第二块的前 overlap 字符
		prevTail := string(prev[len(prev)-80:])
		currHead := string(curr[:80])
		if prevTail != currHead {
			t.Errorf("chunk %d overlap mismatch:\n  prev tail: %q\n  curr head: %q", i, prevTail, currHead)
		}
	}
}

func TestBuildChapterChunks(t *testing.T) {
	tok := testTokenizer(t)
	params := ChapterChunkParams{
		ChapterID:     101,
		ReadingNumber: 1,
		ChapterTitle:  "初入江湖",
		Content:       "张三背着行囊踏进了江湖。\n\n这是他人生的新篇章。故事从这里开始。",
		Summary:       "主角离开家乡，开始了江湖之旅。",
	}

	chunks := BuildChapterChunks(params, tok)
	if len(chunks) < 2 {
		t.Fatalf("expected at least 2 chunks, got %d", len(chunks))
	}

	var summaryFound, briefFound bool
	for _, c := range chunks {
		switch c.ChunkType {
		case "summary":
			summaryFound = true
			if c.Content != params.Summary {
				t.Errorf("summary content mismatch: %q", c.Content)
			}
			if c.ID != "101_summary" {
				t.Errorf("summary ID mismatch: %q", c.ID)
			}
			if c.Metadata == nil {
				t.Error("summary chunk missing metadata")
			} else {
				if v, ok := c.Metadata["chapter_id"].(int64); !ok || v != 101 {
					t.Errorf("summary chapter_id: %v", c.Metadata["chapter_id"])
				}
			}
		case "chapter_brief":
			briefFound = true
			if !strings.Contains(c.Content, params.ChapterTitle) {
				t.Errorf("brief missing title: %q", c.Content)
			}
			if c.ID != "101_brief" {
				t.Errorf("brief ID mismatch: %q", c.ID)
			}
		}
	}
	if !summaryFound {
		t.Error("summary chunk not found")
	}
	if !briefFound {
		t.Error("chapter_brief chunk not found")
	}

	// 验证 content chunk 的 StartRunePos
	contentRunes := []rune(params.Content)
	for _, c := range chunks {
		if c.ChunkType != "content" {
			continue
		}
		if c.StartRunePos < 0 {
			t.Errorf("content chunk %s: negative StartRunePos %d", c.ID, c.StartRunePos)
		}
		if c.StartRunePos > len(contentRunes) {
			t.Errorf("content chunk %s: StartRunePos %d beyond content length %d", c.ID, c.StartRunePos, len(contentRunes))
		}
	}
}

func TestBuildChapterChunks_NoSummary(t *testing.T) {
	tok := testTokenizer(t)
	params := ChapterChunkParams{
		ChapterID:     102,
		ReadingNumber: 2,
		ChapterTitle:  "",
		Content:       "正文内容正文内容。",
		Summary:       "",
	}

	chunks := BuildChapterChunks(params, tok)
	for _, c := range chunks {
		if c.ChunkType == "summary" {
			t.Error("summary chunk should not exist when summary is empty")
		}
	}
	briefFound := false
	for _, c := range chunks {
		if c.ChunkType == "chapter_brief" {
			briefFound = true
			if !strings.Contains(c.Content, "第2章") {
				t.Errorf("expected default title '第2章' in brief: %q", c.Content)
			}
		}
	}
	if !briefFound {
		t.Error("chapter_brief not found")
	}
}

func TestBuildChapterChunks_EmptyContent(t *testing.T) {
	tok := testTokenizer(t)
	params := ChapterChunkParams{
		ChapterID:     103,
		ReadingNumber: 3,
		ChapterTitle:  "空章",
		Content:       "",
		Summary:       "",
	}

	chunks := BuildChapterChunks(params, tok)
	if len(chunks) != 1 {
		for _, c := range chunks {
			t.Logf("  chunk type=%s id=%s content=%q", c.ChunkType, c.ID, c.Content)
		}
		t.Fatalf("expected 1 chunk (brief only), got %d", len(chunks))
	}
	if chunks[0].ChunkType != "chapter_brief" {
		t.Errorf("expected chapter_brief, got %s", chunks[0].ChunkType)
	}
}

func TestSplitText_NoEmptySentences(t *testing.T) {
	tok := testTokenizer(t)
	// 连续标点不应产生空句子
	text := "你好！！世界"
	chunks, positions := SplitText(text, 500, 0, tok)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0] != text {
		t.Errorf("content mismatch: %q", chunks[0])
	}
	if positions[0] != 0 {
		t.Errorf("expected position 0, got %d", positions[0])
	}
}
