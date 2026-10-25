package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sigpanic/goink/internal/config"
	"github.com/sigpanic/goink/internal/llm"
	"github.com/sigpanic/goink/internal/llm/adapter"
	"github.com/sigpanic/goink/internal/session"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetSettings(t *testing.T) {
	app := setupTestApp(t)

	settings, err := app.GetSettings()
	require.NoError(t, err)
	require.NotNil(t, settings)
	assert.Equal(t, uint(1), settings.ID)
	assert.Equal(t, "manual", settings.ApprovalMode)
}

func TestSetSelectedModel(t *testing.T) {
	app := setupTestApp(t)

	selection := llm.ModelSelection{AdapterName: adapter.ProtocolOpenAIChat, ProviderName: "relay", ModelID: "shared-model", ReasoningEffort: "high"}
	err := app.SetSelectedModel(SetSelectedModelInput{ModelSelection: selection})
	require.NoError(t, err)

	settings, err := app.GetSettings()
	require.NoError(t, err)
	assert.Equal(t, selection, settings.DefaultModelSelection)
	assert.Equal(t, "high", settings.DefaultModelSelection.ReasoningEffort)
	for _, id := range []string{"a", "b"} {
		require.NoError(t, app.db.Create(&session.Session{
			SessionID: id, ModelSelection: selection,
		}).Error)
	}
	sessionSelection := selection
	sessionSelection.AdapterName = adapter.ProtocolAnthropicMessages
	sessionSelection.ReasoningEffort = ""
	require.NoError(t, app.SetSelectedModel(SetSelectedModelInput{SessionID: "a", ModelSelection: sessionSelection}))
	detail, err := app.GetSession("a")
	require.NoError(t, err)
	assert.Equal(t, sessionSelection, detail.ModelSelection)
	assert.Empty(t, detail.ModelSelection.ReasoningEffort)
	other, err := app.GetSession("b")
	require.NoError(t, err)
	assert.Equal(t, selection, other.ModelSelection)
	assert.Equal(t, "high", other.ModelSelection.ReasoningEffort)
	require.Error(t, app.SetSelectedModel(SetSelectedModelInput{SessionID: "missing", ModelSelection: sessionSelection}))
	require.Error(t, app.SetSelectedModel(SetSelectedModelInput{SessionID: "a"}))
	after, err := app.GetSettings()
	require.NoError(t, err)
	assert.Equal(t, settings, after)
	persisted, err := config.LoadSettings(app.db)
	require.NoError(t, err)
	assert.Equal(t, settings, persisted)
	reloaded, err := app.GetSession("a")
	require.NoError(t, err)
	assert.Equal(t, detail.ModelSelection, reloaded.ModelSelection)
	assert.Empty(t, reloaded.ModelSelection.ReasoningEffort)
}

func TestSetSelectedModelSaveFailurePreservesSettings(t *testing.T) {
	app := setupTestApp(t)
	selection := llm.ModelSelection{AdapterName: adapter.ProtocolOpenAIChat, ProviderName: "relay", ModelID: "original-model", ReasoningEffort: "low"}
	require.NoError(t, app.SetSelectedModel(SetSelectedModelInput{ModelSelection: selection}))
	before, err := app.GetSettings()
	require.NoError(t, err)

	require.NoError(t, app.db.Exec(`CREATE TRIGGER reject_model_settings
		BEFORE UPDATE ON app_config
		WHEN NEW.default_model_id = 'rejected-model'
		BEGIN SELECT RAISE(FAIL, 'settings save failed'); END`).Error)

	selection.ModelID = "rejected-model"
	selection.ReasoningEffort = "high"
	err = app.SetSelectedModel(SetSelectedModelInput{ModelSelection: selection})
	require.ErrorContains(t, err, "settings save failed")
	settings, err := app.GetSettings()
	require.NoError(t, err)
	persisted, err := config.LoadSettings(app.db)
	require.NoError(t, err)
	assert.Equal(t, before, settings)
	assert.Equal(t, before, persisted)

	require.NoError(t, app.db.Exec("DROP TRIGGER reject_model_settings").Error)
	require.NoError(t, app.SetLastSession("session-after-failed-save"))
	settings, err = app.GetSettings()
	require.NoError(t, err)
	persisted, err = config.LoadSettings(app.db)
	require.NoError(t, err)
	expected := *before
	expected.LastSessionID = "session-after-failed-save"
	assert.Equal(t, &expected, settings)
	assert.Equal(t, &expected, persisted)
}

func TestSetSelectedModelClearsDefaultReasoningEffort(t *testing.T) {
	app := setupTestApp(t)

	selection := llm.ModelSelection{AdapterName: adapter.ProtocolOpenAIChat, ProviderName: "relay", ModelID: "shared-model", ReasoningEffort: "medium"}
	require.NoError(t, app.SetSelectedModel(SetSelectedModelInput{ModelSelection: selection}))
	selection.ReasoningEffort = ""
	require.NoError(t, app.SetSelectedModel(SetSelectedModelInput{ModelSelection: selection}))

	settings, err := app.GetSettings()
	require.NoError(t, err)
	assert.Equal(t, selection, settings.DefaultModelSelection)
	assert.Empty(t, settings.DefaultModelSelection.ReasoningEffort)
}

func TestSetLastSession(t *testing.T) {
	app := setupTestApp(t)

	err := app.SetLastSession("session-abc-123")
	require.NoError(t, err)

	settings, err := app.GetSettings()
	require.NoError(t, err)
	assert.Equal(t, "session-abc-123", settings.LastSessionID)
}

func TestSaveUserName(t *testing.T) {
	app := setupTestApp(t)

	err := app.SaveUserName("TestUser")
	require.NoError(t, err)

	settings, err := app.GetSettings()
	require.NoError(t, err)
	assert.Equal(t, "TestUser", settings.UserName)
}

func TestSaveGitConfig(t *testing.T) {
	app := setupTestApp(t)
	// Create a novel so SaveGitConfig has a repo to sync config to
	novel := createTestNovel(t, app)
	_ = novel

	err := app.SaveGitConfig("Author Name", "author@example.com")
	require.NoError(t, err)

	settings, err := app.GetSettings()
	require.NoError(t, err)
	assert.Equal(t, "Author Name", settings.GitName)
	assert.Equal(t, "author@example.com", settings.GitEmail)
}

func TestSaveAvatar(t *testing.T) {
	app := setupTestApp(t)

	avatarData := []byte("fake-jpeg-data")
	err := app.SaveAvatar(avatarData)
	require.NoError(t, err)

	avatarPath := filepath.Join(config.DataDirPath(), "user", "avatar.jpg")
	data, err := os.ReadFile(avatarPath)
	require.NoError(t, err)
	assert.Equal(t, avatarData, data)
}
