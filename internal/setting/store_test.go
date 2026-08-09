package setting

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func openSettingDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&SettingItem{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func testSettingLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestSettingListByNovel(t *testing.T) {
	db := openSettingDB(t)
	s := NewStore(db, testSettingLogger())
	ctx := context.Background()

	db.Create(&SettingItem{NovelID: 1, Category: "世界观", Content: "修仙体系"})
	db.Create(&SettingItem{NovelID: 1, Category: "角色", Content: "主角武器"})
	db.Create(&SettingItem{NovelID: 2, Category: "地理", Content: "大陆格局"})

	result, _ := s.ListByNovel(ctx, 1, ListOptions{})
	if result.Total != 2 {
		t.Errorf("ListByNovel: expected 2 (novel 1), got %d", result.Total)
	}
}

func TestSettingListByNovel_Search(t *testing.T) {
	db := openSettingDB(t)
	s := NewStore(db, testSettingLogger())
	ctx := context.Background()

	db.Create(&SettingItem{NovelID: 1, Category: "世界观", Content: "修仙体系"})
	db.Create(&SettingItem{NovelID: 1, Category: "角色", Content: "主角性格冷淡"})
	db.Create(&SettingItem{NovelID: 2, Category: "世界观", Content: "另一本小说的设定"})

	// search 命中 content
	result, _ := s.ListByNovel(ctx, 1, ListOptions{Search: "修仙"})
	if result.Total != 1 {
		t.Errorf("search content: expected 1, got %d", result.Total)
	}

	// search 命中 category（novel 2 的不应被命中）
	result, _ = s.ListByNovel(ctx, 1, ListOptions{Search: "世界观"})
	if result.Total != 1 {
		t.Errorf("search category: expected 1 (novel 1 only), got %d", result.Total)
	}
}
