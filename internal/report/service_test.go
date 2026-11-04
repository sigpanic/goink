package report

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/sigpanic/goink/internal/activity"
	"github.com/sigpanic/goink/internal/config"
	"github.com/sigpanic/goink/internal/novel"
	"github.com/sigpanic/goink/internal/writing"
)

func newTestService(t *testing.T, start string, now time.Time) (*Service, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&activity.DailyActivity{}, &activity.DailyLLMUsage{}, &config.AppSettings{}, &writing.WritingLog{}, &novel.Novel{}))
	startedAt, err := time.ParseInLocation(time.DateOnly, start, now.Location())
	require.NoError(t, err)
	startedAt = startedAt.UTC()
	require.NoError(t, db.Create(&config.AppSettings{ID: 1, TrackingStartedAt: &startedAt, TrackingStartDate: start}).Error)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	s := NewService(activity.NewStore(db, log), writing.NewStore(db, log))
	s.now = func() time.Time { return now }
	return s, db
}

func TestGetAnnualAggregatesCoveredDates(t *testing.T) {
	now := time.Date(2025, 1, 2, 0, 30, 0, 0, time.FixedZone("UTC+8", 8*60*60))
	s, db := newTestService(t, "2024-02-28", now)
	require.NoError(t, db.Create(&[]novel.Novel{{ID: 1, Title: "第一部"}, {ID: 2, Title: "第二部"}}).Error)
	require.NoError(t, db.Create(&[]activity.DailyActivity{
		{Date: "2024-02-27", ActivityDelta: activity.ActivityDelta{CreativeSeconds: 9999, StoryArcsCreated: 9999, SkillsCreated: 9999, SessionsCreated: 9999}},
		{Date: "2024-02-28", ActivityDelta: activity.ActivityDelta{
			NovelsCreated: 1, ChaptersCreated: 2, CharactersCreated: 3, LocationsCreated: 4,
			VolumesCreated: 1, CharacterGroupsCreated: 2, CharacterRelationsCreated: 3,
			LocationRelationsCreated: 4, StoryArcsCreated: 5, ArcNodesCreated: 6,
			TimelineEntriesCreated: 7, ReaderPerspectivesCreated: 8, SettingsCreated: 9,
			PreferencesCreated: 10, StyleSamplesCreated: 11, SkillsCreated: 12, ImagesImported: 13,
			SessionsCreated: 1, CreativeSeconds: 3600, ConversationTurns: 4, ToolCalls: 5,
		}},
		{Date: "2024-02-29", ActivityDelta: activity.ActivityDelta{
			VolumesCreated: 1, CharacterGroupsCreated: 2, CharacterRelationsCreated: 3,
			LocationRelationsCreated: 4, StoryArcsCreated: 5, ArcNodesCreated: 6,
			TimelineEntriesCreated: 7, ReaderPerspectivesCreated: 8, SettingsCreated: 9,
			PreferencesCreated: 10, StyleSamplesCreated: 11, SkillsCreated: 12, ImagesImported: 13,
			SessionsCreated: 2, CreativeSeconds: 1800, ConversationTurns: 1, ToolCalls: 2,
		}},
		{Date: "2024-03-01", ActivityDelta: activity.ActivityDelta{CharactersCreated: 1, CreativeSeconds: 600}},
		{Date: "2024-12-31", ActivityDelta: activity.ActivityDelta{NovelsCreated: 1, ChaptersCreated: 1, CreativeSeconds: 60}},
		{Date: "2025-01-01", ActivityDelta: activity.ActivityDelta{CreativeSeconds: 9999, StoryArcsCreated: 9999, ImagesImported: 9999, SessionsCreated: 9999}},
	}).Error)
	require.NoError(t, db.Create(&[]writing.WritingLog{
		{Date: "2024-02-27", NovelID: 1, WordsAdded: 9999, WordDelta: 9999},
		{Date: "2024-02-28", NovelID: 1, WordsAdded: 100, WordsDeleted: 20, WordDelta: 80},
		{Date: "2024-02-28", NovelID: 2, WordsAdded: 50, WordsDeleted: 10, WordDelta: 40},
		{Date: "2024-02-29", NovelID: 1, WordsDeleted: 40, WordDelta: -40},
		{Date: "2024-02-29", NovelID: 4, WordsDeleted: 10, WordDelta: -10},
		{Date: "2024-03-01", NovelID: 1, WordsAdded: 30, WordsDeleted: 30},
		{Date: "2024-03-02", NovelID: 1},
		{Date: "2024-03-03", NovelID: 2, WordsAdded: 40, WordDelta: 40},
		{Date: "2024-12-30", NovelID: 2, WordsAdded: 5, WordDelta: 5},
		{Date: "2024-12-31", NovelID: 3, WordsAdded: 90, WordDelta: 90},
		{Date: "2025-01-01", NovelID: 1, WordsAdded: 9999, WordDelta: 9999},
	}).Error)
	require.NoError(t, db.Create(&[]activity.DailyLLMUsage{
		{Date: "2024-02-27", Provider: "a", Model: "shared", Purpose: "chat", UsageCount: 999, TokenUsage: activity.TokenUsage{TotalTokens: 9999}},
		{Date: "2024-02-28", Provider: "a", Model: "shared", Purpose: "chat", UsageCount: 2, TokenUsage: activity.TokenUsage{InputTokens: 3_000_000_000, OutputTokens: 10, TotalTokens: 3_000_000_099, CacheHitTokens: 20, CacheMissTokens: 40, ReasoningTokens: 1}},
		{Date: "2024-02-29", Provider: "a", Model: "shared", Purpose: "review", UsageCount: 1, TokenUsage: activity.TokenUsage{InputTokens: 100, OutputTokens: 20, TotalTokens: 121, CacheHitTokens: 2, CacheMissTokens: 3, ReasoningTokens: 4}},
		{Date: "2024-03-01", Provider: "b", Model: "shared", Purpose: "chat", UsageCount: 2, TokenUsage: activity.TokenUsage{InputTokens: 20, OutputTokens: 5, TotalTokens: 25}},
		{Date: "2024-03-01", Provider: "a", Model: "other", Purpose: "chat", UsageCount: 2},
		{Date: "2024-12-31", Provider: "a", Model: "shared", Purpose: "chat", UsageCount: 1, TokenUsage: activity.TokenUsage{InputTokens: 7, OutputTokens: 3, TotalTokens: 10}},
		{Date: "2025-01-01", Provider: "a", Model: "shared", Purpose: "chat", UsageCount: 999, TokenUsage: activity.TokenUsage{TotalTokens: 9999}},
	}).Error)

	r, err := s.GetAnnual(context.Background(), 2024)
	require.NoError(t, err)
	require.Equal(t, 2024, r.Year)
	require.Equal(t, &DateRange{Start: "2024-02-28", End: "2025-01-01"}, r.Coverage)
	require.Equal(t, "2024-02-28", r.TrackingStartDate)
	require.Equal(t, "2024-02-27", r.TrackingStartedAt.UTC().Format(time.DateOnly))
	require.Equal(t, activity.ActivityDelta{
		NovelsCreated: 2, ChaptersCreated: 3, CharactersCreated: 4, LocationsCreated: 4,
		VolumesCreated: 2, CharacterGroupsCreated: 4, CharacterRelationsCreated: 6,
		LocationRelationsCreated: 8, StoryArcsCreated: 10, ArcNodesCreated: 12,
		TimelineEntriesCreated: 14, ReaderPerspectivesCreated: 16, SettingsCreated: 18,
		PreferencesCreated: 20, StyleSamplesCreated: 22, SkillsCreated: 24, ImagesImported: 26,
		SessionsCreated: 3, CreativeSeconds: 6060, ConversationTurns: 5, ToolCalls: 7,
	}, r.Activity)
	require.Equal(t, WritingSummary{WordsAdded: 315, WordsDeleted: 110, WordsNet: 205, ActiveDays: 6, LongestStreak: 3}, r.Writing)
	require.Equal(t, activity.TokenUsage{InputTokens: 3_000_000_127, OutputTokens: 38, TotalTokens: 3_000_000_255, CacheHitTokens: 22, CacheMissTokens: 43, ReasoningTokens: 5}, r.Usage)
	require.EqualValues(t, 8, r.UsageCount)
	require.Len(t, r.DailyActivity, 4)
	require.Len(t, r.DailyWriting, 6)
	require.Len(t, r.DailyLLMUsage, 5)
	require.Len(t, r.Monthly, 12)
	require.Equal(t, MonthlySummary{Month: 1}, r.Monthly[0])
	require.Equal(t, MonthlySummary{Month: 2, WordsAdded: 150, CreativeSeconds: 5400}, r.Monthly[1])
	require.Equal(t, MonthlySummary{Month: 3, WordsAdded: 70, CreativeSeconds: 600}, r.Monthly[2])
	require.Equal(t, MonthlySummary{Month: 12, WordsAdded: 95, CreativeSeconds: 60}, r.Monthly[11])
	require.Equal(t, []writing.NovelWords{
		{NovelID: 1, Title: "第一部", WordsAdded: 130},
		{NovelID: 2, Title: "第二部", WordsAdded: 95},
		{NovelID: 3, Deleted: true, WordsAdded: 90},
	}, r.Novels)
	require.Equal(t, []ModelUsage{
		{Provider: "a", Model: "shared", UsageCount: 4, TokenUsage: activity.TokenUsage{InputTokens: 3_000_000_107, OutputTokens: 33, TotalTokens: 3_000_000_230, CacheHitTokens: 22, CacheMissTokens: 43, ReasoningTokens: 5}},
		{Provider: "a", Model: "other", UsageCount: 2},
		{Provider: "b", Model: "shared", UsageCount: 2, TokenUsage: activity.TokenUsage{InputTokens: 20, OutputTokens: 5, TotalTokens: 25}},
	}, r.Models)
	var monthlyWords int
	var monthlySeconds int64
	for _, month := range r.Monthly {
		monthlyWords += month.WordsAdded
		monthlySeconds += month.CreativeSeconds
	}
	require.Equal(t, r.Writing.WordsAdded, monthlyWords)
	require.Equal(t, r.Activity.CreativeSeconds, monthlySeconds)
}

func TestGetAnnualCoverageAndEmptyData(t *testing.T) {
	now := time.Date(2025, 1, 2, 0, 30, 0, 0, time.FixedZone("UTC+8", 8*60*60))
	s, db := newTestService(t, "2024-05-01", now)
	for _, tc := range []struct {
		year     int
		coverage *DateRange
	}{
		{2023, nil},
		{2024, &DateRange{Start: "2024-05-01", End: "2025-01-01"}},
		{2025, &DateRange{Start: "2025-01-01", End: "2025-01-03"}},
	} {
		r, err := s.GetAnnual(context.Background(), tc.year)
		require.NoError(t, err)
		require.Equal(t, tc.coverage, r.Coverage)
		require.Zero(t, r.Writing)
		require.Zero(t, r.Activity)
		require.Zero(t, r.Usage)
		require.NotNil(t, r.DailyActivity)
		require.Empty(t, r.DailyActivity)
		require.NotNil(t, r.DailyWriting)
		require.Empty(t, r.DailyWriting)
		require.NotNil(t, r.DailyLLMUsage)
		require.Empty(t, r.DailyLLMUsage)
		require.NotNil(t, r.Novels)
		require.Empty(t, r.Novels)
		require.NotNil(t, r.Models)
		require.Empty(t, r.Models)
		require.Len(t, r.Monthly, 12)
	}
	for _, date := range []string{"2024-12-31", "2025-01-01", "2025-01-02", "2025-01-03"} {
		require.NoError(t, db.Create(&activity.DailyActivity{Date: date, ActivityDelta: activity.ActivityDelta{CreativeSeconds: 60}}).Error)
		require.NoError(t, db.Create(&writing.WritingLog{Date: date, NovelID: 1, WordsAdded: 1, WordDelta: 1}).Error)
		require.NoError(t, db.Create(&activity.DailyLLMUsage{Date: date, Provider: "p", Model: "m", Purpose: "chat", UsageCount: 1}).Error)
	}
	r, err := s.GetAnnual(context.Background(), 2025)
	require.NoError(t, err)
	require.EqualValues(t, 120, r.Activity.CreativeSeconds)
	require.Equal(t, WritingSummary{WordsAdded: 2, WordsNet: 2, ActiveDays: 2, LongestStreak: 2}, r.Writing)
	require.EqualValues(t, 2, r.UsageCount)
	require.Len(t, r.Novels, 1)
	require.Equal(t, 2, r.Novels[0].WordsAdded)
	for _, year := range []int{-1, 0, 2026, 9999} {
		_, err := s.GetAnnual(context.Background(), year)
		require.ErrorIs(t, err, ErrInvalidYear)
	}
}

func TestGetAnnualStreakUsesCalendarDates(t *testing.T) {
	location, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)
	for _, tc := range []struct {
		name  string
		dates []string
	}{
		{"spring forward", []string{"2024-03-09", "2024-03-10", "2024-03-11"}},
		{"fall back", []string{"2024-11-02", "2024-11-03", "2024-11-04"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, db := newTestService(t, "2024-01-01", time.Date(2025, 1, 1, 0, 0, 0, 0, location))
			for _, date := range append(tc.dates, "2023-12-31", "2025-01-01") {
				require.NoError(t, db.Create(&writing.WritingLog{Date: date, NovelID: 1, WordsDeleted: 1, WordDelta: -1}).Error)
			}
			r, err := s.GetAnnual(context.Background(), 2024)
			require.NoError(t, err)
			require.Equal(t, WritingSummary{WordsDeleted: 3, WordsNet: -3, ActiveDays: 3, LongestStreak: 3}, r.Writing)
			require.Empty(t, r.Novels)
		})
	}
}

func TestGetAnnualRequiresTrackingOrigin(t *testing.T) {
	for _, tc := range []struct {
		name       string
		update     map[string]any
		remove     bool
		errorMatch error
	}{
		{name: "missing configuration", remove: true, errorMatch: gorm.ErrRecordNotFound},
		{name: "tracking not initialized", update: map[string]any{"tracking_started_at": nil}, errorMatch: ErrTrackingUnavailable},
		{name: "missing local date", update: map[string]any{"tracking_start_date": ""}, errorMatch: ErrTrackingUnavailable},
		{name: "invalid local date", update: map[string]any{"tracking_start_date": "2024-02-30"}, errorMatch: ErrTrackingUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, db := newTestService(t, "2024-01-01", time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
			if tc.remove {
				require.NoError(t, db.Delete(&config.AppSettings{}, 1).Error)
			} else {
				require.NoError(t, db.Model(&config.AppSettings{}).Where("id = ?", 1).Updates(tc.update).Error)
			}
			r, err := s.GetAnnual(context.Background(), 2024)
			require.Nil(t, r)
			require.ErrorIs(t, err, tc.errorMatch)
			if tc.remove {
				var count int64
				require.NoError(t, db.Model(&config.AppSettings{}).Count(&count).Error)
				require.Zero(t, count)
			}
		})
	}
}

func TestGetAnnualPropagatesQueryFailures(t *testing.T) {
	for _, table := range []string{"app_config", "activity_daily", "writing_log", "llm_usage_daily", "novels"} {
		t.Run(table, func(t *testing.T) {
			s, db := newTestService(t, "2024-01-01", time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
			require.NoError(t, db.Migrator().DropTable(table))
			r, err := s.GetAnnual(context.Background(), 2024)
			require.Error(t, err)
			require.Nil(t, r)
		})
	}
	t.Run("canceled query", func(t *testing.T) {
		s, _ := newTestService(t, "2024-01-01", time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		r, err := s.GetAnnual(ctx, 2024)
		require.ErrorIs(t, err, context.Canceled)
		require.Nil(t, r)
	})
}
