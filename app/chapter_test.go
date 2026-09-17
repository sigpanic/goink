package app

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetChapters_Empty(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	novelID := novel.ID

	chapters, err := app.GetChapters(novelID)
	require.NoError(t, err)
	assert.Empty(t, chapters)
}

func TestCreateChapter(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	novelID := novel.ID

	ch, err := app.CreateChapter(CreateChapterInput{
		NovelID: novelID,
		Title:   "First Chapter",
	})
	require.NoError(t, err)
	assert.Equal(t, 1, ch.ChapterNumber)
	assert.Equal(t, "First Chapter", ch.Title)
	assert.Equal(t, novelID, ch.NovelID)
	assert.Equal(t, "chapters/id_1.md", ch.FilePath)
}

func TestCreateChapter_AutoIncrement(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	novelID := novel.ID

	ch1, err := app.CreateChapter(CreateChapterInput{
		NovelID: novelID,
		Title:   "Chapter One",
	})
	require.NoError(t, err)
	assert.Equal(t, 1, ch1.ChapterNumber)

	ch2, err := app.CreateChapter(CreateChapterInput{
		NovelID: novelID,
		Title:   "Chapter Two",
	})
	require.NoError(t, err)
	assert.Equal(t, 2, ch2.ChapterNumber)

	ch3, err := app.CreateChapter(CreateChapterInput{
		NovelID: novelID,
		Title:   "Chapter Three",
	})
	require.NoError(t, err)
	assert.Equal(t, 3, ch3.ChapterNumber)
}

func TestGetMaxChapterNumber(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	novelID := novel.ID

	_, err := app.CreateChapter(CreateChapterInput{NovelID: novelID, Title: "A"})
	require.NoError(t, err)
	_, err = app.CreateChapter(CreateChapterInput{NovelID: novelID, Title: "B"})
	require.NoError(t, err)
	_, err = app.CreateChapter(CreateChapterInput{NovelID: novelID, Title: "C"})
	require.NoError(t, err)

	max, err := app.GetMaxChapterNumber(novelID)
	require.NoError(t, err)
	assert.Equal(t, 3, max)
}

func TestUpdateChapterTitle(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	novelID := novel.ID

	_, err := app.CreateChapter(CreateChapterInput{
		NovelID: novelID,
		Title:   "Original Title",
	})
	require.NoError(t, err)

	err = app.UpdateChapterTitle(novelID, 1, "Updated Title")
	require.NoError(t, err)

	chapters, err := app.GetChapters(novelID)
	require.NoError(t, err)
	require.Len(t, chapters, 1)
	assert.Equal(t, "Updated Title", chapters[0].Title)
}

func TestGetChapters_AfterCreate(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	novelID := novel.ID

	_, err := app.CreateChapter(CreateChapterInput{NovelID: novelID, Title: "Alpha"})
	require.NoError(t, err)
	_, err = app.CreateChapter(CreateChapterInput{NovelID: novelID, Title: "Beta"})
	require.NoError(t, err)

	chapters, err := app.GetChapters(novelID)
	require.NoError(t, err)
	require.Len(t, chapters, 2)

	var titles []string
	for _, ch := range chapters {
		titles = append(titles, ch.Title)
	}
	assert.Contains(t, titles, "Alpha")
	assert.Contains(t, titles, "Beta")

	// Verify chapter files were created
	for _, ch := range chapters {
		assert.Equal(t, chapterPath(ch.ID), ch.FilePath)
	}
}

func chapterPath(id int64) string {
	return fmt.Sprintf("chapters/id_%d.md", id)
}
