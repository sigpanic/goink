package mcp_tools

import (
	"context"
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/chapter"
)

func TestEnsureChapterIDsInNovel(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&chapter.Chapter{}); err != nil {
		t.Fatal(err)
	}
	owned := chapter.Chapter{NovelID: 1, SortOrder: 1, Title: "当前小说"}
	other := chapter.Chapter{NovelID: 2, SortOrder: 1, Title: "另一部小说"}
	if err := db.Create(&owned).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&other).Error; err != nil {
		t.Fatal(err)
	}

	tc := ToolContext{DB: db, NovelID: 1}
	result, err := ensureChapterIDsInNovel(context.Background(), tc, []int64{owned.ID, owned.ID})
	if err != nil || result != nil {
		t.Fatalf("valid result = %+v, err = %v", result, err)
	}

	result, err = ensureChapterIDsInNovel(context.Background(), tc, []int64{other.ID, 999})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || result.Success {
		t.Fatalf("invalid result = %+v, want business rejection", result)
	}
	if !strings.Contains(result.Error, "不存在或不属于当前小说") {
		message := result.Error
		t.Errorf("error = %q, want chapter ownership message", message)
	}
}
