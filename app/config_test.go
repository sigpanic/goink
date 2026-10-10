package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sigpanic/goink/internal/config"

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

	err := app.SetSelectedModel("gpt-4o", "high")
	require.NoError(t, err)

	settings, err := app.GetSettings()
	require.NoError(t, err)
	assert.Equal(t, "gpt-4o", settings.SelectedModelKey)
	assert.Equal(t, "high", settings.ReasoningEffort)
}

func TestSetSelectedModelSaveFailurePreservesSettings(t *testing.T) {
	app := setupTestApp(t)
	require.NoError(t, app.SetSelectedModel("original-model", "low"))
	before, err := app.GetSettings()
	require.NoError(t, err)

	require.NoError(t, app.db.Exec(`CREATE TRIGGER reject_model_settings
		BEFORE UPDATE ON app_config
		WHEN NEW.selected_model_key = 'rejected-model'
		BEGIN SELECT RAISE(FAIL, 'settings save failed'); END`).Error)

	err = app.SetSelectedModel("rejected-model", "high")
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

func TestSetReasoningEffort(t *testing.T) {
	app := setupTestApp(t)

	err := app.SetReasoningEffort("medium")
	require.NoError(t, err)

	settings, err := app.GetSettings()
	require.NoError(t, err)
	assert.Equal(t, "medium", settings.ReasoningEffort)
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
