//go:build cgo && e2e

package e2e

import (
	"context"
	"testing"

	"github.com/sigpanic/goink/internal/rag"
)

// getVectorStore returns the shared VectorStore singleton initialized in TestMain.
func getVectorStore(t *testing.T) *rag.VectorStore {
	t.Helper()
	vs := rag.GetVectorStore()
	if vs == nil {
		t.Fatal("GetVectorStore() returned nil — was TestMain initialization successful?")
	}
	return vs
}

func TestVectorStore_CreateTable(t *testing.T) {
	vs := getVectorStore(t)

	ctx := context.Background()
	novelID := int64(10001)

	// Ensure table creation works
	if err := vs.DeleteNovel(ctx, novelID); err != nil {
		t.Logf("DeleteNovel (cleanup): %v", err)
	}

	// IndexChunks should create the table automatically
	chunks := []rag.Chunk{
		{
			ID:            "10001_summary",
			Content:       "这是一部关于少年修仙的小说",
			ChapterNumber: 1,
			ChunkType:     "summary",
			ChunkIndex:    0,
		},
	}
	if err := vs.IndexChunks(ctx, novelID, chunks); err != nil {
		t.Fatalf("IndexChunks() failed: %v", err)
	}

	t.Log("VectorStore table creation and initial index OK")
}

func TestVectorStore_Search(t *testing.T) {
	vs := getVectorStore(t)

	ctx := context.Background()
	novelID := int64(10002)
	vs.DeleteNovel(ctx, novelID)

	// Index some chunks
	chunks := []rag.Chunk{
		{
			ID:            "10002_summary",
			Content:       "主角林风在修炼中突破瓶颈，踏入金丹期",
			ChapterNumber: 1,
			ChunkType:     "summary",
			ChunkIndex:    0,
		},
		{
			ID:            "10002_brief",
			Content:       "第一章 修行之路 林风独自在山洞中修炼，经历了无数次的失败",
			ChapterNumber: 1,
			ChunkType:     "chapter_brief",
			ChunkIndex:    0,
		},
		{
			ID:            "10002_0",
			Content:       "林风盘坐在冰冷的石台上，感受着体内灵力的涌动。经过三年的苦修，他终于触摸到了金丹期的门槛。一道金光从他体内迸发而出，照亮了整个山洞。",
			ChapterNumber: 1,
			ChunkType:     "content",
			ChunkIndex:    0,
		},
		{
			ID:            "10002_1",
			Content:       "山洞外，一只白色的灵狐静静地等待着。它感受到了主人的气息变化，尾巴轻轻摇动。",
			ChapterNumber: 1,
			ChunkType:     "content",
			ChunkIndex:    1,
		},
	}

	if err := vs.IndexChunks(ctx, novelID, chunks); err != nil {
		t.Fatalf("IndexChunks() failed: %v", err)
	}

	// Search for something relevant
	results, err := vs.Search(ctx, novelID, "林风修炼金丹", 3, nil)
	if err != nil {
		t.Fatalf("Search() failed: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("Search() returned no results")
	}

	t.Logf("Search returned %d results", len(results))
	for i, r := range results {
		t.Logf("  [%d] chunk=%s type=%s ch=%d relevance=%.4f content=%.50s...",
			i, r.ChunkID, r.SourceType, r.ChapterNumber, r.Relevance, r.Content)
	}

	// Verify relevance scores are reasonable
	for _, r := range results {
		if r.Relevance < 0 || r.Relevance > 1 {
			t.Errorf("relevance %.4f out of [0,1] range", r.Relevance)
		}
	}
}

func TestVectorStore_SearchWithFilter(t *testing.T) {
	vs := getVectorStore(t)

	ctx := context.Background()
	novelID := int64(10003)
	vs.DeleteNovel(ctx, novelID)

	// Index chunks from multiple chapters
	chunks := []rag.Chunk{
		{ID: "10003_0", Content: "第一章：主角初入江湖", ChapterNumber: 1, ChunkType: "content", ChunkIndex: 0},
		{ID: "10003_1", Content: "第二章：主角遭遇强敌", ChapterNumber: 2, ChunkType: "content", ChunkIndex: 0},
		{ID: "10003_2", Content: "第三章：主角修炼突破", ChapterNumber: 3, ChunkType: "content", ChunkIndex: 0},
	}

	if err := vs.IndexChunks(ctx, novelID, chunks); err != nil {
		t.Fatalf("IndexChunks() failed: %v", err)
	}

	// Search with chapter filter
	filter := &rag.SearchFilter{ChapterNumbers: []int{1, 2}}
	results, err := vs.Search(ctx, novelID, "主角", 10, filter)
	if err != nil {
		t.Fatalf("Search() with filter failed: %v", err)
	}

	// All results should be from chapters 1 or 2
	for _, r := range results {
		if r.ChapterNumber != 1 && r.ChapterNumber != 2 {
			t.Errorf("result from chapter %d, expected 1 or 2", r.ChapterNumber)
		}
	}
	t.Logf("Filter search returned %d results, all from chapters 1-2", len(results))

	// Search with chunk type filter
	typeFilter := &rag.SearchFilter{ChunkTypes: []string{"content"}}
	typeResults, err := vs.Search(ctx, novelID, "主角", 10, typeFilter)
	if err != nil {
		t.Fatalf("Search() with type filter failed: %v", err)
	}
	for _, r := range typeResults {
		if r.SourceType != "content" {
			t.Errorf("result type=%s, expected content", r.SourceType)
		}
	}
	t.Logf("Type filter search returned %d results", len(typeResults))
}

func TestVectorStore_DeleteChapterChunks(t *testing.T) {
	vs := getVectorStore(t)

	ctx := context.Background()
	novelID := int64(10004)
	vs.DeleteNovel(ctx, novelID)

	// Index chunks
	chunks := []rag.Chunk{
		{ID: "10004_0", Content: "第一章内容：英雄出发", ChapterNumber: 1, ChunkType: "content", ChunkIndex: 0},
		{ID: "10004_1", Content: "第二章内容：英雄归来", ChapterNumber: 2, ChunkType: "content", ChunkIndex: 0},
	}
	if err := vs.IndexChunks(ctx, novelID, chunks); err != nil {
		t.Fatalf("IndexChunks() failed: %v", err)
	}

	// Delete chapter 1 chunks
	if err := vs.DeleteChapterChunks(ctx, novelID, 1); err != nil {
		t.Fatalf("DeleteChapterChunks() failed: %v", err)
	}

	// Verify count
	count, err := vs.CountChunks(ctx, novelID)
	if err != nil {
		t.Fatalf("CountChunks() failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 chunk after deletion, got %d", count)
	}

	// Verify search only returns chapter 2
	results, _ := vs.Search(ctx, novelID, "英雄", 10, nil)
	for _, r := range results {
		if r.ChapterNumber == 1 {
			t.Error("found chunk from deleted chapter 1")
		}
	}
}

func TestVectorStore_DeleteNovel(t *testing.T) {
	vs := getVectorStore(t)

	ctx := context.Background()
	novelID := int64(10005)
	vs.DeleteNovel(ctx, novelID)

	// Index chunks
	chunks := []rag.Chunk{
		{ID: "10005_0", Content: "测试内容", ChapterNumber: 1, ChunkType: "content", ChunkIndex: 0},
	}
	if err := vs.IndexChunks(ctx, novelID, chunks); err != nil {
		t.Fatalf("IndexChunks() failed: %v", err)
	}

	// Delete entire novel
	if err := vs.DeleteNovel(ctx, novelID); err != nil {
		t.Fatalf("DeleteNovel() failed: %v", err)
	}

	// Verify count is 0 (table recreated on access)
	count, err := vs.CountChunks(ctx, novelID)
	if err != nil {
		t.Fatalf("CountChunks() after DeleteNovel failed: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 chunks after DeleteNovel, got %d", count)
	}
}
