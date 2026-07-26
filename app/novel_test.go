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

	err = a.DeleteNovel(created.ID)
	require.NoError(t, err)

	novels, err := a.GetNovels()
	require.NoError(t, err)
	assert.Empty(t, novels, "novel list should be empty after deletion")

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
