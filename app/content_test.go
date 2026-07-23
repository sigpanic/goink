package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sigpanic/goink/internal/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetContent_NonExistent(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	novelID := novel.ID

	content, err := app.GetContent(novelID, "chapters/999.md")
	require.NoError(t, err)
	assert.Equal(t, "", content)
}

func TestSaveAndGetContent(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	novelID := novel.ID

	err := app.SaveContent(SaveContentInput{
		NovelID: novelID,
		Path:    "goink.md",
		Content: "Hello, world!",
	})
	require.NoError(t, err)

	content, err := app.GetContent(novelID, "goink.md")
	require.NoError(t, err)
	assert.Equal(t, "Hello, world!", content)
}

func TestSaveContent_ChapterPath(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	novelID := novel.ID

	// Create chapter in DB first (SaveContent updates word_count for chapter paths)
	_, err := app.CreateChapter(CreateChapterInput{NovelID: novelID, Title: "Test Chapter"})
	require.NoError(t, err)

	chapterContent := "This is chapter one content with some words."
	err = app.SaveContent(SaveContentInput{
		NovelID: novelID,
		Path:    "chapters/001.md",
		Content: chapterContent,
	})
	require.NoError(t, err)

	// Verify file exists on disk
	novelDir := config.NovelDirPath(novelID)
	filePath := filepath.Join(novelDir, "chapters", "001.md")
	data, err := os.ReadFile(filePath)
	require.NoError(t, err)
	assert.Equal(t, chapterContent, string(data))

	// Verify word_count was updated in DB
	chapters, err := app.GetChapters(novelID)
	require.NoError(t, err)
	require.Len(t, chapters, 1)
	assert.Greater(t, chapters[0].WordCount, 0)
}
