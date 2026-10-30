package v180

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/config"
	"github.com/sigpanic/goink/internal/migrate/engine"
	"github.com/sigpanic/goink/internal/migrate/v161"
)

func TestModelSelectionMigration(t *testing.T) {
	for _, tt := range []struct {
		name, key, effort, plain string
		current, want            FrozenLlmAdapterModelSelection
	}{
		{
			name: "builtin without config", key: "mimo/mimo-v2.6-flash", effort: "high",
			want: FrozenLlmAdapterModelSelection{"openai_chat", "MiMo", "mimo-v2.6-flash", "high"},
		},
		{
			name: "slashes in provider and model", key: "my/relay/vendor/model", effort: "",
			plain: `{"providers":[{"name":"my/relay","chat_url":"https://relay.example/chat/completions","models":[]}]}`,
			want:  FrozenLlmAdapterModelSelection{"openai_chat", "my/relay", "vendor/model", ""},
		},
		{
			name: "custom name collision", key: "DeepSeek/vendor/model", effort: "low",
			plain: `{"providers":[{"name":"DeepSeek","chat_url":"https://relay.example/chat/completions","models":[]}]}`,
			want:  FrozenLlmAdapterModelSelection{"openai_chat", "DeepSeek (custom)", "vendor/model", "low"},
		},
		{
			name: "already converted config", key: "my/relay/vendor/model", effort: "high",
			plain: `{"version":2,"adapters":{"openai_chat":{"providers":[{"name":"my/relay","provider_type":"openai"}]}}}`,
			want:  FrozenLlmAdapterModelSelection{"openai_chat", "my/relay", "vendor/model", "high"},
		},
		{
			name: "unresolvable old key", key: "fixture-model", effort: "high",
		},
		{
			name: "ambiguous key", key: "my/relay/model", effort: "high",
			plain: `{"providers":[{"name":"my","chat_url":"https://a.example/chat/completions"},{"name":"my/relay","chat_url":"https://b.example/chat/completions"}]}`,
		},
		{
			name: "new choice with default effort wins", key: "mimo/mimo-v2.6-flash", effort: "high",
			current: FrozenLlmAdapterModelSelection{"gemini_interactions", "Gemini", "new-model", ""},
			want:    FrozenLlmAdapterModelSelection{"gemini_interactions", "Gemini", "new-model", ""},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db := legacySelectionDB(t)
			if tt.plain != "" {
				writeLegacyConfig(t, []byte(tt.plain))
			}
			require.NoError(t, db.Create(&v161.FrozenConfigAppSettings{
				ID: 1, SelectedModelKey: tt.key, ReasoningEffort: tt.effort, UserName: "keep-user",
			}).Error)
			require.NoError(t, (engine.AutoMigrateStep{Models: FrozenModels}).Run(db, slog.Default()))
			if tt.current.AdapterName != "" {
				require.NoError(t, db.Table("app_config").Where("id = 1").Updates(map[string]any{
					"default_adapter_name": tt.current.AdapterName, "default_provider_name": tt.current.ProviderName,
					"default_model_id": tt.current.ModelID, "default_reasoning_effort": tt.current.ReasoningEffort,
				}).Error)
			}
			require.NoError(t, (modelSelectionStep{}).Run(db, slog.Default()))
			var settings FrozenConfigAppSettings
			require.NoError(t, db.First(&settings, 1).Error)
			require.Equal(t, tt.want, settings.DefaultModelSelection)
			require.Equal(t, "keep-user", settings.UserName)
			for _, column := range []string{"selected_model_key", "reasoning_effort"} {
				require.False(t, db.Migrator().HasColumn("app_config", column))
			}
			needed, err := needsModelSelectionMigration(db)
			require.NoError(t, err)
			require.False(t, needed)
		})
	}
}

func TestSessionSelectionResetAndMigrationRecovery(t *testing.T) {
	db := legacySelectionDB(t)
	require.NoError(t, db.Create(&v161.FrozenConfigAppSettings{
		ID: 1, SelectedModelKey: "deepseek/deepseek-v4-pro", ReasoningEffort: "high",
	}).Error)
	for _, id := range []string{"old", "new"} {
		require.NoError(t, db.Create(&v161.FrozenSessionSession{
			SessionID: id, Model: "same-model", ReasoningEffort: "high", Title: "keep-title", Usage: `{"input_tokens":123}`,
		}).Error)
	}
	require.NoError(t, db.AutoMigrate(&engine.MigrateState{}))
	steps := []engine.Step{engine.AutoMigrateStep{Models: FrozenModels}, modelSelectionStep{}}
	require.NoError(t, engine.RunSteps(db, slog.Default(), modelSelectionMigration, steps[:1]))
	newSelection := FrozenLlmAdapterModelSelection{"anthropic_messages", "Claude", "new-model", ""}
	require.NoError(t, db.Table("sessions").Where("session_id = ?", "new").Updates(map[string]any{
		"adapter_name": newSelection.AdapterName, "provider_name": newSelection.ProviderName,
		"model_id": newSelection.ModelID, "reasoning_effort": "",
	}).Error)
	require.NoError(t, db.Exec(`CREATE TRIGGER block_selection_reset BEFORE UPDATE ON sessions
		BEGIN SELECT RAISE(ABORT, 'reset blocked'); END`).Error)
	require.ErrorContains(t, engine.RunSteps(db, slog.Default(), modelSelectionMigration, steps), "reset blocked")
	var settings FrozenConfigAppSettings
	require.NoError(t, db.First(&settings, 1).Error)
	require.Empty(t, settings.DefaultModelSelection, "Session 更新失败必须回滚先前的全局回填")
	require.True(t, db.Migrator().HasColumn("sessions", "model"))
	require.True(t, db.Migrator().HasColumn("app_config", "selected_model_key"))
	require.NoError(t, db.Exec("DROP TRIGGER block_selection_reset").Error)
	require.NoError(t, engine.RunSteps(db, slog.Default(), modelSelectionMigration, steps))
	require.NoError(t, db.First(&settings, 1).Error)
	require.Equal(t, FrozenLlmAdapterModelSelection{"openai_chat", "DeepSeek", "deepseek-v4-pro", "high"}, settings.DefaultModelSelection)
	for _, id := range []string{"old", "new"} {
		var session FrozenSessionSession
		require.NoError(t, db.Where("session_id = ?", id).First(&session).Error)
		want := FrozenLlmAdapterModelSelection{}
		if id == "new" {
			want = newSelection
		}
		require.Equal(t, want, session.ModelSelection)
		require.Equal(t, "keep-title", session.Title)
		require.JSONEq(t, `{"input_tokens":123}`, session.Usage)
	}
	require.False(t, db.Migrator().HasColumn("sessions", "model"))
	// 完成后破坏新值；再次运行必须由 migrate_state 跳过，不能重新覆盖用户状态。
	require.NoError(t, db.Exec("UPDATE sessions SET reasoning_effort = 'keep-after-done'").Error)
	require.NoError(t, engine.RunSteps(db, slog.Default(), modelSelectionMigration, steps))
	var kept int64
	require.NoError(t, db.Table("sessions").Where("reasoning_effort = ?", "keep-after-done").Count(&kept).Error)
	require.EqualValues(t, 2, kept)
}

func legacySelectionDB(t *testing.T) *gorm.DB {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "legacy.db")), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&v161.FrozenConfigAppSettings{}, &v161.FrozenSessionSession{}))
	_, err = os.Stat(config.LLMConfigPath())
	require.ErrorIs(t, err, os.ErrNotExist)
	return db
}
