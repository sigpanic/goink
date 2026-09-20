package mcp_tools

import (
	"context"
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/chapter"
	"github.com/sigpanic/goink/internal/reader"
)

func TestCreateReaderPerspectiveEntry_ValidatesChapterNovel(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&chapter.Chapter{}, &reader.ReaderPerspective{}); err != nil {
		t.Fatal(err)
	}
	currentChapter := chapter.Chapter{NovelID: 1, SortOrder: 1, Title: "第一章"}
	otherNovelChapter := chapter.Chapter{NovelID: 2, SortOrder: 1, Title: "另一部小说"}
	if err := db.Create(&currentChapter).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&otherNovelChapter).Error; err != nil {
		t.Fatal(err)
	}

	tool := &CreateReaderPerspectiveEntryTool{}
	tc := ToolContext{DB: db, NovelID: 1}
	result, err := tool.Execute(context.Background(), &CreateReaderPerspectiveEntryArgs{Entries: []CreateReaderPerspectiveEntryItem{{
		Type:             reader.TypeKnown,
		Content:          "读者知道真相",
		PlantedChapterID: currentChapter.ID,
	}}}, tc)
	if err != nil || !result.Success {
		t.Fatalf("create result = %+v, err = %v", result, err)
	}

	var created reader.ReaderPerspective
	if err := db.First(&created).Error; err != nil {
		t.Fatal(err)
	}
	if created.PlantedChapterID == nil || *created.PlantedChapterID != currentChapter.ID {
		t.Errorf("planted chapter ID = %v, want %d", created.PlantedChapterID, currentChapter.ID)
	}

	result, err = tool.Execute(context.Background(), &CreateReaderPerspectiveEntryArgs{Entries: []CreateReaderPerspectiveEntryItem{{
		Type:             reader.TypeKnown,
		Content:          "跨小说引用",
		PlantedChapterID: otherNovelChapter.ID,
	}}}, tc)
	if err != nil {
		t.Fatal(err)
	}
	if result.Success || !strings.Contains(result.Error, "不属于当前小说") {
		t.Errorf("cross-novel result = %+v, want business rejection", result)
	}
}

func TestFormatReaderPerspective_MissingPlantedChapter(t *testing.T) {
	formatted := formatReaderPerspective(
		[]reader.ReaderPerspective{{ID: 1, Content: "历史条目"}},
		nil,
		nil,
		nil,
	)
	if !strings.Contains(formatted, "章节信息缺失") {
		t.Errorf("formatted = %q, want missing chapter label", formatted)
	}
}
