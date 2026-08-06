package chapter

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/storage"
)

func openChDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&Chapter{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func testChLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestChListAllByNovel(t *testing.T) {
	db := openChDB(t)
	s := NewStore(db, testChLogger())
	ctx := context.Background()

	db.Create(&Chapter{NovelID: 1, ChapterNumber: 1, Title: "开头"})
	db.Create(&Chapter{NovelID: 1, ChapterNumber: 2, Title: "发展"})
	db.Create(&Chapter{NovelID: 2, ChapterNumber: 1, Title: "另一部"})

	chapters, _ := s.ListAllByNovel(ctx, 1)
	if len(chapters) != 2 {
		t.Errorf("expected 2, got %d", len(chapters))
	}
	if chapters[0].ChapterNumber != 1 {
		t.Errorf("expected chapter 1 first, got %d", chapters[0].ChapterNumber)
	}
}

func TestChListByNovel_Desc(t *testing.T) {
	db := openChDB(t)
	s := NewStore(db, testChLogger())
	ctx := context.Background()

	db.Create(&Chapter{NovelID: 1, ChapterNumber: 1})
	db.Create(&Chapter{NovelID: 1, ChapterNumber: 2})

	result, _ := s.ListByNovel(ctx, 1, ListByNovelOptions{Order: "desc", PageParams: storage.PageParams{Size: -1}})
	if result.Items[0].ChapterNumber != 2 {
		t.Errorf("desc: expected chapter 2 first, got %d", result.Items[0].ChapterNumber)
	}
}

func TestChGetByNovelAndNumber(t *testing.T) {
	db := openChDB(t)
	s := NewStore(db, testChLogger())
	ctx := context.Background()

	db.Create(&Chapter{NovelID: 1, ChapterNumber: 3, Title: "高潮"})

	ch, err := s.GetByNovelAndNumber(ctx, 1, 3)
	if err != nil {
		t.Fatalf("GetByNovelAndNumber: %v", err)
	}
	if ch.Title != "高潮" {
		t.Errorf("expected 高潮, got %s", ch.Title)
	}
}

func TestChGetByNovelAndNumber_NotFound(t *testing.T) {
	db := openChDB(t)
	s := NewStore(db, testChLogger())
	ctx := context.Background()

	_, err := s.GetByNovelAndNumber(ctx, 1, 999)
	if err == nil {
		t.Error("expected error for not found")
	}
}

func TestChGetLatestNumber(t *testing.T) {
	db := openChDB(t)
	s := NewStore(db, testChLogger())
	ctx := context.Background()

	db.Create(&Chapter{NovelID: 1, ChapterNumber: 5})
	db.Create(&Chapter{NovelID: 1, ChapterNumber: 3})

	n, _ := s.GetLatestNumber(ctx, 1)
	if n != 5 {
		t.Errorf("expected 5, got %d", n)
	}
}

func TestChGetLatestNumber_Empty(t *testing.T) {
	db := openChDB(t)
	s := NewStore(db, testChLogger())
	ctx := context.Background()

	n, _ := s.GetLatestNumber(ctx, 1)
	if n != 0 {
		t.Errorf("expected 0 for empty, got %d", n)
	}
}

func TestChGetRecent(t *testing.T) {
	db := openChDB(t)
	s := NewStore(db, testChLogger())
	ctx := context.Background()

	for i := 1; i <= 5; i++ {
		db.Create(&Chapter{NovelID: 1, ChapterNumber: i})
	}

	recent, _ := s.GetRecent(ctx, 1, 2)
	if len(recent) != 2 {
		t.Fatalf("expected 2, got %d", len(recent))
	}
	if recent[0].ChapterNumber != 5 {
		t.Errorf("recent first should be 5, got %d", recent[0].ChapterNumber)
	}
}

func TestListByNovel_Pagination(t *testing.T) {
	db := openChDB(t)
	s := NewStore(db, testChLogger())
	ctx := context.Background()

	for i := 1; i <= 10; i++ {
		db.Create(&Chapter{NovelID: 1, ChapterNumber: i})
	}

	result, _ := s.ListByNovel(ctx, 1, ListByNovelOptions{
		PageParams: storage.PageParams{Page: 2, Size: 3},
	})
	if result.Page != 2 {
		t.Errorf("expected page 2, got %d", result.Page)
	}
	if len(result.Items) != 3 {
		t.Errorf("expected 3 items, got %d", len(result.Items))
	}
}
