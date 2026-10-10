//go:build cgo

package migrate_test

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/activity"
	"github.com/sigpanic/goink/internal/config"
	"github.com/sigpanic/goink/internal/migrate"
)

func assertActivitySchema(t *testing.T, db *gorm.DB) {
	t.Helper()
	for _, model := range []any{&activity.DailyActivity{}, &activity.DailyLLMUsage{}} {
		require.True(t, db.Migrator().HasTable(model))
	}
	for _, column := range []string{"tracking_started_at", "tracking_start_date"} {
		require.True(t, db.Migrator().HasColumn(&config.AppSettings{}, column))
	}
}

func TestActivityMigrationPreservesExistingSettingsAndStatistics(t *testing.T) {
	setupMigrationIntegrationEnv(t)
	db := openMigrationIntegrationDB(t)
	require.NoError(t, migrate.Run(db, slog.Default()))
	require.NoError(t, db.Create(&config.AppSettings{ID: 1, UserName: "已有作者", SelectedModelKey: "provider/model"}).Error)
	require.NoError(t, db.Migrator().DropTable(&activity.DailyActivity{}, &activity.DailyLLMUsage{}))
	require.NoError(t, db.Migrator().DropColumn(&config.AppSettings{}, "tracking_started_at"))
	require.NoError(t, db.Migrator().DropColumn(&config.AppSettings{}, "tracking_start_date"))
	require.NoError(t, migrate.Run(db, slog.Default()))
	assertActivitySchema(t, db)
	settings, err := config.LoadSettings(db)
	require.NoError(t, err)
	require.Equal(t, "已有作者", settings.UserName)
	require.Equal(t, "provider/model", settings.SelectedModelKey)
	require.Nil(t, settings.TrackingStartedAt)
	require.Empty(t, settings.TrackingStartDate)
	store := activity.NewStore(db, slog.Default())
	store.InitTracking(context.Background())
	store.AddActivity(context.Background(), activity.ActivityDelta{NovelsCreated: 1})
	store.AddLLMUsage(context.Background(), "provider", "model", "chat", activity.TokenUsage{TotalTokens: 123})
	settings, err = config.LoadSettings(db)
	require.NoError(t, err)
	require.NotNil(t, settings.TrackingStartedAt)
	for range 2 {
		require.NoError(t, migrate.Run(db, slog.Default()))
	}
	activity.NewStore(db, slog.Default()).InitTracking(context.Background())
	reloaded, err := config.LoadSettings(db)
	require.NoError(t, err)
	require.Equal(t, settings, reloaded)
	var daily activity.DailyActivity
	require.NoError(t, db.First(&daily).Error)
	require.EqualValues(t, 1, daily.NovelsCreated)
	var usage activity.DailyLLMUsage
	require.NoError(t, db.First(&usage).Error)
	require.EqualValues(t, 1, usage.UsageCount)
	require.EqualValues(t, 123, usage.TotalTokens)
}
