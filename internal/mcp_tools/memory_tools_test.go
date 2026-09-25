//go:build cgo

package mcp_tools

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/sigpanic/goink/internal/chapter"
	"github.com/sigpanic/goink/internal/rag"
)

func TestSearchStoryMemorySchema_UsesChapterIDs(t *testing.T) {
	var schema map[string]any
	if err := json.Unmarshal((&SearchStoryMemoryTool{}).JSONSchema(), &schema); err != nil {
		t.Fatal(err)
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("schema has no properties")
	}
	if _, ok := properties["chapter_ids"]; !ok {
		t.Error("schema should expose chapter_ids")
	}
	if _, ok := properties["chapter_numbers"]; ok {
		t.Error("schema must not expose legacy chapter_numbers")
	}
}

func TestFormatSearchStoryMemoryResults_UsesChapterIDAndReadingNumber(t *testing.T) {
	content, maxRelevance := formatSearchStoryMemoryResults(
		"城门",
		[]rag.SearchResult{{
			ChapterID:  42,
			Content:    "主角抵达城门。",
			SourceType: "content",
			Relevance:  0.86,
		}},
		map[int64]chapter.Chapter{
			42: {ID: 42, Title: "夜入皇城", ReadingNumber: 1},
		},
	)

	if maxRelevance != 0.86 {
		t.Errorf("max relevance = %v, want 0.86", maxRelevance)
	}
	if !strings.Contains(content, "第1章 夜入皇城 [chapter_id:42]") {
		t.Errorf("content = %q, want chapter found by ID with reading number", content)
	}
}
