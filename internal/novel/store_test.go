package novel

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/storage"
)

func openNovDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&Novel{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func testNovLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestNovList(t *testing.T) {
	db := openNovDB(t)
	s := NewStore(db, testNovLogger())
	ctx := context.Background()

	db.Create(&Novel{Title: "小说A"})
	db.Create(&Novel{Title: "小说B"})

	result, _ := s.List(ctx, ListNovelsOptions{})
	if result.Total != 2 {
		t.Errorf("expected 2, got %d", result.Total)
	}
}

func TestNovList_Search(t *testing.T) {
	db := openNovDB(t)
	s := NewStore(db, testNovLogger())
	ctx := context.Background()

	db.Create(&Novel{Title: "仙逆"})
	db.Create(&Novel{Title: "斗破苍穹"})

	result, _ := s.List(ctx, ListNovelsOptions{Search: "仙"})
	if result.Total != 1 {
		t.Errorf("search: expected 1, got %d", result.Total)
	}
	if result.Items[0].Title != "仙逆" {
		t.Errorf("expected 仙逆, got %s", result.Items[0].Title)
	}
}

func TestNovList_Genre(t *testing.T) {
	db := openNovDB(t)
	s := NewStore(db, testNovLogger())
	ctx := context.Background()

	db.Create(&Novel{Title: "A", Genre: "玄幻"})
	db.Create(&Novel{Title: "B", Genre: "科幻"})

	result, _ := s.List(ctx, ListNovelsOptions{Genre: "玄幻"})
	if result.Total != 1 {
		t.Errorf("genre filter: expected 1, got %d", result.Total)
	}
}

func TestNovList_Pagination(t *testing.T) {
	db := openNovDB(t)
	s := NewStore(db, testNovLogger())
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		db.Create(&Novel{Title: "A"})
	}

	result, _ := s.List(ctx, ListNovelsOptions{
		PageParams: storage.PageParams{Page: 1, Size: 2},
	})
	if len(result.Items) != 2 {
		t.Errorf("expected 2 items, got %d", len(result.Items))
	}
	if result.Total != 5 {
		t.Errorf("total should be 5, got %d", result.Total)
	}
}
