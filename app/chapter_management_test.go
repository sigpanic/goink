package app

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/sigpanic/goink/internal/character"
	"github.com/sigpanic/goink/internal/git"
	"github.com/sigpanic/goink/internal/reader"
	"github.com/sigpanic/goink/internal/storyarc"
	"github.com/sigpanic/goink/internal/timeline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChapterStructureOperations(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	volumeName := "第一卷"
	volume, err := app.PlaceVolume(PlaceVolumeInput{NovelID: novel.ID, Name: &volumeName})
	require.NoError(t, err)

	firstTitle := "第一章"
	first, err := app.PlaceChapter(PlaceChapterInput{NovelID: novel.ID, Title: &firstTitle, TargetVolumeID: &volume.ID})
	require.NoError(t, err)
	assert.Equal(t, git.ChapterPath(first.ID), first.FilePath)
	assert.Equal(t, git.OutlinePath(first.ID), first.OutlineFilePath)
	secondTitle := "第二章"
	second, err := app.PlaceChapter(PlaceChapterInput{NovelID: novel.ID, Title: &secondTitle, TargetVolumeID: &volume.ID})
	require.NoError(t, err)
	insertedTitle := "插入章"
	inserted, err := app.PlaceChapter(PlaceChapterInput{NovelID: novel.ID, Title: &insertedTitle, TargetVolumeID: &volume.ID, BeforeChapterID: &second.ID})
	require.NoError(t, err)

	chapters, err := app.GetChapters(novel.ID)
	require.NoError(t, err)
	assert.Equal(t, []int64{first.ID, inserted.ID, second.ID}, []int64{chapters[0].ID, chapters[1].ID, chapters[2].ID})
	assert.Equal(t, []int{1, 2, 3}, []int{chapters[0].ReadingNumber, chapters[1].ReadingNumber, chapters[2].ReadingNumber})
	for _, ch := range chapters {
		assert.Equal(t, git.ChapterPath(ch.ID), ch.FilePath)
		assert.Equal(t, git.OutlinePath(ch.ID), ch.OutlineFilePath)
	}

	moved, err := app.PlaceChapter(PlaceChapterInput{NovelID: novel.ID, SourceChapterID: &inserted.ID})
	require.NoError(t, err)
	assert.Nil(t, moved.VolumeID)
	assert.Equal(t, 3, moved.ReadingNumber)
	assert.Equal(t, git.OutlinePath(moved.ID), moved.OutlineFilePath)
}

func TestPlaceChapterMovesWithinAndAcrossVolumes(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	firstVolumeName := "第一卷"
	firstVolume, err := app.PlaceVolume(PlaceVolumeInput{NovelID: novel.ID, Name: &firstVolumeName})
	require.NoError(t, err)
	secondVolumeName := "第二卷"
	secondVolume, err := app.PlaceVolume(PlaceVolumeInput{NovelID: novel.ID, Name: &secondVolumeName})
	require.NoError(t, err)

	titleA, titleB, titleC, titleD := "A", "B", "C", "D"
	first, err := app.PlaceChapter(PlaceChapterInput{NovelID: novel.ID, Title: &titleA, TargetVolumeID: &firstVolume.ID})
	require.NoError(t, err)
	second, err := app.PlaceChapter(PlaceChapterInput{NovelID: novel.ID, Title: &titleB, TargetVolumeID: &firstVolume.ID})
	require.NoError(t, err)
	third, err := app.PlaceChapter(PlaceChapterInput{NovelID: novel.ID, Title: &titleC, TargetVolumeID: &firstVolume.ID})
	require.NoError(t, err)
	fourth, err := app.PlaceChapter(PlaceChapterInput{NovelID: novel.ID, Title: &titleD, TargetVolumeID: &secondVolume.ID})
	require.NoError(t, err)

	_, err = app.PlaceChapter(PlaceChapterInput{NovelID: novel.ID, SourceChapterID: &third.ID, TargetVolumeID: &firstVolume.ID, BeforeChapterID: &first.ID})
	require.NoError(t, err)
	moved, err := app.PlaceChapter(PlaceChapterInput{NovelID: novel.ID, SourceChapterID: &second.ID, TargetVolumeID: &secondVolume.ID, BeforeChapterID: &fourth.ID})
	require.NoError(t, err)
	assert.Equal(t, &secondVolume.ID, moved.VolumeID)

	chapters, err := app.GetChapters(novel.ID)
	require.NoError(t, err)
	assert.Equal(t, []int64{third.ID, first.ID, second.ID, fourth.ID}, []int64{chapters[0].ID, chapters[1].ID, chapters[2].ID, chapters[3].ID})
}

func TestPlaceChapterRollsBackRecordAndOrderWhenBodyCreationFails(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	first := createTestChapter(t, app, novel.ID)

	bodyPath, err := git.ResolvePath(git.ChapterPath(first.ID), novel.ID)
	require.NoError(t, err)
	require.NoError(t, os.Remove(bodyPath))
	require.NoError(t, os.Remove(filepath.Dir(bodyPath)))
	require.NoError(t, os.WriteFile(filepath.Dir(bodyPath), []byte("blocked"), 0o644))

	title := "无法创建"
	_, err = app.PlaceChapter(PlaceChapterInput{NovelID: novel.ID, Title: &title, BeforeChapterID: &first.ID})
	require.Error(t, err)

	chapters, err := app.GetChapters(novel.ID)
	require.NoError(t, err)
	require.Len(t, chapters, 1)
	assert.Equal(t, first.ID, chapters[0].ID)
	assert.Equal(t, 1, chapters[0].SortOrder)
}

func TestDeleteChapterReturnsReferencesAndCleansFiles(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	ch := createTestChapter(t, app, novel.ID)
	chapterID := ch.ID
	require.NoError(t, git.WriteFile(novel.ID, git.OutlinePath(chapterID), "章节大纲"))

	require.NoError(t, app.db.Create(&timeline.TimelineEntry{NovelID: novel.ID, Category: "foreshadowing", Status: "pending", Title: "伏笔", SourceChapterID: &chapterID, ResolvedChapterID: &chapterID}).Error)
	require.NoError(t, app.db.Create(&storyarc.ArcNode{NovelID: novel.ID, StoryArcID: 1, Title: "节点", ActualChapterID: &chapterID, Status: "completed"}).Error)
	require.NoError(t, app.db.Create(&reader.ReaderPerspective{NovelID: novel.ID, Type: reader.TypeKnown, Content: "认知", PlantedChapterID: &chapterID, RevealedChapterID: &chapterID}).Error)
	require.NoError(t, app.db.Create(&character.CharacterRelation{NovelID: novel.ID, SourceCharacterID: 1, TargetCharacterID: 2, RelationDescribe: "盟友", ChapterID: &chapterID, IsCurrent: true}).Error)

	result, err := app.DeleteChapter(novel.ID, chapterID)
	require.NoError(t, err)
	assert.False(t, result.Deleted)
	require.Len(t, result.References, 6)
	assert.ElementsMatch(t, []string{
		"timeline_source",
		"timeline_resolved",
		"story_arc",
		"reader_planted",
		"reader_revealed",
		"character_relation",
	}, []string{
		result.References[0].Kind,
		result.References[1].Kind,
		result.References[2].Kind,
		result.References[3].Kind,
		result.References[4].Kind,
		result.References[5].Kind,
	})

	require.NoError(t, app.db.Where("novel_id = ?", novel.ID).Delete(&timeline.TimelineEntry{}).Error)
	require.NoError(t, app.db.Where("novel_id = ?", novel.ID).Delete(&storyarc.ArcNode{}).Error)
	require.NoError(t, app.db.Where("novel_id = ?", novel.ID).Delete(&reader.ReaderPerspective{}).Error)
	require.NoError(t, app.db.Where("novel_id = ?", novel.ID).Delete(&character.CharacterRelation{}).Error)

	result, err = app.DeleteChapter(novel.ID, chapterID)
	require.NoError(t, err)
	assert.True(t, result.Deleted)
	_, err = git.ReadFile(novel.ID, git.ChapterPath(chapterID))
	assert.True(t, errors.Is(err, os.ErrNotExist), "err = %v, want os.ErrNotExist", err)
	_, err = git.ReadFile(novel.ID, git.OutlinePath(chapterID))
	assert.True(t, errors.Is(err, os.ErrNotExist), "err = %v, want os.ErrNotExist", err)
}

func TestDeleteChapterRollsBackRecordWhenFileCleanupFails(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	ch := createTestChapter(t, app, novel.ID)

	outlinePath, err := git.ResolvePath(git.OutlinePath(ch.ID), novel.ID)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(outlinePath, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(outlinePath, "blocked"), []byte("blocked"), 0o644))

	_, err = app.DeleteChapter(novel.ID, ch.ID)
	require.Error(t, err)
	_, err = app.chapter.GetByID(app.ctx, nil, novel.ID, ch.ID)
	require.NoError(t, err)
}
