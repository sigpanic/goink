package pattern

import "github.com/sigpanic/goink/internal/llm"

type ExtractPatternInput struct {
	TaskID          string  `json:"task_id,omitempty"`
	NovelID         int64   `json:"novel_id"`
	ProviderName    string  `json:"provider_name"`
	ModelID         string  `json:"model_id"`
	ReasoningEffort string  `json:"reasoning_effort"`
	ChapterIDs      []int64 `json:"chapter_ids,omitempty"`
}

type ExtractPatternResult struct {
	TaskID      string `json:"task_id,omitempty"`
	Name        string `json:"name"`
	Description string `json:"description"`
	RawContent  string `json:"raw_content"`
	FilePath    string `json:"file_path"`
	Trace       *Trace `json:"trace,omitempty"`
}

// Pipeline stage names
const (
	StageLoaded         = "loaded"
	StageBoundaries     = "boundaries"
	StageSummaries      = "summaries"
	StageInitialChunks  = "initial_chunks"
	StageCompressChunks = "compress_chunks"
	StageFinalizing     = "finalizing"
	StageDone           = "done"
)

// LLMStatus 是 llm.CallToolStatus 的别名，保持 pattern 包对外类型兼容。
type LLMStatus = llm.CallToolStatus

const (
	LLMThinking   = llm.CallToolThinking   // 模型正在推理/思考
	LLMGenerating = llm.CallToolGenerating // 模型正在输出结果
)

type Progress struct {
	TaskID     string               `json:"task_id,omitempty"`
	NovelID    int64                `json:"novel_id"`
	Stage      string               `json:"stage"`
	Message    string               `json:"message"`
	LLMStatus  LLMStatus            `json:"llm_status,omitempty"`
	Round      int                  `json:"round,omitempty"`
	BatchIndex int                  `json:"batch_index,omitempty"`
	BatchTotal int                  `json:"batch_total,omitempty"`
	Tokens     int                  `json:"tokens,omitempty"`
	Boundaries []BoundaryHint       `json:"boundaries,omitempty"`
	Summaries  []ChapterSummaryItem `json:"summaries,omitempty"`
	Chunks     []Chunk              `json:"chunks,omitempty"`
}

type Trace struct {
	TaskID        string               `json:"task_id,omitempty"`
	NovelID       int64                `json:"novel_id"`
	ChapterCount  int                  `json:"chapter_count"`
	ContextWindow int                  `json:"context_window"`
	BatchBudget   int                  `json:"batch_budget"`
	Boundaries    []BoundaryHint       `json:"boundaries"`
	Summaries     []ChapterSummaryItem `json:"summaries"`
	ChunkRounds   []ChunkRoundTrace    `json:"chunk_rounds"`
	FinalTokens   int                  `json:"final_tokens"`
}

type ChunkRoundTrace struct {
	Round       int          `json:"round"`
	InputCount  int          `json:"input_count"`
	OutputCount int          `json:"output_count"`
	TokenCount  int          `json:"token_count"`
	Batches     []BatchTrace `json:"batches"`
	Chunks      []Chunk      `json:"chunks"`
}

type BatchTrace struct {
	Index        int `json:"index"`
	InputCount   int `json:"input_count"`
	OutputCount  int `json:"output_count"`
	ApproxTokens int `json:"approx_tokens"`
}

type BoundaryHint struct {
	StartChapter int    `json:"start_chapter" jsonschema:"required,description=起始章节号"`
	EndChapter   int    `json:"end_chapter" jsonschema:"required,description=结束章节号"`
	Hint         string `json:"hint" jsonschema:"required,description=为什么这里可能是叙事阶段边界"`
}

type BoundaryHintsOutput struct {
	Boundaries []BoundaryHint `json:"boundaries" jsonschema:"required,description=可能的叙事阶段边界列表"`
}

type ChapterSummaryItem struct {
	ChapterNumber int    `json:"chapter_number" jsonschema:"required,description=章节号"`
	Summary       string `json:"summary" jsonschema:"required,description=80-150字的章节叙事摘要，覆盖核心事件、人物行为与转折"`
}

type ChapterSummariesOutput struct {
	Summaries []ChapterSummaryItem `json:"summaries" jsonschema:"required,description=生成的章节摘要列表"`
}

type Chunk struct {
	Name         string `json:"name" jsonschema:"required,description=叙事阶段名称（如：崛起、转折）"`
	StartChapter int    `json:"start_chapter" jsonschema:"required,description=起始章节号"`
	EndChapter   int    `json:"end_chapter" jsonschema:"required,description=结束章节号"`
	Content      string `json:"content" jsonschema:"required,description=100-200字的阶段叙事概括，覆盖核心事件与转折"`
}

type ChunksOutput struct {
	Chunks []Chunk `json:"chunks" jsonschema:"required,description=压缩后的叙事阶段块列表"`
}

type SkillOutput struct {
	Name        string `json:"name" jsonschema:"required,description=技能名称（中文模式名）"`
	Description string `json:"description" jsonschema:"required,description=一句话描述何时使用此叙事模式"`
	Content     string `json:"content" jsonschema:"required,description=技能正文 markdown，含 ## 套路概览/阶段拆解/爽点节奏/角色功能模板/可复用叙事规律/使用注意 等章节，不含 frontmatter"`
}

// ChapterSource 是从 DB 元数据和 git 正文拼出的章节数据传递对象
type ChapterSource struct {
	ID            int64
	ChapterNumber int
	Title         string
	Summary       string
	Content       string
}
