package writing

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
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

func int64Ptr(v int64) *int64 { return &v }

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
	var records []WritingLog
	if err := db.Order("id ASC").Find(&records).Error; err != nil {
		t.Fatal(err)
	}
	for _, record := range records {
		if record.ChapterID == nil || *record.ChapterID != 10 {
			t.Errorf("chapter_id = %v, want 10", record.ChapterID)
		}
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
	db.Create(&WritingLog{Date: today, NovelID: 1, ChapterID: int64Ptr(1), WordDelta: 200})
	db.Create(&WritingLog{Date: today, NovelID: 1, ChapterID: int64Ptr(2), WordDelta: 300})

	result, err := s.GetDailyActivity(ctx, 1)
	if err != nil {
		t.Fatalf("GetDailyActivity: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 day, got %d", len(result))
	}
	if result[0].WordsNet != 500 {
		t.Errorf("expected 500 words, got %d", result[0].WordsNet)
	}
}

func TestGetDailyActivity_FiltersNegative(t *testing.T) {
	db := openTestDB(t)
	s := NewStore(db, testLogger())
	ctx := context.Background()

	today := time.Now().Format("2006-01-02")
	db.Create(&WritingLog{Date: today, NovelID: 1, ChapterID: int64Ptr(1), WordDelta: -100})
	db.Create(&WritingLog{Date: today, NovelID: 1, ChapterID: int64Ptr(2), WordDelta: 200})

	result, err := s.GetDailyActivity(ctx, 1)
	if err != nil {
		t.Fatalf("GetDailyActivity: %v", err)
	}
	if len(result) != 1 {
		// 负值被 WHERE word_delta > 0 过滤，正数仍计入
		t.Fatalf("expected 1 day (only positive), got %d", len(result))
	}
	if result[0].WordsAdded != 200 {
		t.Errorf("expected 200 words (only positive), got %d", result[0].WordsAdded)
	}
	require.Equal(t, 100, result[0].WordsDeleted)
	require.Equal(t, 100, result[0].WordsNet)
}

func TestGetDailyActivity_MultipleDays(t *testing.T) {
	db := openTestDB(t)
	s := NewStore(db, testLogger())
	ctx := context.Background()

	d1 := time.Now().AddDate(0, 0, -3).Format("2006-01-02")
	d2 := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	db.Create(&WritingLog{Date: d1, NovelID: 1, ChapterID: int64Ptr(1), WordDelta: 100})
	db.Create(&WritingLog{Date: d2, NovelID: 1, ChapterID: int64Ptr(1), WordDelta: 250})

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
	db.Create(&WritingLog{Date: today, NovelID: 1, ChapterID: int64Ptr(1), WordDelta: 500})
	db.Create(&WritingLog{Date: yesterday, NovelID: 1, ChapterID: int64Ptr(2), WordDelta: 300})

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

	db.Create(&WritingLog{Date: dayBefore, NovelID: 1, ChapterID: int64Ptr(1), WordDelta: 100})
	db.Create(&WritingLog{Date: yesterday, NovelID: 1, ChapterID: int64Ptr(1), WordDelta: 100})
	db.Create(&WritingLog{Date: today, NovelID: 1, ChapterID: int64Ptr(1), WordDelta: 100})

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

	db.Create(&WritingLog{Date: gapDay, NovelID: 1, ChapterID: int64Ptr(1), WordDelta: 100})
	db.Create(&WritingLog{Date: today, NovelID: 1, ChapterID: int64Ptr(1), WordDelta: 100})

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

func TestLogChangesAndLegacyAggregation(t *testing.T) {
	db := openTestDB(t)
	s := NewStore(db, testLogger())
	ctx := context.Background()
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	require.NoError(t, db.Create(&WritingLog{Date: yesterday, WordDelta: 100}).Error)
	require.NoError(t, db.Create(&WritingLog{Date: yesterday, WordDelta: -50}).Error)
	s.LogChanges(ctx, 1, 10, 250, 200)
	s.LogChanges(ctx, 1, 10, 200, 200)
	s.LogChanges(ctx, 1, 10, 100, 150)
	s.LogChanges(ctx, 1, 10, 0, 80)
	s.LogChanges(ctx, 1, 10, 0, 0)

	var logs []WritingLog
	require.NoError(t, db.Order("id").Find(&logs).Error)
	require.Len(t, logs, 6)
	require.Nil(t, logs[0].WordsAdded)
	require.Nil(t, logs[0].WordsDeleted)
	for i, expected := range []struct{ added, deleted int }{{250, 200}, {200, 200}, {100, 150}, {0, 80}} {
		record := logs[i+2]
		require.Equal(t, int64(10), *record.ChapterID)
		require.NotNil(t, record.WordsAdded)
		require.NotNil(t, record.WordsDeleted)
		require.Equal(t, expected.added, *record.WordsAdded)
		require.Equal(t, expected.deleted, *record.WordsDeleted)
		require.Equal(t, expected.added-expected.deleted, record.WordDelta)
	}
	require.NoError(t, db.Model(&WritingLog{}).Where("id = ?", logs[2].ID).Update("date", yesterday).Error)
	daily, err := s.GetDailyActivity(ctx, 1)
	require.NoError(t, err)
	require.Len(t, daily, 2)
	require.Equal(t, 350, daily[0].WordsAdded)
	require.Equal(t, 250, daily[0].WordsDeleted)
	require.Equal(t, 100, daily[0].WordsNet)
	require.Equal(t, 300, daily[1].WordsAdded)
	require.Equal(t, 430, daily[1].WordsDeleted)
	require.Equal(t, -130, daily[1].WordsNet)
	stats, err := s.GetWritingStats(ctx, 1, 1)
	require.NoError(t, err)
	require.Equal(t, 650, stats.TotalWords)
	require.Equal(t, 2, stats.TotalDaysActive)
	require.Equal(t, 2, stats.CurrentStreak)
	require.Equal(t, 2, stats.LongestStreak)
}

func TestEditingActivityIncludesDeletionAndEqualLengthRewrite(t *testing.T) {
	db := openTestDB(t)
	s := NewStore(db, testLogger())
	ctx := context.Background()
	for i, changes := range []struct{ added, deleted int }{{0, 80}, {120, 120}, {200, 300}} {
		date := time.Now().AddDate(0, 0, i-2).Format("2006-01-02")
		require.NoError(t, db.Create(&WritingLog{
			Date: date, WordDelta: changes.added - changes.deleted,
			WordsAdded: &changes.added, WordsDeleted: &changes.deleted,
		}).Error)
	}
	require.NoError(t, db.Create(&WritingLog{Date: time.Now().AddDate(0, 0, -3).Format("2006-01-02")}).Error)
	daily, err := s.GetDailyActivity(ctx, 1)
	require.NoError(t, err)
	require.Len(t, daily, 3)
	require.Equal(t, 0, daily[0].WordsAdded)
	require.Equal(t, 80, daily[0].WordsDeleted)
	require.Equal(t, -80, daily[0].WordsNet)
	require.Equal(t, 120, daily[1].WordsAdded)
	require.Equal(t, 120, daily[1].WordsDeleted)
	require.Zero(t, daily[1].WordsNet)
	require.Equal(t, 200, daily[2].WordsAdded)
	require.Equal(t, 300, daily[2].WordsDeleted)
	require.Equal(t, -100, daily[2].WordsNet)
	stats, err := s.GetWritingStats(ctx, 1, 1)
	require.NoError(t, err)
	require.Equal(t, 320, stats.TotalWords)
	require.Equal(t, 3, stats.TotalDaysActive)
	require.Equal(t, 3, stats.CurrentStreak)
	require.Equal(t, 3, stats.LongestStreak)
}
