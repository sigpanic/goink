package writing

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := db.AutoMigrate(&WritingLog{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestLogDelta_Insert(t *testing.T) {
	db := openTestDB(t)
	s := NewStore(db, testLogger())
	ctx := context.Background()

	s.LogDelta(ctx, 1, 10, 500)
	s.LogDelta(ctx, 1, 10, 300)

	var count int64
	db.Model(&WritingLog{}).Count(&count)
	if count != 2 {
		t.Errorf("expected 2 records, got %d", count)
	}
}

func TestLogDelta_SkipsZero(t *testing.T) {
	db := openTestDB(t)
	s := NewStore(db, testLogger())
	ctx := context.Background()

	s.LogDelta(ctx, 1, 1, 0)

	var count int64
	db.Model(&WritingLog{}).Count(&count)
	if count != 0 {
		t.Errorf("zero delta should be skipped, got %d records", count)
	}
}

func TestGetDailyActivity_Basic(t *testing.T) {
	db := openTestDB(t)
	s := NewStore(db, testLogger())
	ctx := context.Background()

	today := time.Now().Format("2006-01-02")
	db.Create(&WritingLog{Date: today, NovelID: 1, ChapterNumber: 1, WordDelta: 200})
	db.Create(&WritingLog{Date: today, NovelID: 1, ChapterNumber: 2, WordDelta: 300})

	result, err := s.GetDailyActivity(ctx, 1)
	if err != nil {
		t.Fatalf("GetDailyActivity: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 day, got %d", len(result))
	}
	if result[0].Words != 500 {
		t.Errorf("expected 500 words, got %d", result[0].Words)
	}
}

func TestGetDailyActivity_FiltersNegative(t *testing.T) {
	db := openTestDB(t)
	s := NewStore(db, testLogger())
	ctx := context.Background()

	today := time.Now().Format("2006-01-02")
	db.Create(&WritingLog{Date: today, NovelID: 1, ChapterNumber: 1, WordDelta: -100})
	db.Create(&WritingLog{Date: today, NovelID: 1, ChapterNumber: 2, WordDelta: 200})

	result, err := s.GetDailyActivity(ctx, 1)
	if err != nil {
		t.Fatalf("GetDailyActivity: %v", err)
	}
	if len(result) != 1 {
		// 负值被 WHERE word_delta > 0 过滤，正数仍计入
		t.Fatalf("expected 1 day (only positive), got %d", len(result))
	}
	if result[0].Words != 200 {
		t.Errorf("expected 200 words (only positive), got %d", result[0].Words)
	}
}

func TestGetDailyActivity_MultipleDays(t *testing.T) {
	db := openTestDB(t)
	s := NewStore(db, testLogger())
	ctx := context.Background()

	d1 := time.Now().AddDate(0, 0, -3).Format("2006-01-02")
	d2 := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	db.Create(&WritingLog{Date: d1, NovelID: 1, ChapterNumber: 1, WordDelta: 100})
	db.Create(&WritingLog{Date: d2, NovelID: 1, ChapterNumber: 1, WordDelta: 250})

	result, err := s.GetDailyActivity(ctx, 1)
	if err != nil {
		t.Fatalf("GetDailyActivity: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 days, got %d", len(result))
	}
	if result[0].Date != d1 {
		t.Errorf("first date should be %s, got %s", d1, result[0].Date)
	}
	if result[1].Date != d2 {
		t.Errorf("second date should be %s, got %s", d2, result[1].Date)
	}
}

func TestGetWritingStats_Basic(t *testing.T) {
	db := openTestDB(t)
	s := NewStore(db, testLogger())
	ctx := context.Background()

	today := time.Now().Format("2006-01-02")
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	db.Create(&WritingLog{Date: today, NovelID: 1, ChapterNumber: 1, WordDelta: 500})
	db.Create(&WritingLog{Date: yesterday, NovelID: 1, ChapterNumber: 2, WordDelta: 300})

	stats, err := s.GetWritingStats(ctx, 2, 5)
	if err != nil {
		t.Fatalf("GetWritingStats: %v", err)
	}
	if stats.TotalWords != 800 {
		t.Errorf("TotalWords: expected 800, got %d", stats.TotalWords)
	}
	if stats.TotalDaysActive != 2 {
		t.Errorf("TotalDaysActive: expected 2, got %d", stats.TotalDaysActive)
	}
	if stats.TotalNovels != 2 {
		t.Errorf("TotalNovels: expected 2, got %d", stats.TotalNovels)
	}
	if stats.TotalChapters != 5 {
		t.Errorf("TotalChapters: expected 5, got %d", stats.TotalChapters)
	}
}

func TestGetWritingStats_CurrentStreak(t *testing.T) {
	db := openTestDB(t)
	s := NewStore(db, testLogger())
	ctx := context.Background()

	today := time.Now().Format("2006-01-02")
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	dayBefore := time.Now().AddDate(0, 0, -2).Format("2006-01-02")

	db.Create(&WritingLog{Date: dayBefore, NovelID: 1, ChapterNumber: 1, WordDelta: 100})
	db.Create(&WritingLog{Date: yesterday, NovelID: 1, ChapterNumber: 1, WordDelta: 100})
	db.Create(&WritingLog{Date: today, NovelID: 1, ChapterNumber: 1, WordDelta: 100})

	stats, err := s.GetWritingStats(ctx, 1, 1)
	if err != nil {
		t.Fatalf("GetWritingStats: %v", err)
	}
	// 三天连续且最新是今天 → currentStreak >= 3
	if stats.CurrentStreak != 3 {
		t.Errorf("CurrentStreak: expected 3, got %d", stats.CurrentStreak)
	}
	if stats.LongestStreak != 3 {
		t.Errorf("LongestStreak: expected 3, got %d", stats.LongestStreak)
	}
}

func TestGetWritingStats_BrokenStreak(t *testing.T) {
	db := openTestDB(t)
	s := NewStore(db, testLogger())
	ctx := context.Background()

	today := time.Now().Format("2006-01-02")
	gapDay := time.Now().AddDate(0, 0, -3).Format("2006-01-02") // 断了

	db.Create(&WritingLog{Date: gapDay, NovelID: 1, ChapterNumber: 1, WordDelta: 100})
	db.Create(&WritingLog{Date: today, NovelID: 1, ChapterNumber: 1, WordDelta: 100})

	stats, err := s.GetWritingStats(ctx, 1, 1)
	if err != nil {
		t.Fatalf("GetWritingStats: %v", err)
	}
	if stats.CurrentStreak != 1 {
		t.Errorf("CurrentStreak: expected 1 (only today), got %d", stats.CurrentStreak)
	}
	if stats.LongestStreak != 1 {
		t.Errorf("LongestStreak: expected 1, got %d", stats.LongestStreak)
	}
}

func TestGetWritingStats_EmptyData(t *testing.T) {
	db := openTestDB(t)
	s := NewStore(db, testLogger())
	ctx := context.Background()

	stats, err := s.GetWritingStats(ctx, 0, 0)
	if err != nil {
		t.Fatalf("GetWritingStats: %v", err)
	}
	if stats.TotalWords != 0 || stats.TotalDaysActive != 0 {
		t.Errorf("empty data should be zeros")
	}
	if stats.CurrentStreak != 0 || stats.LongestStreak != 0 {
		t.Errorf("empty data should have zero streaks")
	}
}
