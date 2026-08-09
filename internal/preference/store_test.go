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

func TestNovListNovelPreferences(t *testing.T) {
	db := openNovDB(t)
	s := NewStore(db, testNovLogger())
	ctx := context.Background()

	db.Create(&PreferenceItem{NovelID: 1, IsGlobal: false, Category: "风格", Content: "简洁"})
	db.Create(&PreferenceItem{NovelID: 1, IsGlobal: true, Category: "对话", Content: "冷"})

	result, _ := s.ListNovelPreferences(ctx, 1, ListOptions{})
	if result.Total != 1 {
		t.Errorf("ListNovelPreferences: expected 1 (not global), got %d", result.Total)
	}
}

func TestNovListGlobalPreferences(t *testing.T) {
	db := openNovDB(t)
	s := NewStore(db, testNovLogger())
	ctx := context.Background()

	db.Create(&PreferenceItem{NovelID: 0, IsGlobal: true, Category: "全局", Content: "适用于所有"})
	db.Create(&PreferenceItem{NovelID: 1, IsGlobal: false, Category: "专属", Content: "仅此小说"})

	result, _ := s.ListGlobalPreferences(ctx, ListOptions{})
	if result.Total != 1 {
		t.Errorf("ListGlobalPreferences: expected 1, got %d", result.Total)
	}
}

func TestNovListNovelPreferences_Search(t *testing.T) {
	db := openNovDB(t)
	s := NewStore(db, testNovLogger())
	ctx := context.Background()

	db.Create(&PreferenceItem{NovelID: 1, IsGlobal: false, Category: "风格", Content: "简洁有力"})
	db.Create(&PreferenceItem{NovelID: 1, IsGlobal: false, Category: "角色", Content: "主角性格冷淡"})
	db.Create(&PreferenceItem{NovelID: 1, IsGlobal: true, Category: "风格", Content: "全局风格"})

	// search 命中 content
	result, _ := s.ListNovelPreferences(ctx, 1, ListOptions{Search: "简洁"})
	if result.Total != 1 {
		t.Errorf("search content: expected 1, got %d", result.Total)
	}

	// search 命中 category（全局的不应被 novel 查询命中）
	result, _ = s.ListNovelPreferences(ctx, 1, ListOptions{Search: "风格"})
	if result.Total != 1 {
		t.Errorf("search category: expected 1 (novel only), got %d", result.Total)
	}

	// 全局查询 search 命中 category
	gResult, _ := s.ListGlobalPreferences(ctx, ListOptions{Search: "风格"})
	if gResult.Total != 1 {
		t.Errorf("global search category: expected 1, got %d", gResult.Total)
	}
}
