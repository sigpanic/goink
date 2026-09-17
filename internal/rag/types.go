package rag

import "context"

// Embedder 将文本转换为向量。
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
	Dim() int
	Close() error
}

// Chunk 是待索引的文本块，携带来源元数据。
type Chunk struct {
	ID            string
	Content       string
	ChapterNumber int    // 章节号，用于 chunk_id 字符串（如 "3_summary"）与日志可读性
	ChapterID     int64  // v1.6.0 新增：chapters.id 外键，写入 vec 表 chapter_id 列；0=未知章节
	ChunkType     string // "summary" / "chapter_brief" / "content"
	ChunkIndex    int
	StartRunePos  int // chunk 在原始正文中的 rune 偏移
	Metadata      map[string]any
}

// SearchResult 是单条检索结果。
type SearchResult struct {
	ChunkID       string
	Content       string
	SourceType    string
	ChapterNumber int   // 已废弃：vector_store.Search 恒不填充（恒 0）。v1.6.0 后用 ChapterID，需章节号时由调用方按 id 反查
	ChapterID     int64 // v1.6.0 新增：从 vec 表 chapter_id 列读出
	StartRunePos  int   // chunk 在原始正文中的 rune 偏移
	Distance      float64
	Relevance     float64
	Embedding     []float32 // 512维向量，用于MMR多样性计算
}

// SearchFilter 限定检索范围。
// v1.6.0 后 vec 表已无 chapter_number 列，过滤请用 ChapterIDs；
// ChapterNumbers 仅存兼容占位，调用方应迁移到 ChapterIDs（commit 4.3）。
type SearchFilter struct {
	ChapterNumbers []int
	ChapterIDs     []int64 // v1.6.0 新增：按 chapters.id 过滤
	ChunkTypes     []string
}

// ChapterChunkParams 是 BuildChapterChunks 的输入参数。
type ChapterChunkParams struct {
	ChapterNumber int
	ChapterID     int64 // v1.6.0 新增：chapters.id，填入 Chunk.ChapterID
	ChapterTitle  string
	Content       string
	Summary       string
}
