package activity

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/sigpanic/goink/internal/config"
	"github.com/sigpanic/goink/internal/storage"
)

func newTestStore(t *testing.T) (*Store, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&DailyActivity{}, &DailyLLMUsage{}, &config.AppSettings{}, &storage.OperationLogRecord{}))
	s := NewStore(db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	s.now = func() time.Time {
		return time.Date(2027, 1, 1, 0, 30, 0, 0, time.FixedZone("UTC+8", 8*60*60))
	}
	return s, db
}

func TestInitTrackingPreservesOriginAndSettings(t *testing.T) {
	for _, existing := range []bool{false, true} {
		name := "new settings"
		if existing {
			name = "existing settings"
		}
		t.Run(name, func(t *testing.T) {
			s, db := newTestStore(t)
			if existing {
				require.NoError(t, db.Create(&config.AppSettings{ID: 1, UserName: "原作者", SelectedModelKey: "provider/model"}).Error)
			}
			startedAt := s.now()
			s.InitTracking(context.Background())
			settings, err := config.LoadSettings(db)
			require.NoError(t, err)
			require.NotNil(t, settings.TrackingStartedAt)
			require.True(t, startedAt.Equal(*settings.TrackingStartedAt))
			require.Equal(t, "2027-01-01", settings.TrackingStartDate)
			require.Equal(t, "2026-12-31", settings.TrackingStartedAt.UTC().Format(time.DateOnly))
			require.Equal(t, "manual", settings.ApprovalMode)
			if existing {
				require.Equal(t, "原作者", settings.UserName)
				require.Equal(t, "provider/model", settings.SelectedModelKey)
			}
			restarted := NewStore(db, s.logger)
			restarted.now = func() time.Time { return startedAt.AddDate(1, 0, 0) }
			restarted.InitTracking(context.Background())
			settings.UserName = "修改后的作者"
			require.NoError(t, config.SaveSettings(db, settings))
			saved, err := config.LoadSettings(db)
			require.NoError(t, err)
			require.NotNil(t, saved.TrackingStartedAt)
			require.True(t, startedAt.Equal(*saved.TrackingStartedAt))
			require.Equal(t, "2027-01-01", saved.TrackingStartDate)
			require.Equal(t, "修改后的作者", saved.UserName)
		})
	}
}

func TestAddActivityAccumulatesAndSeparatesLocalDates(t *testing.T) {
	s, db := newTestStore(t)
	ctx := context.Background()
	s.AddActivity(ctx, ActivityDelta{})
	var count int64
	require.NoError(t, db.Model(&DailyActivity{}).Count(&count).Error)
	require.Zero(t, count)
	s.AddActivity(ctx, ActivityDelta{
		NovelsCreated: 1, ChaptersCreated: 2, CharactersCreated: 3, LocationsCreated: 4,
		CreativeSeconds: 50, ConversationTurns: 6, ToolCalls: 7,
	})
	s.AddActivity(ctx, ActivityDelta{ChaptersCreated: 1, CreativeSeconds: 10, ToolCalls: 2})
	s.AddActivity(ctx, ActivityDelta{NovelsCreated: -1, ChaptersCreated: 100})
	var first DailyActivity
	require.NoError(t, db.First(&first, "date = ?", "2027-01-01").Error)
	require.Equal(t, ActivityDelta{
		NovelsCreated: 1, ChaptersCreated: 3, CharactersCreated: 3, LocationsCreated: 4,
		CreativeSeconds: 60, ConversationTurns: 6, ToolCalls: 9,
	}, first.ActivityDelta)
	tomorrow := s.now().AddDate(0, 0, 1)
	s.now = func() time.Time { return tomorrow }
	s.AddActivity(ctx, ActivityDelta{NovelsCreated: 1})
	var next DailyActivity
	require.NoError(t, db.First(&next, "date = ?", "2027-01-02").Error)
	require.Equal(t, ActivityDelta{NovelsCreated: 1}, next.ActivityDelta)
	require.NoError(t, db.Model(&DailyActivity{}).Count(&count).Error)
	require.EqualValues(t, 2, count)
}

func TestAddLLMUsageAccumulatesAllDimensions(t *testing.T) {
	s, db := newTestStore(t)
	ctx := context.Background()
	usage := TokenUsage{
		InputTokens: 3_000_000_000, OutputTokens: 500, TotalTokens: 3_000_000_500,
		CacheHitTokens: 100, CacheMissTokens: 200, ReasoningTokens: 50,
	}
	s.AddLLMUsage(ctx, "provider", "model", "chat", usage)
	s.AddLLMUsage(ctx, "provider", "model", "chat", usage)
	s.AddLLMUsage(ctx, "provider", "model", "chat", TokenUsage{})
	s.AddLLMUsage(ctx, "provider", "model", "chat", TokenUsage{InputTokens: -1})
	var combined DailyLLMUsage
	require.NoError(t, db.First(&combined).Error)
	require.Equal(t, "2027-01-01", combined.Date)
	require.EqualValues(t, 3, combined.UsageCount)
	require.Equal(t, TokenUsage{
		InputTokens: 6_000_000_000, OutputTokens: 1000, TotalTokens: 6_000_001_000,
		CacheHitTokens: 200, CacheMissTokens: 400, ReasoningTokens: 100,
	}, combined.TokenUsage)
	for _, dimension := range [][3]string{
		{"another-provider", "model", "chat"},
		{"provider", "another-model", "chat"},
		{"provider", "model", "style"},
	} {
		s.AddLLMUsage(ctx, dimension[0], dimension[1], dimension[2], usage)
	}
	tomorrow := s.now().AddDate(0, 0, 1)
	s.now = func() time.Time { return tomorrow }
	s.AddLLMUsage(ctx, "provider", "model", "chat", usage)
	var records []DailyLLMUsage
	require.NoError(t, db.Find(&records).Error)
	require.Len(t, records, 5)
	for _, record := range records {
		if record.Date == combined.Date && record.Provider == combined.Provider && record.Model == combined.Model && record.Purpose == combined.Purpose {
			continue
		}
		require.EqualValues(t, 1, record.UsageCount)
		require.Equal(t, usage, record.TokenUsage)
	}
}

func TestDailyCountersAccumulateConcurrently(t *testing.T) {
	s, db := newTestStore(t)
	var wg sync.WaitGroup
	const writes = 40
	for range writes {
		wg.Go(func() {
			s.AddActivity(context.Background(), ActivityDelta{ToolCalls: 1, CreativeSeconds: 10})
			s.AddLLMUsage(context.Background(), "provider", "model", "chat", TokenUsage{InputTokens: 20, TotalTokens: 20})
		})
	}
	wg.Wait()
	var activity DailyActivity
	require.NoError(t, db.First(&activity).Error)
	require.EqualValues(t, writes, activity.ToolCalls)
	require.EqualValues(t, writes*10, activity.CreativeSeconds)
	var usage DailyLLMUsage
	require.NoError(t, db.First(&usage).Error)
	require.EqualValues(t, writes, usage.UsageCount)
	require.EqualValues(t, writes*20, usage.InputTokens)
	require.EqualValues(t, writes*20, usage.TotalTokens)
}

func TestStatisticsFailuresOnlyWarn(t *testing.T) {
	s, db := newTestStore(t)
	var logs bytes.Buffer
	s.logger = slog.New(slog.NewTextHandler(&logs, nil))
	ctx := context.Background()
	for _, table := range []string{"app_config", "activity_daily", "llm_usage_daily"} {
		require.NoError(t, db.Exec("CREATE TRIGGER reject_"+table+" BEFORE INSERT ON "+table+" BEGIN SELECT RAISE(FAIL, 'statistics unavailable'); END").Error)
	}
	s.InitTracking(ctx)
	s.AddActivity(ctx, ActivityDelta{ChaptersCreated: 1})
	s.AddLLMUsage(ctx, "provider", "model", "chat", TokenUsage{TotalTokens: 10})
	require.Contains(t, logs.String(), "初始化创作统计起点失败")
	require.Contains(t, logs.String(), "记录每日创作统计失败")
	require.Contains(t, logs.String(), "记录每日 Token 用量失败")
	for _, model := range []any{&config.AppSettings{}, &DailyActivity{}, &DailyLLMUsage{}} {
		var count int64
		require.NoError(t, db.Model(model).Count(&count).Error)
		require.Zero(t, count)
	}
	require.NoError(t, db.Exec("DROP TRIGGER reject_app_config").Error)
	settings, err := config.LoadSettings(db)
	require.NoError(t, err)
	settings.UserName = "业务设置仍可保存"
	require.NoError(t, config.SaveSettings(db, settings))
}

func TestDailyStatisticsAreExcludedFromOperationLog(t *testing.T) {
	s, db := newTestStore(t)
	require.NoError(t, storage.RegisterOplogHooks(db))
	ctx := storage.WithTurn(context.Background(), "session", 1)
	s.InitTracking(ctx)
	for range 2 {
		s.AddActivity(ctx, ActivityDelta{CharactersCreated: 1})
		s.AddLLMUsage(ctx, "provider", "model", "chat", TokenUsage{TotalTokens: 10})
	}
	var recorded DailyActivity
	require.NoError(t, db.First(&recorded).Error)
	require.EqualValues(t, 2, recorded.CharactersCreated)
	var usage DailyLLMUsage
	require.NoError(t, db.First(&usage).Error)
	require.EqualValues(t, 2, usage.UsageCount)
	var count int64
	require.NoError(t, db.Model(&storage.OperationLogRecord{}).Count(&count).Error)
	require.Zero(t, count)
}
