package preference

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func openNovDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&PreferenceItem{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func testNovLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestNovListPreferences(t *testing.T) {
	db := openNovDB(t)
	s := NewStore(db, testNovLogger())
	ctx := context.Background()

	db.Create(&PreferenceItem{NovelID: 1, IsGlobal: true, Category: "风格", Content: "简洁"})
	db.Create(&PreferenceItem{NovelID: 1, IsGlobal: false, Category: "角色", Content: "主角性格冷淡"})
	db.Create(&PreferenceItem{NovelID: 2, IsGlobal: false, Category: "对话", Content: "别太啰嗦"})

	items, _ := s.ListPreferences(ctx, 1)
	if len(items) != 2 { // 1 global + 1 for novel 1
		t.Errorf("ListPreferences: expected 2, got %d", len(items))
	}
}

func TestNovListNovelPreferences(t *testing.T) {
	db := openNovDB(t)
	s := NewStore(db, testNovLogger())
	ctx := context.Background()

	db.Create(&PreferenceItem{NovelID: 1, IsGlobal: false, Category: "风格", Content: "简洁"})
	db.Create(&PreferenceItem{NovelID: 1, IsGlobal: true, Category: "对话", Content: "冷"})

	items, _ := s.ListNovelPreferences(ctx, 1)
	if len(items) != 1 {
		t.Errorf("ListNovelPreferences: expected 1 (not global), got %d", len(items))
	}
}

func TestNovListGlobalPreferences(t *testing.T) {
	db := openNovDB(t)
	s := NewStore(db, testNovLogger())
	ctx := context.Background()

	db.Create(&PreferenceItem{NovelID: 0, IsGlobal: true, Category: "全局", Content: "适用于所有"})
	db.Create(&PreferenceItem{NovelID: 1, IsGlobal: false, Category: "专属", Content: "仅此小说"})

	items, _ := s.ListGlobalPreferences(ctx)
	if len(items) != 1 {
		t.Errorf("ListGlobalPreferences: expected 1, got %d", len(items))
	}
}
