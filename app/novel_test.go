package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sigpanic/goink/internal/config"
)

func TestGetNovels_Empty(t *testing.T) {
	a := setupTestApp(t)

	novels, err := a.GetNovels()
	require.NoError(t, err)
	require.NotNil(t, novels, "GetNovels should return a non-nil slice even when empty")
	assert.Empty(t, novels, "no novels should exist yet")
}

func TestCreateNovel(t *testing.T) {
	a := setupTestApp(t)

	n, err := a.CreateNovel(CreateNovelInput{
		Title:       "My Novel",
		Description: "A test novel",
		Genre:       "sci-fi",
	})
	require.NoError(t, err)
	require.NotNil(t, n)
	assert.Greater(t, n.ID, int64(0))
	assert.Equal(t, "My Novel", n.Title)
	assert.Equal(t, "sci-fi", n.Genre)
	assert.Equal(t, "A test novel", n.Description)

	// Verify git repo was initialized (check .git dir exists).
	novelDir := config.NovelDirPath(n.ID)
	gitDir := filepath.Join(novelDir, ".git")
	info, err := os.Stat(gitDir)
	require.NoError(t, err, ".git directory should exist for new novel")
	assert.True(t, info.IsDir(), ".git should be a directory")
}

func TestCreateNovel_GitMissing(t *testing.T) {
	a := setupTestApp(t)

	// Override PATH so that git cannot be found.
	t.Setenv("PATH", "/nonexistent")

	_, err := a.CreateNovel(CreateNovelInput{
		Title: "Gitless Novel",
		Genre: "fantasy",
	})
	require.Error(t, err, "CreateNovel should fail when git is not available")
	assert.Contains(t, err.Error(), "git")
}

func TestGetNovels_AfterCreate(t *testing.T) {
	a := setupTestApp(t)

	created, err := a.CreateNovel(CreateNovelInput{
		Title: "Listable Novel",
		Genre: "romance",
	})
	require.NoError(t, err)

	novels, err := a.GetNovels()
	require.NoError(t, err)
	require.Len(t, novels, 1)
	assert.Equal(t, created.ID, novels[0].ID)
	assert.Equal(t, "Listable Novel", novels[0].Title)
}

func TestUpdateNovel(t *testing.T) {
	a := setupTestApp(t)

	created, err := a.CreateNovel(CreateNovelInput{
		Title:       "Original Title",
		Description: "Original desc",
		Genre:       "horror",
	})
	require.NoError(t, err)

	updated, err := a.UpdateNovel(created.ID, UpdateNovelInput{
		Title:       "Updated Title",
		Description: "Updated description",
		Genre:       "thriller",
	})
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, created.ID, updated.ID)
	assert.Equal(t, "Updated Title", updated.Title)
	assert.Equal(t, "Updated description", updated.Description)
	assert.Equal(t, "thriller", updated.Genre)
}

func TestDeleteNovel(t *testing.T) {
	a := setupTestApp(t)

	created, err := a.CreateNovel(CreateNovelInput{
		Title: "To Be Deleted",
		Genre: "mystery",
	})
	require.NoError(t, err)

	// 给 Novel 加专属偏好 + 设定（级联删除应清掉）
	_, err = a.CreatePreference(created.ID, CreatePreferenceInput{
		IsGlobal: false, Category: "style", Content: "short sentences",
	})
	require.NoError(t, err)
	_, err = a.CreateNovelSetting(created.ID, CreateNovelSettingInput{
		Category: "worldview", Content: "magic system",
	})
	require.NoError(t, err)
	// 同时加全局偏好（删除 Novel 后应保留）；v2 取消全局设定概念，不再创建 global setting
	_, err = a.CreatePreference(created.ID, CreatePreferenceInput{
		IsGlobal: true, Category: "global-style", Content: "concise",
	})
	require.NoError(t, err)

	err = a.DeleteNovel(created.ID)
	require.NoError(t, err)

	novels, err := a.GetNovels()
	require.NoError(t, err)
	assert.Empty(t, novels, "novel list should be empty after deletion")

	// 全局 preference 保留（删 Novel 不影响全局数据）；小说专属偏好已级联删除
	prefs, err := a.GetPreferences(created.ID)
	require.NoError(t, err)
	assert.Empty(t, prefs.Novel, "novel-specific preference should be cascade-deleted")
	assert.Len(t, prefs.Global, 1, "global preference should survive novel deletion")

	// v2 设定全部归属小说，删 Novel 后应全部级联删除（无全局保留概念）
	settings, err := a.GetNovelSettings(created.ID)
	require.NoError(t, err)
	assert.Empty(t, settings.Items, "all settings should be cascade-deleted with novel")

	// On-disk novel directory cleanup is best-effort (Windows git lock files).
	// Just verify the DB record is gone — filesystem cleanup is platform-specific.
	_ = config.NovelDirPath(created.ID)
}

func TestSetActiveNovel(t *testing.T) {
	a := setupTestApp(t)

	created, err := a.CreateNovel(CreateNovelInput{
		Title: "Active Novel",
		Genre: "comedy",
	})
	require.NoError(t, err)

	err = a.SetActiveNovel(SetActiveNovelInput{NovelID: created.ID})
	require.NoError(t, err)

	assert.Equal(t, created.ID, a.settings.LastNovelID)

	// Re-load settings from DB to confirm persistence.
	reloaded, err := config.LoadSettings(a.db)
	require.NoError(t, err)
	assert.Equal(t, created.ID, reloaded.LastNovelID)
}
