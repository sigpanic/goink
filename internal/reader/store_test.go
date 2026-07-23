package reader

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/storage"
)

func openRdDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&ReaderPerspective{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func testRdLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestRdListByNovel_FilterType(t *testing.T) {
	db := openRdDB(t)
	s := NewStore(db, testRdLogger())
	ctx := context.Background()

	db.Create(&ReaderPerspective{NovelID: 1, Type: "known", PlantedChapter: 1})
	db.Create(&ReaderPerspective{NovelID: 1, Type: "suspense", PlantedChapter: 2})

	result, _ := s.ListByNovel(ctx, 1, ListByNovelOptions{Type: "suspense"})
	if result.Total != 1 {
		t.Errorf("filter by type: expected 1, got %d", result.Total)
	}
}

func TestRdListActive(t *testing.T) {
	db := openRdDB(t)
	s := NewStore(db, testRdLogger())
	ctx := context.Background()

	db.Create(&ReaderPerspective{NovelID: 1, Type: "known", PlantedChapter: 1, RevealedChapter: 0})
	db.Create(&ReaderPerspective{NovelID: 1, Type: "suspense", PlantedChapter: 2, RevealedChapter: 5}) // 已揭示
	db.Create(&ReaderPerspective{NovelID: 1, Type: "misconception", PlantedChapter: 3, RevealedChapter: 0})

	active, _ := s.ListActive(ctx, 1)
	if len(active) != 2 {
		t.Errorf("expected 2 active (revealed_chapter=0), got %d", len(active))
	}
}

func TestRdListByNovel_Pagination(t *testing.T) {
	db := openRdDB(t)
	s := NewStore(db, testRdLogger())
	ctx := context.Background()

	for i := 1; i <= 4; i++ {
		db.Create(&ReaderPerspective{NovelID: 1, Type: "known", PlantedChapter: i})
	}

	result, _ := s.ListByNovel(ctx, 1, ListByNovelOptions{
		PageParams: storage.PageParams{Page: 1, Size: 2},
	})
	if len(result.Items) != 2 {
		t.Errorf("expected 2 items, got %d", len(result.Items))
	}
	if result.Total != 4 {
		t.Errorf("total should be 4, got %d", result.Total)
	}
}

// ── CRUD ────────────────────────────────────────────────────

func TestRdCreate(t *testing.T) {
	db := openRdDB(t)
	ctx := context.Background()

	item := ReaderPerspective{
		NovelID: 1, Type: "known", Content: "主角身世未知",
		PlantedChapter: 1, RelatedTruth: "主角是皇帝私生子",
	}
	if err := db.WithContext(ctx).Create(&item).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	if item.ID == 0 {
		t.Error("ID should be set after create")
	}

	var found ReaderPerspective
	db.First(&found, item.ID)
	if found.Content != "主角身世未知" {
		t.Errorf("expected 主角身世未知, got %s", found.Content)
	}
	if found.Type != "known" {
		t.Errorf("expected known, got %s", found.Type)
	}
}

func TestRdUpdate(t *testing.T) {
	db := openRdDB(t)
	ctx := context.Background()

	item := ReaderPerspective{NovelID: 1, Type: "suspense", Content: "旧悬念", PlantedChapter: 2}
	db.WithContext(ctx).Create(&item)

	type UpdateInput struct {
		Content         string `json:"content,omitempty"`
		Type            string `json:"type,omitempty"`
		RevealedChapter int    `json:"revealed_chapter,omitempty"`
	}
	input := UpdateInput{RevealedChapter: 5, Type: ""}
	if err := db.WithContext(ctx).Model(&ReaderPerspective{}).Where("id = ?", item.ID).Updates(&input).Error; err != nil {
		t.Fatalf("update: %v", err)
	}

	var updated ReaderPerspective
	db.WithContext(ctx).First(&updated, item.ID)
	if updated.RevealedChapter != 5 {
		t.Errorf("revealed_chapter: expected 5, got %d", updated.RevealedChapter)
	}
	if updated.Type != "suspense" {
		t.Errorf("type should be unchanged (empty string skipped), got %s", updated.Type)
	}
}

func TestRdDelete(t *testing.T) {
	db := openRdDB(t)
	ctx := context.Background()

	item := ReaderPerspective{NovelID: 1, Type: "known", Content: "待删", PlantedChapter: 1}
	db.WithContext(ctx).Create(&item)

	if err := db.WithContext(ctx).Where("id = ?", item.ID).Delete(&ReaderPerspective{}).Error; err != nil {
		t.Fatalf("delete: %v", err)
	}

	var found ReaderPerspective
	if db.First(&found, item.ID).Error == nil {
		t.Error("item should be deleted")
	}
}
