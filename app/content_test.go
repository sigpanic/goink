package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sigpanic/goink/internal/config"
	"github.com/sigpanic/goink/internal/git"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetContent_NonExistent(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	ch := createTestChapter(t, app, novel.ID)

	content, err := app.GetContent(novel.ID, ch.OutlineFilePath)
	require.NoError(t, err)
	assert.Equal(t, "", content)
}

func TestVolumeOutlineContentPath(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	otherNovel := createTestNovel(t, app)
	name := "第一卷"
	v, err := app.PlaceVolume(PlaceVolumeInput{NovelID: novel.ID, Name: &name})
	require.NoError(t, err)
	path := v.OutlineFilePath

	content, err := app.GetContent(novel.ID, path)
	require.NoError(t, err)
	assert.Empty(t, content)
	require.NoError(t, app.SaveContent(SaveContentInput{
		NovelID: novel.ID,
		Path:    path,
		Content: "卷纲内容",
	}))
	content, err = app.GetContent(novel.ID, path)
	require.NoError(t, err)
	assert.Equal(t, "卷纲内容", content)

	invalidPaths := []string{
		fmt.Sprintf("volumes/%d.md", v.ID),
		fmt.Sprintf("volumes/id_0%d.md", v.ID),
		fmt.Sprintf("volumes/id_%d.txt", v.ID),
		"volumes/id_0.md",
		"volumes/id_999999.md",
		"./" + path,
		strings.Replace(path, "/", "\\", 1),
		"volumes/../goink.md",
	}
	for _, invalidPath := range invalidPaths {
		t.Run(invalidPath, func(t *testing.T) {
			_, err := app.GetContent(novel.ID, invalidPath)
			require.Error(t, err)
			err = app.SaveContent(SaveContentInput{
				NovelID: novel.ID,
				Path:    invalidPath,
				Content: "should not be written",
			})
			require.Error(t, err)
		})
	}
	content, err = app.GetContent(novel.ID, "goink.md")
	require.NoError(t, err)
	assert.NotEqual(t, "should not be written", content)

	_, err = app.GetContent(otherNovel.ID, path)
	require.Error(t, err)
	require.Error(t, app.SaveContent(SaveContentInput{
		NovelID: otherNovel.ID,
		Path:    path,
		Content: "foreign outline",
	}))
	foreignPath, err := git.ResolvePath(path, otherNovel.ID)
	require.NoError(t, err)
	_, err = os.Stat(foreignPath)
	require.ErrorIs(t, err, os.ErrNotExist)

	require.NoError(t, app.DeleteVolume(novel.ID, v.ID))
	_, err = app.GetContent(novel.ID, path)
	require.Error(t, err)
	require.Error(t, app.SaveContent(SaveContentInput{
		NovelID: novel.ID,
		Path:    path,
		Content: "orphan outline",
	}))
	deletedPath, err := git.ResolvePath(path, novel.ID)
	require.NoError(t, err)
	_, err = os.Stat(deletedPath)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestContentRejectsNonCanonicalChapterPaths(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	ch := createTestChapter(t, app, novel.ID)
	paths := []string{
		"chapters/001.md",
		"outlines/001.md",
		"outlines/NaN.md",
		"chapters/new.md",
		"outlines/new.md",
		"chapters/id_0.md",
		"chapters/id_999999.md",
		"chapters\\001.md",
		"./chapters/001.md",
		fmt.Sprintf("chapters/1/id_%d.md", ch.ID),
		fmt.Sprintf("outlines/1/id_%d.md", ch.ID),
		fmt.Sprintf("chapters/id_0%d.md", ch.ID),
	}
	for _, filePath := range paths {
		t.Run(filePath, func(t *testing.T) {
			_, err := app.GetContent(novel.ID, filePath)
			require.Error(t, err)
			err = app.SaveContent(SaveContentInput{
				NovelID: novel.ID,
				Path:    filePath,
				Content: "should not be written",
			})
			require.Error(t, err)
			fullPath, err := git.ResolvePath(filePath, novel.ID)
			require.NoError(t, err)
			_, err = os.Stat(fullPath)
			require.ErrorIs(t, err, os.ErrNotExist)
		})
	}
}

func TestContentRejectsChapterFromAnotherNovel(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	otherNovel := createTestNovel(t, app)
	otherChapter := createTestChapter(t, app, otherNovel.ID)

	for _, filePath := range []string{otherChapter.FilePath, otherChapter.OutlineFilePath} {
		_, err := app.GetContent(novel.ID, filePath)
		require.Error(t, err)
		err = app.SaveContent(SaveContentInput{
			NovelID: novel.ID,
			Path:    filePath,
			Content: "should not be written",
		})
		require.Error(t, err)
		fullPath, err := git.ResolvePath(filePath, novel.ID)
		require.NoError(t, err)
		_, err = os.Stat(fullPath)
		require.ErrorIs(t, err, os.ErrNotExist)
	}
}

func TestContentDoesNotReadOrOverwriteExistingLegacyChapter(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	legacyPath := filepath.Join(config.NovelDirPath(novel.ID), "chapters", "001.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(legacyPath), 0o755))
	require.NoError(t, os.WriteFile(legacyPath, []byte("legacy content"), 0o644))

	_, err := app.GetContent(novel.ID, "chapters/001.md")
	require.Error(t, err)
	err = app.SaveContent(SaveContentInput{
		NovelID: novel.ID,
		Path:    "chapters/001.md",
		Content: "replacement",
	})
	require.Error(t, err)
	data, err := os.ReadFile(legacyPath)
	require.NoError(t, err)
	assert.Equal(t, "legacy content", string(data))
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
	ch, err := app.CreateChapter(CreateChapterInput{NovelID: novelID, Title: "Test Chapter"})
	require.NoError(t, err)

	chapterContent := "This is chapter one content with some words."
	err = app.SaveContent(SaveContentInput{
		NovelID: novelID,
		Path:    ch.FilePath,
		Content: chapterContent,
	})
	require.NoError(t, err)

	// Verify file exists on disk
	novelDir := config.NovelDirPath(novelID)
	filePath := filepath.Join(novelDir, ch.FilePath)
	data, err := os.ReadFile(filePath)
	require.NoError(t, err)
	assert.Equal(t, chapterContent, string(data))

	// Verify word_count was updated in DB
	chapters, err := app.GetChapters(novelID)
	require.NoError(t, err)
	require.Len(t, chapters, 1)
	assert.Greater(t, chapters[0].WordCount, 0)

	err = app.SaveContent(SaveContentInput{
		NovelID: novelID,
		Path:    ch.OutlineFilePath,
		Content: "Chapter outline",
	})
	require.NoError(t, err)
	outline, err := app.GetContent(novelID, ch.OutlineFilePath)
	require.NoError(t, err)
	assert.Equal(t, "Chapter outline", outline)
}
