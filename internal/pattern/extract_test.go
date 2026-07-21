package pattern

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"novel/internal/chapter"
	"novel/internal/llm"
	"novel/internal/skill"
)

// ---------------------------------------------------------------------------
// batchBudget
// ---------------------------------------------------------------------------

func TestBatchBudget_LargeWindow(t *testing.T) {
	got := batchBudget(200_000)
	if got != 100_000 {
		t.Errorf("batchBudget(200000) = %d, want 100000", got)
	}
}

func TestBatchBudget_SmallWindow(t *testing.T) {
	got := batchBudget(128_000)
	want := int(float64(128_000) * 0.7) // 89600
	if got != want {
		t.Errorf("batchBudget(128000) = %d, want %d", got, want)
	}
}

func TestBatchBudget_ZeroWindow(t *testing.T) {
	got := batchBudget(0)
	if got != 0 {
		t.Errorf("batchBudget(0) = %d, want 0", got)
	}
}

// ---------------------------------------------------------------------------
// normalizeBoundaries
// ---------------------------------------------------------------------------

func TestNormalizeBoundaries_Normal(t *testing.T) {
	input := []BoundaryHint{
		{StartChapter: 5, EndChapter: 10, Hint: "第一幕结束"},
		{StartChapter: 1, EndChapter: 4, Hint: "开篇"},
	}
	got := normalizeBoundaries(input)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].StartChapter != 1 {
		t.Errorf("first item StartChapter = %d, want 1", got[0].StartChapter)
	}
	if got[1].StartChapter != 5 {
		t.Errorf("second item StartChapter = %d, want 5", got[1].StartChapter)
	}
}

func TestNormalizeBoundaries_SwapRange(t *testing.T) {
	input := []BoundaryHint{
		{StartChapter: 10, EndChapter: 5, Hint: "逆序"},
	}
	got := normalizeBoundaries(input)
	if got[0].StartChapter != 5 || got[0].EndChapter != 10 {
		t.Errorf("swapped = %+v, want Start=5 End=10", got[0])
	}
}

func TestNormalizeBoundaries_FilterInvalid(t *testing.T) {
	input := []BoundaryHint{
		{StartChapter: 1, EndChapter: 5, Hint: "valid"},
		{StartChapter: 0, EndChapter: 3, Hint: "zero start"},
		{StartChapter: -1, EndChapter: 5, Hint: "negative"},
		{StartChapter: 2, EndChapter: 0, Hint: "zero end"},
	}
	got := normalizeBoundaries(input)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1 (only valid)", len(got))
	}
	if got[0].Hint != "valid" {
		t.Errorf("hint = %q, want valid", got[0].Hint)
	}
}

func TestNormalizeBoundaries_Empty(t *testing.T) {
	got := normalizeBoundaries(nil)
	if len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}
}

// ---------------------------------------------------------------------------
// normalizeChunks
// ---------------------------------------------------------------------------

func TestNormalizeChunks_Normal(t *testing.T) {
	input := []Chunk{
		{Name: "转折", StartChapter: 10, EndChapter: 15, Content: "主人公觉醒"},
		{Name: "开篇", StartChapter: 1, EndChapter: 9, Content: "平凡日常"},
	}
	got := normalizeChunks(input)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].StartChapter != 1 {
		t.Errorf("first StartChapter = %d, want 1", got[0].StartChapter)
	}
}

func TestNormalizeChunks_SwapRange(t *testing.T) {
	input := []Chunk{
		{Name: "test", StartChapter: 20, EndChapter: 10, Content: "逆序范围"},
	}
	got := normalizeChunks(input)
	if got[0].StartChapter != 10 || got[0].EndChapter != 20 {
		t.Errorf("swapped = %+v, want Start=10 End=20", got[0])
	}
}

func TestNormalizeChunks_FilterEmpty(t *testing.T) {
	input := []Chunk{
		{Name: "valid", StartChapter: 1, EndChapter: 5, Content: "内容"},
		{Name: "", StartChapter: 1, EndChapter: 5, Content: "无名称"},
		{Name: "无内容", StartChapter: 1, EndChapter: 5, Content: ""},
		{Name: "全是空格", StartChapter: 1, EndChapter: 5, Content: "   "},
	}
	got := normalizeChunks(input)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].Name != "valid" {
		t.Errorf("name = %q, want valid", got[0].Name)
	}
}

func TestNormalizeChunks_FilterInvalid(t *testing.T) {
	input := []Chunk{
		{Name: "ok", StartChapter: 1, EndChapter: 5, Content: "ok"},
		{Name: "bad", StartChapter: 0, EndChapter: 5, Content: "zero start"},
		{Name: "bad2", StartChapter: 1, EndChapter: -1, Content: "neg end"},
	}
	got := normalizeChunks(input)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
}

// ---------------------------------------------------------------------------
// buildChapterBatches
// ---------------------------------------------------------------------------

func makeChapters(count, contentLen int) []ChapterSource {
	out := make([]ChapterSource, count)
	for i := range count {
		out[i] = ChapterSource{
			ID:            int64(i + 1),
			ChapterNumber: i + 1,
			Title:         "章节标题",
			Content:       strings.Repeat("a", contentLen),
		}
	}
	return out
}

func TestBuildChapterBatches_SingleBatch(t *testing.T) {
	chapters := makeChapters(3, 100)
	budget := 10_000
	batches := buildChapterBatches(chapters, nil, budget)
	if len(batches) != 1 {
		t.Fatalf("len = %d, want 1", len(batches))
	}
	if len(batches[0]) != 3 {
		t.Errorf("batch[0] len = %d, want 3", len(batches[0]))
	}
}

func TestBuildChapterBatches_MultiBatch(t *testing.T) {
	// 每章约 25 tokens (100 ASCII chars), 预算 60 tokens -> 大约 2 章一批
	chapters := makeChapters(10, 100)
	budget := 60
	batches := buildChapterBatches(chapters, nil, budget)
	if len(batches) < 2 {
		t.Errorf("expected multiple batches, got %d", len(batches))
	}
	// 验证所有章节都被分配
	total := 0
	for _, b := range batches {
		total += len(b)
	}
	if total != 10 {
		t.Errorf("total chapters = %d, want 10", total)
	}
}

func TestBuildChapterBatches_BoundaryAlignment(t *testing.T) {
	chapters := makeChapters(10, 100) // 每章约 50 tokens
	boundaries := []BoundaryHint{
		{StartChapter: 1, EndChapter: 4, Hint: "第一阶段"},
		{StartChapter: 5, EndChapter: 7, Hint: "第二阶段"},
		{StartChapter: 8, EndChapter: 10, Hint: "第三阶段"},
	}
	// 小预算，强制分批，但边界对齐可能让切分点落在边界上
	budget := 300
	batches := buildChapterBatches(chapters, boundaries, budget)
	total := 0
	for _, b := range batches {
		total += len(b)
	}
	if total != 10 {
		t.Errorf("total chapters = %d, want 10", total)
	}
}

// ---------------------------------------------------------------------------
// buildSummaryBatches
// ---------------------------------------------------------------------------

func TestBuildSummaryBatches(t *testing.T) {
	items := make([]ChapterSummaryItem, 20)
	for i := range items {
		items[i] = ChapterSummaryItem{
			ChapterNumber: i + 1,
			Summary:       strings.Repeat("摘要内容", 10), // ~40 chars
		}
	}
	// 每条约 20+16=36 tokens, 预算 100 -> 2-3 条/批
	batches := buildSummaryBatches(items, 100)
	if len(batches) < 2 {
		t.Errorf("expected multiple batches, got %d", len(batches))
	}
	total := 0
	for _, b := range batches {
		total += len(b)
	}
	if total != 20 {
		t.Errorf("total items = %d, want 20", total)
	}
}

// ---------------------------------------------------------------------------
// buildChunkBatches
// ---------------------------------------------------------------------------

func TestBuildChunkBatches(t *testing.T) {
	chunks := make([]Chunk, 10)
	for i := range chunks {
		chunks[i] = Chunk{
			Name:         "阶段",
			StartChapter: i*10 + 1,
			EndChapter:   (i + 1) * 10,
			Content:      strings.Repeat("内容概括", 20), // ~80 chars
		}
	}
	// 每条约 40+32=72 tokens, 预算 200 -> 2-3 条/批
	batches := buildChunkBatches(chunks, 200)
	if len(batches) < 2 {
		t.Errorf("expected multiple batches, got %d", len(batches))
	}
	total := 0
	for _, b := range batches {
		total += len(b)
	}
	if total != 10 {
		t.Errorf("total chunks = %d, want 10", total)
	}
}

// ---------------------------------------------------------------------------
// tokensOfChunks / tokensOfSummaries
// ---------------------------------------------------------------------------

func TestTokensOfChunks(t *testing.T) {
	chunks := []Chunk{
		{Name: "开篇", StartChapter: 1, EndChapter: 5, Content: "平凡日常的开始"},
		{Name: "转折", StartChapter: 6, EndChapter: 10, Content: "命运的改变"},
	}
	got := tokensOfChunks(chunks)
	// 手动算一次确保一致
	manual := 0
	for _, ch := range chunks {
		manual += approxTokens(ch.Name+ch.Content) + 32
	}
	if got != manual {
		t.Errorf("tokensOfChunks = %d, manual = %d", got, manual)
	}
}

func TestTokensOfSummaries(t *testing.T) {
	items := []ChapterSummaryItem{
		{ChapterNumber: 1, Summary: "开篇叙事"},
		{ChapterNumber: 2, Summary: "冲突升级"},
	}
	got := tokensOfSummaries(items)
	manual := 0
	for _, item := range items {
		manual += approxTokens(item.Summary) + 16
	}
	if got != manual {
		t.Errorf("tokensOfSummaries = %d, manual = %d", got, manual)
	}
}

// ---------------------------------------------------------------------------
// sanitizeFileName
// ---------------------------------------------------------------------------

func TestSanitizeFileName_Normal(t *testing.T) {
	got := skill.SanitizeFileName("我的模式")
	if got != "我的模式" {
		t.Errorf("got %q, want 我的模式", got)
	}
}

func TestSanitizeFileName_SpecialChars(t *testing.T) {
	got := skill.SanitizeFileName("a/b:c*d?e\"f<g>h|i")
	if got != "abcdefghi" {
		t.Errorf("got %q, want abcdefghi", got)
	}
}

func TestSanitizeFileName_Empty(t *testing.T) {
	got := skill.SanitizeFileName("   ")
	if got != "unnamed" {
		t.Errorf("got %q, want unnamed", got)
	}
}

// ---------------------------------------------------------------------------
// approxTokens
// ---------------------------------------------------------------------------

func TestApproxTokens_ASCII(t *testing.T) {
	// tiktoken 可用时会精确计算，不可用时退化为 rune/2
	got := approxTokens("hello world")
	if got <= 0 {
		t.Errorf("approxTokens for ASCII should be > 0, got %d", got)
	}
}

func TestApproxTokens_Chinese(t *testing.T) {
	got := approxTokens("这是一段中文内容")
	if got <= 0 {
		t.Errorf("approxTokens for Chinese should be > 0, got %d", got)
	}
}

func TestApproxTokens_Empty(t *testing.T) {
	got := approxTokens("")
	if got != 0 {
		t.Errorf("approxTokens empty = %d, want 0", got)
	}
}

// ---------------------------------------------------------------------------
// maybeExtendToBoundary
// ---------------------------------------------------------------------------

func TestMaybeExtendToBoundary_Found(t *testing.T) {
	chapters := makeChapters(15, 50)
	boundaries := []BoundaryHint{
		{StartChapter: 1, EndChapter: 7, Hint: "第一阶段结束"},
	}
	// start=5, remaining 足够容纳到第7章
	got := maybeExtendToBoundary(chapters, 5, boundaries, 500)
	if got <= 5 {
		t.Errorf("should extend past 5 when boundary found, got %d", got)
	}
}

func TestMaybeExtendToBoundary_NotFound(t *testing.T) {
	chapters := makeChapters(15, 50)
	// 没有边界匹配
	got := maybeExtendToBoundary(chapters, 5, nil, 500)
	if got != 5 {
		t.Errorf("no boundary = start unchanged, got %d, want 5", got)
	}
}

func TestMaybeExtendToBoundary_ExceedsRemaining(t *testing.T) {
	chapters := makeChapters(15, 5000) // 每章 ~2500 tokens
	boundaries := []BoundaryHint{
		{StartChapter: 1, EndChapter: 10, Hint: "远"},
	}
	// remaining=100 不够容纳第5章 (5000 chars -> ~2500 tokens)
	got := maybeExtendToBoundary(chapters, 5, boundaries, 100)
	if got != 5 {
		t.Errorf("should not extend when tokens exceed remaining, got %d, want 5", got)
	}
}

// ---------------------------------------------------------------------------
// chapterInList
// ---------------------------------------------------------------------------

func TestChapterInList_Found(t *testing.T) {
	list := []ChapterSource{{ChapterNumber: 3}, {ChapterNumber: 7}}
	if !chapterInList(3, list) {
		t.Error("chapter 3 should be in list")
	}
}

func TestChapterInList_NotFound(t *testing.T) {
	list := []ChapterSource{{ChapterNumber: 3}, {ChapterNumber: 7}}
	if chapterInList(5, list) {
		t.Error("chapter 5 should not be in list")
	}
}

// ---------------------------------------------------------------------------
// prompt message formatting
// ---------------------------------------------------------------------------

func TestBoundaryMessages_Format(t *testing.T) {
	chapters := []ChapterSource{
		{ChapterNumber: 1, Title: "初遇"},
		{ChapterNumber: 2, Title: ""},
	}
	msgs := boundaryMessages(chapters)
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	userContent, ok := msgs[1]["content"].(string)
	if !ok {
		t.Fatal("user message content is not string")
	}
	if !strings.Contains(userContent, "第1章：初遇") {
		t.Error("should contain chapter 1 title")
	}
	if !strings.Contains(userContent, "第2章（无标题）") {
		t.Error("should contain fallback for empty title")
	}
}

func TestSummaryMessages_WithBoundaries(t *testing.T) {
	chapters := []ChapterSource{
		{ChapterNumber: 1, Title: "开始", Summary: "已有摘要", Content: "正文"},
		{ChapterNumber: 2, Title: "发展", Summary: "", Content: "新正文"},
	}
	boundaries := []BoundaryHint{
		{StartChapter: 1, EndChapter: 5, Hint: "第一阶段"},
	}
	msgs := summaryMessages(chapters, boundaries)
	userContent, ok := msgs[1]["content"].(string)
	if !ok {
		t.Fatal("user message content is not string")
	}
	if !strings.Contains(userContent, "疑似阶段边界参考") {
		t.Error("should contain boundary reference section")
	}
	if !strings.Contains(userContent, "[上下文参考") {
		t.Error("should mark chapters with existing summary as context reference")
	}
	if !strings.Contains(userContent, "[需提取]") {
		t.Error("should mark chapters without summary as needing extraction")
	}
}

func TestSummaryMessages_NoBoundaries(t *testing.T) {
	chapters := []ChapterSource{
		{ChapterNumber: 1, Title: "开始", Content: "正文"},
	}
	msgs := summaryMessages(chapters, nil)
	userContent, ok := msgs[1]["content"].(string)
	if !ok {
		t.Fatal("user message content is not string")
	}
	if strings.Contains(userContent, "疑似阶段边界参考") {
		t.Error("should not contain boundary section when no boundaries")
	}
}

func TestInitialChunkMessages_Format(t *testing.T) {
	summaries := []ChapterSummaryItem{
		{ChapterNumber: 1, Summary: "开篇叙事"},
	}
	msgs := initialChunkMessages(summaries)
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	userContent, ok := msgs[1]["content"].(string)
	if !ok {
		t.Fatal("user message content is not string")
	}
	if !strings.Contains(userContent, "开篇叙事") {
		t.Error("should contain summary text")
	}
}

func TestCompressChunkMessages_ContainsRound(t *testing.T) {
	chunks := []Chunk{
		{Name: "崛起", StartChapter: 1, EndChapter: 10, Content: "主角崛起"},
	}
	msgs := compressChunkMessages(chunks, 3)
	userContent, ok := msgs[1]["content"].(string)
	if !ok {
		t.Fatal("user message content is not string")
	}
	if !strings.Contains(userContent, "第 3 轮压缩") {
		t.Error("should contain round number")
	}
}

func TestFinalSkillMessages_Format(t *testing.T) {
	chunks := []Chunk{
		{Name: "终局", StartChapter: 50, EndChapter: 60, Content: "大结局"},
	}
	msgs := finalSkillMessages(chunks)
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	sysContent, ok := msgs[0]["content"].(string)
	if !ok {
		t.Fatal("system message content is not string")
	}
	if !strings.Contains(sysContent, "套路模板") {
		t.Error("system prompt should mention 套路模板")
	}
}

// ---------------------------------------------------------------------------
// callOptions
// ---------------------------------------------------------------------------

func TestCallOptions_WithReasoningEffort(t *testing.T) {
	input := ExtractPatternInput{ReasoningEffort: "low"}
	opts := callOptions(input)
	if opts.ReasoningEffort == nil || *opts.ReasoningEffort != "low" {
		t.Error("ReasoningEffort should be 'low'")
	}
}

func TestCallOptions_WithoutReasoningEffort(t *testing.T) {
	input := ExtractPatternInput{}
	opts := callOptions(input)
	if opts.ReasoningEffort != nil {
		t.Errorf("ReasoningEffort should be nil, got %v", opts.ReasoningEffort)
	}
}

// ---------------------------------------------------------------------------
// Extract validation — these tests only cover the early-exit checks before
// any LLM/DB/git interaction. We use zero-value struct pointers so the
// nil-checks pass but loadChapters fails gracefully afterward.
// ---------------------------------------------------------------------------

func TestExtract_NilChapters(t *testing.T) {
	e := &Extractor{Chapters: nil, LLMClient: nil}
	_, err := e.Extract(context.TODO(), ExtractPatternInput{NovelID: 1, ProviderName: "p", ModelID: "m"})
	if err == nil {
		t.Fatal("expected error for nil Chapters")
	}
	if !strings.Contains(err.Error(), "章节存储") {
		t.Errorf("error = %q, should mention 章节存储", err.Error())
	}
}

func TestExtract_NilLLM(t *testing.T) {
	// &chapter.Store{} is non-nil so the Chapters check passes,
	// but we expect LLM nil-check to fire first.
	e := &Extractor{
		Chapters:  &chapter.Store{},
		LLMClient: nil,
	}
	_, err := e.Extract(context.TODO(), ExtractPatternInput{NovelID: 1, ProviderName: "p", ModelID: "m"})
	if err == nil {
		t.Fatal("expected error for nil LLMClient")
	}
	if !strings.Contains(err.Error(), "LLM") {
		t.Errorf("error = %q, should mention LLM", err.Error())
	}
}

func TestExtract_InvalidNovelID(t *testing.T) {
	e := &Extractor{
		Chapters:  &chapter.Store{},
		LLMClient: &llm.Client{},
	}
	_, err := e.Extract(context.TODO(), ExtractPatternInput{NovelID: 0, ProviderName: "p", ModelID: "m"})
	if err == nil {
		t.Fatal("expected error for novel_id <= 0")
	}
	if !strings.Contains(err.Error(), "novel_id") {
		t.Errorf("error = %q, should mention novel_id", err.Error())
	}
}

func TestExtract_EmptyProvider(t *testing.T) {
	e := &Extractor{
		Chapters:  &chapter.Store{},
		LLMClient: &llm.Client{},
	}
	_, err := e.Extract(context.TODO(), ExtractPatternInput{NovelID: 1, ProviderName: "", ModelID: "m"})
	if err == nil {
		t.Fatal("expected error for empty provider")
	}
	if !strings.Contains(err.Error(), "provider_name") {
		t.Errorf("error = %q, should mention provider_name", err.Error())
	}
}

// ===========================================================================
// Mock LLM integration tests
//
// 使用 httptest.Server 模拟 OpenAI-compatible SSE API，测试 Pipeline 子步骤
// 的端到端逻辑（分批、Tool Calling 解析、流式输出等）。
// ===========================================================================

const testProvider = "mock"
const testModel = "test-model"

// newMockLLMClient 创建一个指向 httptest.Server 的真实 llm.Client。
// handler 负责根据请求中的 tools 字段返回预设 SSE 流。
func newMockLLMClient(handler http.HandlerFunc) *llm.Client {
	server := httptest.NewServer(handler)
	t := server // 用于测试中引用

	_ = t // server lifetime managed by test

	providers := map[string]llm.Provider{
		testProvider: {
			Name:    testProvider,
			ChatURL: server.URL,
			APIKey:  "test-key",
			Models: []llm.ModelInfo{
				{ID: testModel, ContextWindow: 128_000},
			},
		},
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	return llm.NewClient(providers, logger)
}

// mockToolHandler 返回一个 HTTP handler，对包含 tool_calls 的请求返回指定 tool name
// 和 JSON arguments 的 SSE 流。对不含 tools 的请求返回纯 content SSE 流。
func mockToolHandler(toolName string, argsJSON string, contentFallback string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		r.Body.Close()

		var req struct {
			Tools []map[string]any `json:"tools"`
		}
		_ = json.Unmarshal(body, &req)

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "no flusher", 500)
			return
		}

		if len(req.Tools) > 0 && toolName != "" {
			// Tool Calling 模式：发送 tool_calls delta
			// 1. Start: id + name
			startChunk := map[string]any{
				"choices": []map[string]any{{
					"delta": map[string]any{
						"tool_calls": []map[string]any{{
							"index":    0,
							"id":       "call_mock",
							"function": map[string]any{"name": toolName},
						}},
					},
				}},
			}
			fmt.Fprintf(w, "data: %s\n\n", mustJSON(startChunk))
			flusher.Flush()

			// 2. Arguments delta
			argsChunk := map[string]any{
				"choices": []map[string]any{{
					"delta": map[string]any{
						"tool_calls": []map[string]any{{
							"index":    0,
							"function": map[string]any{"arguments": argsJSON},
						}},
					},
				}},
			}
			fmt.Fprintf(w, "data: %s\n\n", mustJSON(argsChunk))
			flusher.Flush()
		} else {
			// 纯 content 流
			contentChunk := map[string]any{
				"choices": []map[string]any{{
					"delta": map[string]any{"content": contentFallback},
				}},
			}
			fmt.Fprintf(w, "data: %s\n\n", mustJSON(contentChunk))
			flusher.Flush()
		}

		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// testExtractor 创建一个用于测试的 Extractor，LLM 指向 mock server。
// Chapters 字段为非 nil 的空 Store（仅用于 nil 检查，不实际查询）。
func testExtractor(handler http.HandlerFunc) *Extractor {
	client := newMockLLMClient(handler)
	return &Extractor{
		Chapters:  &chapter.Store{},
		LLMClient: client,
	}
}

func defaultInput() ExtractPatternInput {
	return ExtractPatternInput{
		TaskID:       "test-task",
		NovelID:      1,
		ProviderName: testProvider,
		ModelID:      testModel,
	}
}

// ---------------------------------------------------------------------------
// preAnalyzeBoundaries with mock LLM
// ---------------------------------------------------------------------------

func TestPreAnalyzeBoundaries_MockLLM(t *testing.T) {
	boundaries := BoundaryHintsOutput{
		Boundaries: []BoundaryHint{
			{StartChapter: 1, EndChapter: 5, Hint: "开篇阶段"},
			{StartChapter: 6, EndChapter: 10, Hint: "转折阶段"},
		},
	}
	argsJSON, _ := json.Marshal(boundaries)

	chapters := makeChapters(10, 50)
	e := testExtractor(mockToolHandler(outputBoundariesTool, string(argsJSON), ""))

	got, err := e.preAnalyzeBoundaries(context.Background(), defaultInput(), chapters)
	if err != nil {
		t.Fatalf("preAnalyzeBoundaries: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("boundaries count = %d, want 2", len(got))
	}
	if got[0].Hint != "开篇阶段" {
		t.Errorf("first boundary hint = %q, want 开篇阶段", got[0].Hint)
	}
	if got[1].StartChapter != 6 {
		t.Errorf("second boundary start = %d, want 6", got[1].StartChapter)
	}
}

// ---------------------------------------------------------------------------
// initialChunks with mock LLM
// ---------------------------------------------------------------------------

func TestInitialChunks_MockLLM(t *testing.T) {
	output := ChunksOutput{
		Chunks: []Chunk{
			{Name: "崛起", StartChapter: 1, EndChapter: 5, Content: "主角从平凡到觉醒"},
			{Name: "转折", StartChapter: 6, EndChapter: 10, Content: "遭遇重大变故"},
		},
	}
	argsJSON, _ := json.Marshal(output)

	summaries := []ChapterSummaryItem{
		{ChapterNumber: 1, Summary: "第一章摘要"},
		{ChapterNumber: 2, Summary: "第二章摘要"},
	}

	e := testExtractor(mockToolHandler(outputChunksTool, string(argsJSON), ""))
	chunks, trace, err := e.initialChunks(context.Background(), defaultInput(), summaries, 100_000)
	if err != nil {
		t.Fatalf("initialChunks: %v", err)
	}
	if len(chunks) != 2 {
		t.Fatalf("chunks count = %d, want 2", len(chunks))
	}
	if chunks[0].Name != "崛起" {
		t.Errorf("first chunk name = %q, want 崛起", chunks[0].Name)
	}
	if trace.Round != 1 {
		t.Errorf("trace round = %d, want 1", trace.Round)
	}
	if trace.OutputCount != 2 {
		t.Errorf("trace output count = %d, want 2", trace.OutputCount)
	}
}

// ---------------------------------------------------------------------------
// compressChunks with mock LLM
// ---------------------------------------------------------------------------

func TestCompressChunks_MockLLM(t *testing.T) {
	output := ChunksOutput{
		Chunks: []Chunk{
			{Name: "全篇", StartChapter: 1, EndChapter: 10, Content: "完整叙事概括"},
		},
	}
	argsJSON, _ := json.Marshal(output)

	inputChunks := []Chunk{
		{Name: "崛起", StartChapter: 1, EndChapter: 5, Content: "主角从平凡到觉醒"},
		{Name: "转折", StartChapter: 6, EndChapter: 10, Content: "遭遇重大变故"},
	}

	e := testExtractor(mockToolHandler(outputChunksTool, string(argsJSON), ""))
	compressed, trace, err := e.compressChunks(context.Background(), defaultInput(), inputChunks, 100_000, 2)
	if err != nil {
		t.Fatalf("compressChunks: %v", err)
	}
	if len(compressed) != 1 {
		t.Fatalf("compressed count = %d, want 1", len(compressed))
	}
	if compressed[0].Name != "全篇" {
		t.Errorf("compressed name = %q, want 全篇", compressed[0].Name)
	}
	if trace.Round != 2 {
		t.Errorf("trace round = %d, want 2", trace.Round)
	}
	if trace.InputCount != 2 {
		t.Errorf("trace input count = %d, want 2", trace.InputCount)
	}
}

// ---------------------------------------------------------------------------
// finalSkill with mock LLM
// ---------------------------------------------------------------------------

func TestFinalSkill_MockLLM(t *testing.T) {
	skillContent := `---
name: 逆袭模式
description: 适合底层逆袭类故事
category: 套路模板
mode: auto
author: ai
version: 1
---
# 逆袭模式
## 套路概览
底层主角通过努力改变命运。
## 阶段拆解
第一阶段：平凡日常
第二阶段：机缘降临
第三阶段：逆袭成功`

	e := testExtractor(mockToolHandler("", "", skillContent))
	chunks := []Chunk{
		{Name: "崛起", StartChapter: 1, EndChapter: 5, Content: "主角觉醒"},
	}

	raw, err := e.finalSkill(context.Background(), defaultInput(), chunks)
	if err != nil {
		t.Fatalf("finalSkill: %v", err)
	}
	if !strings.Contains(raw, "逆袭模式") {
		t.Errorf("final skill should contain 逆袭模式, got:\n%s", raw)
	}
	if !strings.Contains(raw, "套路模板") {
		t.Errorf("final skill should contain 套路模板 category")
	}
}

// ---------------------------------------------------------------------------
// callTool error: LLM does not call the expected tool
// ---------------------------------------------------------------------------

func TestCallTool_ToolNotCalled(t *testing.T) {
	// handler 返回 content 而不是 tool call
	e := testExtractor(mockToolHandler("", "", "some content"))

	_, err := e.callTool(
		context.Background(),
		defaultInput(),
		outputBoundariesTool,
		BoundaryHintsOutput{},
		boundaryMessages(makeChapters(5, 50)),
		1,
		nil,
	)
	if err == nil {
		t.Fatal("expected error when LLM does not call tool")
	}
	if !strings.Contains(err.Error(), outputBoundariesTool) {
		t.Errorf("error should mention tool name, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// callTool error: provider not found
// ---------------------------------------------------------------------------

func TestCallTool_UnknownProvider(t *testing.T) {
	e := testExtractor(mockToolHandler("", "", ""))
	input := defaultInput()
	input.ProviderName = "nonexistent"

	_, err := e.callTool(
		context.Background(),
		input,
		outputBoundariesTool,
		BoundaryHintsOutput{},
		boundaryMessages(makeChapters(5, 50)),
		1,
		nil,
	)
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
}

// ---------------------------------------------------------------------------
// preAnalyzeBoundaries with context cancellation
// ---------------------------------------------------------------------------

func TestPreAnalyzeBoundaries_Cancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	e := testExtractor(mockToolHandler(outputBoundariesTool, `{"boundaries":[]}`, ""))
	chapters := makeChapters(5, 50)

	_, err := e.preAnalyzeBoundaries(ctx, defaultInput(), chapters)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

// ---------------------------------------------------------------------------
// Progress callback verification
// ---------------------------------------------------------------------------

func TestPreAnalyzeBoundaries_ProgressCallback(t *testing.T) {
	boundaries := BoundaryHintsOutput{
		Boundaries: []BoundaryHint{
			{StartChapter: 1, EndChapter: 5, Hint: "阶段一"},
		},
	}
	argsJSON, _ := json.Marshal(boundaries)

	var progressCalls []Progress
	e := testExtractor(mockToolHandler(outputBoundariesTool, string(argsJSON), ""))
	e.Progress = func(p Progress) {
		progressCalls = append(progressCalls, p)
	}

	chapters := makeChapters(5, 50)
	_, err := e.preAnalyzeBoundaries(context.Background(), defaultInput(), chapters)
	if err != nil {
		t.Fatalf("preAnalyzeBoundaries: %v", err)
	}

	// mock server 不发送 reasoning_content，因此不会触发 LLMThinking。
	// 但 Tool Calling 的 EventToolCallStart 会触发 LLMGenerating。
	foundGenerating := false
	for _, p := range progressCalls {
		if p.LLMStatus == LLMGenerating {
			foundGenerating = true
		}
	}
	if !foundGenerating {
		t.Error("progress callback should receive LLMGenerating status")
	}
}
