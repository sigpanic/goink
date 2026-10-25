package session

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/config"
	"github.com/sigpanic/goink/internal/llm/adapter"
)

func TestModelSelectionPersistenceKeepsScopesIndependent(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "selection.db")), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&Session{}, &config.AppSettings{}))

	initial := adapter.ModelSelection{
		AdapterName: adapter.ProtocolOpenAIChat, ProviderName: "shared/relay", ModelID: "org/shared-model",
	}
	settings, err := config.LoadSettings(db)
	require.NoError(t, err)
	require.Empty(t, settings.DefaultModelSelection)
	settings.DefaultModelSelection = initial
	settings.ReasoningEffort = "high"
	require.NoError(t, config.SaveSettings(db, settings))
	for _, id := range []string{"a", "b"} {
		require.NoError(t, db.Create(&Session{
			SessionID: id, NovelID: 1, Title: id, ModelSelection: initial, ReasoningEffort: "high",
			Summary: "saved summary", Usage: `{"total_tokens":10}`, ActiveVersion: 3, LastTurnID: 9,
		}).Error)
	}
	store := NewStore(db, discardLogger())
	ctx := context.Background()
	beforeA, err := store.GetSession(ctx, "a")
	require.NoError(t, err)
	beforeB, err := store.GetSession(ctx, "b")
	require.NoError(t, err)
	beforeGlobal, err := config.LoadSettings(db)
	require.NoError(t, err)
	require.Equal(t, initial, beforeGlobal.DefaultModelSelection)
	require.Equal(t, initial, beforeA.ModelSelection)

	selectionA := initial
	selectionA.AdapterName = adapter.ProtocolAnthropicMessages
	require.NoError(t, store.UpdateSessionModelSelection(ctx, "a", selectionA, ""))
	require.NoError(t, store.UpdateSessionUsage(ctx, "a", `{"total_tokens":20}`))
	require.NoError(t, store.UpdateSessionTitle(ctx, "a", "updated title"))
	afterA, err := store.GetSession(ctx, "a")
	require.NoError(t, err)
	wantA := *beforeA
	wantA.ModelSelection, wantA.ReasoningEffort, wantA.Title = selectionA, "", "updated title"
	wantA.Usage = `{"total_tokens":20}`
	wantA.UpdatedAt = afterA.UpdatedAt
	require.Equal(t, wantA, *afterA)
	afterB, err := store.GetSession(ctx, "b")
	require.NoError(t, err)
	require.Equal(t, beforeB, afterB)
	afterGlobal, err := config.LoadSettings(db)
	require.NoError(t, err)
	require.Equal(t, beforeGlobal, afterGlobal)

	defaultSelection := initial
	defaultSelection.AdapterName = adapter.ProtocolGeminiInteractions
	afterGlobal.DefaultModelSelection, afterGlobal.ReasoningEffort = defaultSelection, ""
	require.NoError(t, config.SaveSettings(db, afterGlobal))
	loadedGlobal, err := config.LoadSettings(db)
	require.NoError(t, err)
	require.Equal(t, defaultSelection, loadedGlobal.DefaultModelSelection)
	require.Empty(t, loadedGlobal.ReasoningEffort)
	loadedA, err := store.GetSession(ctx, "a")
	require.NoError(t, err)
	require.Equal(t, afterA, loadedA)
	loadedB, err := store.GetSession(ctx, "b")
	require.NoError(t, err)
	require.Equal(t, beforeB, loadedB)
	require.ErrorIs(t, store.UpdateSessionModelSelection(ctx, "missing", selectionA, ""), gorm.ErrRecordNotFound)
}
