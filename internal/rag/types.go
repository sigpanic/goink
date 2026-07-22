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
	ChapterNumber int
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
	ChapterNumber int
	StartRunePos  int // chunk 在原始正文中的 rune 偏移
	Distance      float64
	Relevance     float64
	Embedding     []float32 // 512维向量，用于MMR多样性计算
}

// SearchFilter 限定检索范围。
type SearchFilter struct {
	ChapterNumbers []int
	ChunkTypes     []string
}

// ChapterChunkParams 是 BuildChapterChunks 的输入参数。
type ChapterChunkParams struct {
	ChapterNumber int
	ChapterTitle  string
	Content       string
	Summary       string
}
