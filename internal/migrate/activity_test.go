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
	"github.com/sigpanic/goink/internal/llm/adapter"
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
	selection := adapter.ModelSelection{AdapterName: adapter.ProtocolOpenAIChat, ProviderName: "provider", ModelID: "model"}
	require.NoError(t, db.Create(&config.AppSettings{ID: 1, UserName: "已有作者", DefaultModelSelection: selection}).Error)
	require.NoError(t, db.Migrator().DropTable(&activity.DailyActivity{}, &activity.DailyLLMUsage{}))
	require.NoError(t, db.Migrator().DropColumn(&config.AppSettings{}, "tracking_started_at"))
	require.NoError(t, db.Migrator().DropColumn(&config.AppSettings{}, "tracking_start_date"))
	require.NoError(t, migrate.Run(db, slog.Default()))
	assertActivitySchema(t, db)
	settings, err := config.LoadSettings(db)
	require.NoError(t, err)
	require.Equal(t, "已有作者", settings.UserName)
	require.Equal(t, selection, settings.DefaultModelSelection)
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

func TestActivityEntityColumnsUpgradePreservesStatistics(t *testing.T) {
	setupMigrationIntegrationEnv(t)
	db := openMigrationIntegrationDB(t)
	require.NoError(t, migrate.Run(db, slog.Default()))
	ctx := context.Background()
	store := activity.NewStore(db, slog.Default())
	store.InitTracking(ctx)
	previous := activity.ActivityDelta{
		NovelsCreated: 2, ChaptersCreated: 3, CharactersCreated: 4, LocationsCreated: 5,
		CreativeSeconds: 600, ConversationTurns: 7, ToolCalls: 8,
	}
	store.AddActivity(ctx, previous)
	settings, err := config.LoadSettings(db)
	require.NoError(t, err)
	date := settings.TrackingStartDate
	columns := []string{
		"volumes_created", "character_groups_created", "character_relations_created",
		"location_relations_created", "story_arcs_created", "arc_nodes_created",
		"timeline_entries_created", "reader_perspectives_created", "settings_created",
		"preferences_created", "style_samples_created", "skills_created", "images_imported",
		"sessions_created",
	}
	for _, column := range columns {
		require.NoError(t, db.Migrator().DropColumn(&activity.DailyActivity{}, column))
	}
	for range 2 {
		require.NoError(t, migrate.Run(db, slog.Default()))
	}
	store.InitTracking(ctx)
	reloaded, err := config.LoadSettings(db)
	require.NoError(t, err)
	require.Equal(t, settings, reloaded)
	var persisted map[string]any
	require.NoError(t, db.Table("activity_daily").Where("date = ?", date).Take(&persisted).Error)
	for _, column := range columns {
		require.EqualValues(t, 0, persisted[column], column)
	}
	var recorded activity.DailyActivity
	require.NoError(t, db.First(&recorded, "date = ?", date).Error)
	require.Equal(t, previous, recorded.ActivityDelta)
	store.AddActivity(ctx, activity.ActivityDelta{StoryArcsCreated: 4, SkillsCreated: 3, ImagesImported: 5, SessionsCreated: 2})
	require.NoError(t, db.First(&recorded, "date = ?", date).Error)
	previous.StoryArcsCreated = 4
	previous.SkillsCreated = 3
	previous.ImagesImported = 5
	previous.SessionsCreated = 2
	require.Equal(t, previous, recorded.ActivityDelta)
}
