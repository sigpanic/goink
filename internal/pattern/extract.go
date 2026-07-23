package pattern

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/chapter"
	"github.com/sigpanic/goink/internal/config"
	"github.com/sigpanic/goink/internal/git"
	"github.com/sigpanic/goink/internal/llm"
	"github.com/sigpanic/goink/internal/mcp_tools"
	"github.com/sigpanic/goink/internal/skill"
)

const (
	outputBoundariesTool       = "output_boundary_hints"
	outputChapterSummariesTool = "output_chapter_summaries"
	outputChunksTool           = "output_chunks"
	finalChunkBudget           = 80_000
	defaultContextWindow       = 128_000
)

type Extractor struct {
	Chapters  *chapter.Store
	LLMClient *llm.Client
	DB        *gorm.DB
	Progress  func(Progress)
}

// Extract 执行完整的模式提取 Pipeline：Pre-step 边界分析 → Step 1 章节摘要 → Step 2 递归压缩 → Step 3 生成技能
func (e *Extractor) Extract(ctx context.Context, input ExtractPatternInput) (*ExtractPatternResult, error) {
	if e.Chapters == nil {
		return nil, fmt.Errorf("章节存储未初始化")
	}
	if e.LLMClient == nil {
		return nil, fmt.Errorf("LLM 客户端未初始化")
	}
	if input.NovelID <= 0 {
		return nil, fmt.Errorf("novel_id 不能为空")
	}
	if input.ProviderName == "" || input.ModelID == "" {
		return nil, fmt.Errorf("provider_name 和 model_id 不能为空")
	}
	if input.TaskID == "" {
		input.TaskID = fmt.Sprintf("pattern-%d", time.Now().UnixNano())
	}

	chapters, err := e.loadChapters(ctx, input.NovelID, input.ChapterIDs)
	if err != nil {
		return nil, err
	}
	if len(chapters) == 0 {
		return nil, fmt.Errorf("请先导入或创建章节，然后再提取模式")
	}
	if len(chapters) < 5 {
		return nil, fmt.Errorf("章节数量不足，模式提取至少需要 5 个有内容的章节")
	}

	contextWindow := e.modelContextWindow(input.ProviderName, input.ModelID)
	budget := batchBudget(contextWindow)
	trace := &Trace{
		TaskID:        input.TaskID,
		NovelID:       input.NovelID,
		ChapterCount:  len(chapters),
		ContextWindow: contextWindow,
		BatchBudget:   budget,
	}
	e.emit(input, Progress{
		Stage:   StageLoaded,
		Message: fmt.Sprintf("已加载 %d 章，开始分析结构边界", len(chapters)),
	})

	// Pre-step：章节标题分析，获取疑似阶段边界
	e.emit(input, Progress{
		Stage:   StageBoundaries,
		Message: "正在分析结构边界...",
	})
	boundaries, err := e.preAnalyzeBoundaries(ctx, input, chapters)
	if err != nil {
		return nil, err
	}
	trace.Boundaries = boundaries
	e.emit(input, Progress{
		Stage:      StageBoundaries,
		Message:    fmt.Sprintf("已找到 %d 条疑似叙事边界", len(boundaries)),
		Boundaries: boundaries,
	})
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Step 1：增量生成章节摘要（已有摘要的跳过）
	summaries, err := e.ensureSummaries(ctx, input, chapters, boundaries, budget)
	if err != nil {
		return nil, err
	}
	trace.Summaries = summaries
	e.emit(input, Progress{
		Stage:   StageSummaries,
		Message: fmt.Sprintf("已准备 %d 条章节摘要", len(summaries)),
	})
	if len(summaries) == 0 {
		return nil, fmt.Errorf("没有可用的章节摘要，无法提取模式")
	}

	// Step 2 第 1 轮：摘要 → 初始叙事阶段块
	e.emit(input, Progress{
		Stage:   StageInitialChunks,
		Message: "正在生成初始叙事阶段块...",
		Round:   1,
	})
	chunks, roundTrace, err := e.initialChunks(ctx, input, summaries, budget)
	if err != nil {
		return nil, err
	}
	trace.ChunkRounds = append(trace.ChunkRounds, roundTrace)
	e.emit(input, Progress{
		Stage:   StageInitialChunks,
		Message: fmt.Sprintf("第 1 轮生成 %d 个叙事阶段块", len(chunks)),
		Round:   1,
		Tokens:  tokensOfChunks(chunks),
		Chunks:  chunks,
	})
	if len(chunks) == 0 {
		return nil, fmt.Errorf("LLM 未返回任何叙事阶段块")
	}

	// Step 2 后续轮：递归压缩，直到总 token ≤ 阈值
	stallCount := 0
	const minReductionRatio = 0.05 // 每轮至少减少5%才视为有效收敛
	const maxStallRounds = 2       // 连续2轮收敛不足则终止
	for round := 2; tokensOfChunks(chunks) > finalChunkBudget; round++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		e.emit(input, Progress{
			Stage:   StageCompressChunks,
			Message: fmt.Sprintf("第 %d 轮压缩中...", round),
			Round:   round,
			Tokens:  tokensOfChunks(chunks),
		})
		prevTokens := tokensOfChunks(chunks)
		next, roundTrace, err := e.compressChunks(ctx, input, chunks, budget, round)
		if err != nil {
			return nil, err
		}
		nextTokens := tokensOfChunks(next)
		if len(next) == 0 {
			// 退避 30s 后重试一次，LLM 偶尔可能返回空结果
			select {
			case <-time.After(30 * time.Second):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			next, roundTrace, err = e.compressChunks(ctx, input, chunks, budget, round)
			if err != nil {
				return nil, fmt.Errorf("第%d轮压缩未返回任何叙事阶段块，重试也失败: %w", round, err)
			}
			if len(next) == 0 {
				return nil, fmt.Errorf("第%d轮压缩重试后仍未返回任何叙事阶段块，无法继续生成最终技能", round)
			}
			nextTokens = tokensOfChunks(next)
			e.emit(input, Progress{
				Stage:   StageCompressChunks,
				Message: fmt.Sprintf("第 %d 轮重试后得到 %d 个叙事阶段块", round, len(next)),
				Round:   round,
				Tokens:  nextTokens,
				Chunks:  next,
			})
		} else {
			e.emit(input, Progress{
				Stage:   StageCompressChunks,
				Message: fmt.Sprintf("第 %d 轮压缩为 %d 个叙事阶段块", round, len(next)),
				Round:   round,
				Tokens:  nextTokens,
				Chunks:  next,
			})
		}
		trace.ChunkRounds = append(trace.ChunkRounds, roundTrace)
		if nextTokens >= prevTokens {
			// 压缩未降低 token 量：若当前 chunks 仍在模型上下文窗口 80% 以内，容忍并退出循环
			if prevTokens < int(float64(contextWindow)*0.8) {
				break
			}
			return nil, fmt.Errorf("第%d轮压缩未降低 token 量（%d -> %d），最终输入仍超过预算 %d", round, prevTokens, nextTokens, finalChunkBudget)
		}
		reduction := float64(prevTokens-nextTokens) / float64(prevTokens)
		if reduction < minReductionRatio {
			stallCount++
			if stallCount >= maxStallRounds {
				if tokensOfChunks(next) < int(float64(contextWindow)*0.8) {
					chunks = next
					break
				}
				return nil, fmt.Errorf("连续%d轮压缩收敛不足（降幅<%.0f%%），最终输入仍超过预算 %d", maxStallRounds, minReductionRatio*100, finalChunkBudget)
			}
		} else {
			stallCount = 0
		}
		chunks = next
	}

	// Step 3：所有阶段块 → 最终套路技能
	trace.FinalTokens = tokensOfChunks(chunks)
	e.emit(input, Progress{
		Stage:   StageFinalizing,
		Message: "正在生成最终套路技能...",
		Tokens:  trace.FinalTokens,
		Chunks:  chunks,
	})
	raw, err := e.finalSkill(ctx, input, chunks)
	if err != nil {
		return nil, err
	}
	sk, err := skill.ParseBytes([]byte(raw), "ai")
	if err != nil {
		return nil, fmt.Errorf("解析生成的技能失败: %w", err)
	}

	safeName := skill.SanitizeFileName(sk.Name)
	e.emit(input, Progress{
		Stage:   StageDone,
		Message: fmt.Sprintf("已生成「%s」", sk.Name),
		Tokens:  trace.FinalTokens,
	})
	return &ExtractPatternResult{
		TaskID:      input.TaskID,
		Name:        sk.Name,
		Description: sk.Description,
		RawContent:  sk.RawContent,
		FilePath:    fmt.Sprintf("~/.goink/skills/%s.md", safeName),
		Trace:       trace,
	}, nil
}

// loadChapters 加载指定 Novel 的章节，支持按 ID 过滤
func (e *Extractor) loadChapters(ctx context.Context, novelID int64, ids []int64) ([]ChapterSource, error) {
	dbChapters, err := e.Chapters.ListAllByNovel(ctx, novelID)
	if err != nil {
		return nil, err
	}
	idSet := map[int64]bool{}
	if len(ids) > 0 {
		for _, id := range ids {
			idSet[id] = true
		}
	}
	out := make([]ChapterSource, 0, len(dbChapters))
	for _, ch := range dbChapters {
		if len(idSet) > 0 && !idSet[ch.ID] {
			continue
		}
		content, err := git.ReadFile(novelID, git.ChapterPath(ch.ChapterNumber))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("读取第%d章失败: %w", ch.ChapterNumber, err)
		}
		out = append(out, ChapterSource{
			ID:            ch.ID,
			ChapterNumber: ch.ChapterNumber,
			Title:         ch.Title,
			Summary:       strings.TrimSpace(ch.Summary),
			Content:       strings.TrimSpace(content),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ChapterNumber < out[j].ChapterNumber })
	return out, nil
}

// preAnalyzeBoundaries Pre-step：通过章节标题推断叙事阶段边界
func (e *Extractor) preAnalyzeBoundaries(ctx context.Context, input ExtractPatternInput, chapters []ChapterSource) ([]BoundaryHint, error) {
	onStatus := func(s LLMStatus) {
		e.emit(input, Progress{Stage: StageBoundaries, LLMStatus: s})
	}
	raw, err := e.callTool(ctx, input, outputBoundariesTool, BoundaryHintsOutput{}, boundaryMessages(chapters), 1, 0, onStatus)
	if err != nil {
		return nil, fmt.Errorf("分析章节标题边界失败: %w", err)
	}
	var out BoundaryHintsOutput
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("解析边界提示失败: %w", err)
	}
	return normalizeBoundaries(out.Boundaries), nil
}

// ensureSummaries Step 1：增量生成章节摘要，已有摘要的章节作为上下文参考跳过
func (e *Extractor) ensureSummaries(ctx context.Context, input ExtractPatternInput, chapters []ChapterSource, boundaries []BoundaryHint, budget int) ([]ChapterSummaryItem, error) {
	batches := buildChapterBatches(chapters, boundaries, budget)
	summaryByNumber := map[int]string{}
	for _, ch := range chapters {
		if strings.TrimSpace(ch.Summary) != "" {
			summaryByNumber[ch.ChapterNumber] = strings.TrimSpace(ch.Summary)
		}
	}

	for i, batch := range batches {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var missing []ChapterSource
		for _, ch := range batch {
			if _, ok := summaryByNumber[ch.ChapterNumber]; !ok {
				missing = append(missing, ch)
			}
		}
		if len(missing) == 0 {
			e.emit(input, Progress{
				Stage:      StageSummaries,
				Message:    fmt.Sprintf("摘要第 %d/%d 批已复用缓存", i+1, len(batches)),
				BatchIndex: i + 1,
				BatchTotal: len(batches),
			})
			continue
		}

		e.emit(input, Progress{
			Stage:      StageSummaries,
			Message:    fmt.Sprintf("摘要第 %d/%d 批...", i+1, len(batches)),
			BatchIndex: i + 1,
			BatchTotal: len(batches),
		})
		raw, err := e.callTool(ctx, input, outputChapterSummariesTool, ChapterSummariesOutput{}, summaryMessages(batch, boundaries), 2, 0, nil)
		if err != nil {
			return nil, fmt.Errorf("生成章节摘要失败: %w", err)
		}
		var out ChapterSummariesOutput
		if err := json.Unmarshal(raw, &out); err != nil {
			return nil, fmt.Errorf("解析章节摘要失败: %w", err)
		}
		for _, item := range out.Summaries {
			item.Summary = strings.TrimSpace(item.Summary)
			if item.ChapterNumber == 0 || item.Summary == "" {
				continue
			}
			if !chapterInList(item.ChapterNumber, missing) {
				continue
			}
			summaryByNumber[item.ChapterNumber] = item.Summary
			if err := e.Chapters.DB.WithContext(ctx).
				Model(&chapter.Chapter{}).
				Where("novel_id = ? AND chapter_number = ?", input.NovelID, item.ChapterNumber).
				Update("summary", item.Summary).Error; err != nil {
				return nil, fmt.Errorf("保存第%d章摘要失败: %w", item.ChapterNumber, err)
			}
		}
		e.emit(input, Progress{
			Stage:      StageSummaries,
			Message:    fmt.Sprintf("摘要第 %d/%d 批完成，新增 %d 条", i+1, len(batches), len(out.Summaries)),
			BatchIndex: i + 1,
			BatchTotal: len(batches),
			Summaries:  out.Summaries,
		})
	}

	result := make([]ChapterSummaryItem, 0, len(chapters))
	for _, ch := range chapters {
		summary := summaryByNumber[ch.ChapterNumber]
		if summary == "" && ch.Content == "" {
			summary = "无内容。"
		}
		if summary == "" {
			continue
		}
		result = append(result, ChapterSummaryItem{ChapterNumber: ch.ChapterNumber, Summary: summary})
	}
	return result, nil
}

// initialChunks Step 2 第 1 轮：将章节摘要压缩为初始叙事阶段块
func (e *Extractor) initialChunks(ctx context.Context, input ExtractPatternInput, summaries []ChapterSummaryItem, budget int) ([]Chunk, ChunkRoundTrace, error) {
	var all []Chunk
	batches := buildSummaryBatches(summaries, budget)
	trace := ChunkRoundTrace{Round: 1, InputCount: len(summaries)}
	for i, batch := range batches {
		if err := ctx.Err(); err != nil {
			return nil, trace, err
		}
		e.emit(input, Progress{
			Stage:      StageInitialChunks,
			Message:    fmt.Sprintf("第 1 轮第 %d/%d 批...", i+1, len(batches)),
			Round:      1,
			BatchIndex: i + 1,
			BatchTotal: len(batches),
		})
		raw, err := e.callTool(ctx, input, outputChunksTool, ChunksOutput{}, initialChunkMessages(batch), 2, 0, nil)
		if err != nil {
			return nil, trace, fmt.Errorf("生成初始阶段块失败: %w", err)
		}
		var out ChunksOutput
		if err := json.Unmarshal(raw, &out); err != nil {
			return nil, trace, fmt.Errorf("解析初始阶段块失败: %w", err)
		}
		normalized := normalizeChunks(out.Chunks)
		all = append(all, normalized...)
		trace.Batches = append(trace.Batches, BatchTrace{
			Index:        i + 1,
			InputCount:   len(batch),
			OutputCount:  len(normalized),
			ApproxTokens: tokensOfSummaries(batch),
		})
		e.emit(input, Progress{
			Stage:      StageInitialChunks,
			Message:    fmt.Sprintf("第 1 轮第 %d/%d 批生成 %d 个阶段块", i+1, len(batches), len(normalized)),
			Round:      1,
			BatchIndex: i + 1,
			BatchTotal: len(batches),
			Chunks:     normalized,
		})
	}
	trace.Chunks = all
	trace.OutputCount = len(all)
	trace.TokenCount = tokensOfChunks(all)
	return all, trace, nil
}

// compressChunks Step 2 后续轮：递归合并叙事阶段块
func (e *Extractor) compressChunks(ctx context.Context, input ExtractPatternInput, chunks []Chunk, budget int, round int) ([]Chunk, ChunkRoundTrace, error) {
	var all []Chunk
	batches := buildChunkBatches(chunks, budget)
	trace := ChunkRoundTrace{Round: round, InputCount: len(chunks)}
	for i, batch := range batches {
		if err := ctx.Err(); err != nil {
			return nil, trace, err
		}
		e.emit(input, Progress{
			Stage:      StageCompressChunks,
			Message:    fmt.Sprintf("第 %d 轮第 %d/%d 批...", round, i+1, len(batches)),
			Round:      round,
			BatchIndex: i + 1,
			BatchTotal: len(batches),
		})
		raw, err := e.callTool(ctx, input, outputChunksTool, ChunksOutput{}, compressChunkMessages(batch, round), 2, 0, nil)
		if err != nil {
			return nil, trace, fmt.Errorf("第%d轮压缩失败: %w", round, err)
		}
		var out ChunksOutput
		if err := json.Unmarshal(raw, &out); err != nil {
			return nil, trace, fmt.Errorf("解析第%d轮压缩结果失败: %w", round, err)
		}
		normalized := normalizeChunks(out.Chunks)
		all = append(all, normalized...)
		trace.Batches = append(trace.Batches, BatchTrace{
			Index:        i + 1,
			InputCount:   len(batch),
			OutputCount:  len(normalized),
			ApproxTokens: tokensOfChunks(batch),
		})
		e.emit(input, Progress{
			Stage:      StageCompressChunks,
			Message:    fmt.Sprintf("第 %d 轮第 %d/%d 批压缩为 %d 个阶段块", round, i+1, len(batches), len(normalized)),
			Round:      round,
			BatchIndex: i + 1,
			BatchTotal: len(batches),
			Chunks:     normalized,
		})
	}
	trace.Chunks = all
	trace.OutputCount = len(all)
	trace.TokenCount = tokensOfChunks(all)
	return all, trace, nil
}

// finalSkill Step 3：所有阶段块 → 通过 output_skill 工具调用生成最终套路技能
func (e *Extractor) finalSkill(ctx context.Context, input ExtractPatternInput, chunks []Chunk) (string, error) {
	onStatus := func(s LLMStatus) {
		e.emit(input, Progress{Stage: StageFinalizing, LLMStatus: s})
	}
	raw, err := e.callTool(ctx, input, "output_skill", SkillOutput{}, finalSkillMessages(chunks), 2, 32000, onStatus)
	if err != nil {
		return "", err
	}
	var out SkillOutput
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("解析生成的技能失败: %w", err)
	}
	if strings.TrimSpace(out.Name) == "" || strings.TrimSpace(out.Content) == "" {
		return "", fmt.Errorf("LLM 返回的技能内容不完整")
	}
	return buildSkillMarkdown(out.Name, out.Description, out.Content), nil
}

// buildSkillMarkdown 组装完整的 skill markdown：固定 frontmatter + 正文。
func buildSkillMarkdown(name, description, content string) string {
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "name: %s\n", name)
	fmt.Fprintf(&b, "description: %s\n", description)
	b.WriteString("category: 套路模板\n")
	b.WriteString("mode: auto\n")
	b.WriteString("author: ai\n")
	b.WriteString("version: 1\n")
	b.WriteString("---\n\n")
	b.WriteString(strings.TrimSpace(content))
	b.WriteString("\n")
	return b.String()
}

// callTool 请求 LLM 调用指定工具，返回结构化 JSON。
// 部分供应商的 thinking mode 不兼容 tool_choice，因此只提供 tools，由提示词约束模型调用目标工具。
// onStatus 回调在 LLM 流事件时被调用，用于推送 thinking/generating 状态。
func (e *Extractor) callTool(ctx context.Context, input ExtractPatternInput, toolName string, schema any, messages []map[string]any, attempts int, maxTokens int, onStatus func(LLMStatus)) (json.RawMessage, error) {
	tools := []map[string]any{{
		"type": "function",
		"function": map[string]any{
			"name":        toolName,
			"description": "返回模式提取的结构化输出。",
			"parameters":  mcp_tools.SchemaOf(schema),
		},
	}}
	opts := callOptions(input)
	if maxTokens > 0 {
		opts.MaxTokens = &maxTokens
	}
	var allErrs []error
	for i := range attempts {
		if i > 0 {
			select {
			case <-time.After(30 * time.Second):
				// continue retry
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		var lastErr error
		sentThinking := false
		events := e.LLMClient.ChatStream(ctx, input.ProviderName, messages, tools, input.ModelID, opts)
		for evt := range events {
			switch evt.Type {
			case llm.EventError:
				lastErr = evt.Error
			case llm.EventThinking:
				if onStatus != nil && !sentThinking {
					sentThinking = true
					onStatus(LLMThinking)
				}
			case llm.EventToolCallStart:
				if onStatus != nil {
					onStatus(LLMGenerating)
				}
			case llm.EventToolCallEnd:
				if evt.Delta != nil && evt.Delta.ToolName == toolName && len(evt.Delta.ArgumentsJSON) > 0 {
					return evt.Delta.ArgumentsJSON, nil
				}
			}
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if lastErr == nil {
			lastErr = fmt.Errorf("LLM 未调用工具 %s", toolName)
		}
		allErrs = append(allErrs, lastErr)
	}
	return nil, errors.Join(allErrs...)
}

func callOptions(input ExtractPatternInput) *llm.CallOptions {
	opts := &llm.CallOptions{}
	if input.ReasoningEffort != "" {
		opts.ReasoningEffort = &input.ReasoningEffort
	}
	return opts
}

// batchBudget 根据模型上下文窗口计算每批 token 预算：≥200K 用 100K，否则取 70%
func batchBudget(contextWindow int) int {
	if contextWindow >= 200_000 {
		return 100_000
	}
	return int(float64(contextWindow) * 0.7)
}

func (e *Extractor) modelContextWindow(providerName, modelID string) int {
	if e.LLMClient != nil {
		if m, ok := e.LLMClient.ProviderModel(providerName, modelID); ok && m.ContextWindow > 0 {
			return m.ContextWindow
		}
	}
	return defaultContextWindow
}

// buildChapterBatches 按 token 预算分批章节正文，切分时尽量对齐 Pre-step 的疑似边界
func buildChapterBatches(chapters []ChapterSource, boundaries []BoundaryHint, budget int) [][]ChapterSource {
	var batches [][]ChapterSource
	var batch []ChapterSource
	tokens := 0
	for i := 0; i < len(chapters); i++ {
		ch := chapters[i]
		t := approxTokens(ch.Content)
		if tokens+t > budget && len(batch) > 0 {
			cut := maybeExtendToBoundary(chapters, i, boundaries, budget-tokens)
			if cut > i {
				batch = append(batch, chapters[i:cut]...)
				i = cut - 1
				batches = append(batches, batch)
				batch = nil
				tokens = 0
				continue
			}
			batches = append(batches, batch)
			batch = nil
			tokens = 0
		}
		batch = append(batch, ch)
		tokens += t
	}
	if len(batch) > 0 {
		batches = append(batches, batch)
	}
	return batches
}

// maybeExtendToBoundary 向前最多看 10 章，尝试在疑似边界处切分
func maybeExtendToBoundary(chapters []ChapterSource, start int, boundaries []BoundaryHint, remaining int) int {
	limit := min(start+10, len(chapters))
	extraTokens := 0
	for i := start; i < limit; i++ {
		extraTokens += approxTokens(chapters[i].Content)
		if extraTokens > remaining {
			return start
		}
		for _, b := range boundaries {
			if b.EndChapter == chapters[i].ChapterNumber {
				return i + 1
			}
		}
	}
	return start
}

func buildSummaryBatches(items []ChapterSummaryItem, budget int) [][]ChapterSummaryItem {
	var batches [][]ChapterSummaryItem
	var batch []ChapterSummaryItem
	tokens := 0
	for _, item := range items {
		t := approxTokens(item.Summary) + 16
		if tokens+t > budget && len(batch) > 0 {
			batches = append(batches, batch)
			batch = nil
			tokens = 0
		}
		batch = append(batch, item)
		tokens += t
	}
	if len(batch) > 0 {
		batches = append(batches, batch)
	}
	return batches
}

func buildChunkBatches(chunks []Chunk, budget int) [][]Chunk {
	var batches [][]Chunk
	var batch []Chunk
	tokens := 0
	for _, ch := range chunks {
		t := approxTokens(ch.Name+ch.Content) + 32
		if tokens+t > budget && len(batch) > 0 {
			batches = append(batches, batch)
			batch = nil
			tokens = 0
		}
		batch = append(batch, ch)
		tokens += t
	}
	if len(batch) > 0 {
		batches = append(batches, batch)
	}
	return batches
}

func tokensOfChunks(chunks []Chunk) int {
	total := 0
	for _, ch := range chunks {
		total += approxTokens(ch.Name+ch.Content) + 32
	}
	return total
}

func tokensOfSummaries(items []ChapterSummaryItem) int {
	total := 0
	for _, item := range items {
		total += approxTokens(item.Summary) + 16
	}
	return total
}

func (e *Extractor) emit(input ExtractPatternInput, p Progress) {
	if e.Progress == nil {
		return
	}
	p.TaskID = input.TaskID
	p.NovelID = input.NovelID
	e.Progress(p)
}

// approxTokens 估算文本 token 数：优先用 tiktoken 精确计算，失败时退化为字符数/2
func approxTokens(text string) int {
	n, err := llm.CountTokens(text)
	if err == nil {
		return n
	}
	runes := len([]rune(text))
	if runes == 0 {
		return 0
	}
	return runes / 2
}

// normalizeBoundaries 清洗并排序边界提示：交换逆序区间、过滤无效条目、按起始章排序
func normalizeBoundaries(items []BoundaryHint) []BoundaryHint {
	out := make([]BoundaryHint, 0, len(items))
	for _, it := range items {
		if it.StartChapter <= 0 || it.EndChapter <= 0 {
			continue
		}
		if it.StartChapter > it.EndChapter {
			it.StartChapter, it.EndChapter = it.EndChapter, it.StartChapter
		}
		it.Hint = strings.TrimSpace(it.Hint)
		out = append(out, it)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartChapter < out[j].StartChapter })
	return out
}

// normalizeChunks 清洗并排序叙事阶段块：过滤空字段、交换逆序区间、按起止章排序
func normalizeChunks(items []Chunk) []Chunk {
	out := make([]Chunk, 0, len(items))
	for _, it := range items {
		it.Name = strings.TrimSpace(it.Name)
		it.Content = strings.TrimSpace(it.Content)
		if it.Name == "" || it.Content == "" || it.StartChapter <= 0 || it.EndChapter <= 0 {
			continue
		}
		if it.StartChapter > it.EndChapter {
			it.StartChapter, it.EndChapter = it.EndChapter, it.StartChapter
		}
		out = append(out, it)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].StartChapter == out[j].StartChapter {
			return out[i].EndChapter < out[j].EndChapter
		}
		return out[i].StartChapter < out[j].StartChapter
	})
	return out
}

func chapterInList(num int, chapters []ChapterSource) bool {
	for _, ch := range chapters {
		if ch.ChapterNumber == num {
			return true
		}
	}
	return false
}

func NovelSkillPath(novelID int64, name string) string {
	return filepath.Join(config.NovelSkillsDir(novelID), skill.SanitizeFileName(name)+".md")
}
