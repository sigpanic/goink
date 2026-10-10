package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sigpanic/goink/internal/config"
	"github.com/sigpanic/goink/internal/git"
	"github.com/sigpanic/goink/internal/writing"

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
		fmt.Sprintf("/volumes/id_%d.md", v.ID),
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
		"/chapters/001.md",
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

// 前导 / 会让 ResolvePath 落到本小说真实章节文件上；修复前 SaveContent 会绕过校验并覆盖它。
func TestContentRejectsLeadingSlashChapterPath(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	ch := createTestChapter(t, app, novel.ID)
	require.NoError(t, app.SaveContent(SaveContentInput{
		NovelID: novel.ID,
		Path:    ch.FilePath,
		Content: "original content",
	}))

	leadingSlash := "/" + ch.FilePath
	_, err := app.GetContent(novel.ID, leadingSlash)
	require.Error(t, err)
	err = app.SaveContent(SaveContentInput{
		NovelID: novel.ID,
		Path:    leadingSlash,
		Content: "should not be written",
	})
	require.Error(t, err)

	content, err := app.GetContent(novel.ID, ch.FilePath)
	require.NoError(t, err)
	assert.Equal(t, "original content", content)
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

func TestSaveContentRejectsStaleEditorVersion(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	path := "goink.md"
	initial, err := app.GetContent(novel.ID, path)
	require.NoError(t, err)
	require.NoError(t, git.WriteFile(novel.ID, path, "AI update"))

	err = app.SaveContent(SaveContentInput{
		NovelID:         novel.ID,
		Path:            path,
		Content:         "stale draft",
		ExpectedContent: &initial,
	})
	require.ErrorContains(t, err, "CONTENT_CONFLICT")
	content, err := app.GetContent(novel.ID, path)
	require.NoError(t, err)
	assert.Equal(t, "AI update", content)

	require.NoError(t, app.SaveContent(SaveContentInput{
		NovelID:         novel.ID,
		Path:            path,
		Content:         "reconciled draft",
		ExpectedContent: &content,
	}))
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

func TestSaveContentRecordsTextChanges(t *testing.T) {
	for _, withExpected := range []bool{true, false} {
		t.Run(fmt.Sprintf("expected=%t", withExpected), func(t *testing.T) {
			app := setupTestApp(t)
			novel := createTestNovel(t, app)
			ch := createTestChapter(t, app, novel.ID)
			previous := ""
			for _, content := range []string{"主角走进房间", "主角冲出房间", "主角房间", "主角房间"} {
				input := SaveContentInput{NovelID: novel.ID, Path: ch.FilePath, Content: content}
				if withExpected {
					input.ExpectedContent = &previous
				}
				require.NoError(t, app.SaveContent(input))
				previous = content
			}
			stale := "主角走进房间"
			require.ErrorContains(t, app.SaveContent(SaveContentInput{
				NovelID: novel.ID, Path: ch.FilePath, Content: "冲突内容", ExpectedContent: &stale,
			}), "CONTENT_CONFLICT")
			require.NoError(t, app.SaveContent(SaveContentInput{NovelID: novel.ID, Path: ch.OutlineFilePath, Content: "大纲"}))
			var logs []writing.WritingLog
			require.NoError(t, app.writing.DB.Order("id").Find(&logs).Error)
			expectedCount := 0
			if withExpected {
				expectedCount = 1
			}
			require.Len(t, logs, expectedCount)
			if withExpected {
				assert.Equal(t, novel.ID, logs[0].NovelID)
				assert.Equal(t, 8, logs[0].WordsAdded)
				assert.Equal(t, 4, logs[0].WordsDeleted)
				assert.Equal(t, 4, logs[0].WordDelta)
			}
			chapters, err := app.GetChapters(novel.ID)
			require.NoError(t, err)
			require.Len(t, chapters, 1)
			assert.Equal(t, 4, chapters[0].WordCount)
		})
	}
}

func TestSaveContentWritingLogFailureDoesNotBlockSave(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	ch := createTestChapter(t, app, novel.ID)
	require.NoError(t, app.writing.DB.Migrator().DropTable(&writing.WritingLog{}))
	previous := ""
	require.NoError(t, app.SaveContent(SaveContentInput{NovelID: novel.ID, Path: ch.FilePath, Content: "保存正文", ExpectedContent: &previous}))
	content, err := app.GetContent(novel.ID, ch.FilePath)
	require.NoError(t, err)
	assert.Equal(t, "保存正文", content)
	chapters, err := app.GetChapters(novel.ID)
	require.NoError(t, err)
	require.Len(t, chapters, 1)
	assert.Equal(t, 4, chapters[0].WordCount)
}
