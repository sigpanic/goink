package app

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/sigpanic/goink/internal/git"
	"github.com/sigpanic/goink/internal/volume"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVolumeManagementAPI(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)

	firstName := "第一卷"
	first, err := app.PlaceVolume(PlaceVolumeInput{NovelID: novel.ID, Name: &firstName})
	require.NoError(t, err)
	assert.Equal(t, git.VolumePath(first.ID), first.OutlineFilePath)
	secondName := "第二卷"
	second, err := app.PlaceVolume(PlaceVolumeInput{NovelID: novel.ID, Name: &secondName})
	require.NoError(t, err)
	assert.Equal(t, git.VolumePath(second.ID), second.OutlineFilePath)

	volumes, err := app.GetVolumes(novel.ID)
	require.NoError(t, err)
	require.Len(t, volumes, 2)
	assert.Equal(t, []int64{first.ID, second.ID}, []int64{volumes[0].ID, volumes[1].ID})
	assert.Equal(t, []string{first.OutlineFilePath, second.OutlineFilePath}, []string{volumes[0].OutlineFilePath, volumes[1].OutlineFilePath})

	require.NoError(t, app.UpdateVolume(novel.ID, second.ID, "终卷"))
	moved, err := app.PlaceVolume(PlaceVolumeInput{NovelID: novel.ID, SourceVolumeID: &first.ID})
	require.NoError(t, err)
	assert.Equal(t, first.OutlineFilePath, moved.OutlineFilePath)

	volumes, err = app.GetVolumes(novel.ID)
	require.NoError(t, err)
	require.Len(t, volumes, 2)
	assert.Equal(t, "终卷", volumes[0].Name)
	assert.Equal(t, []int64{second.ID, first.ID}, []int64{volumes[0].ID, volumes[1].ID})
	assert.Equal(t, []string{second.OutlineFilePath, first.OutlineFilePath}, []string{volumes[0].OutlineFilePath, volumes[1].OutlineFilePath})

	require.NoError(t, git.WriteFile(novel.ID, git.VolumePath(first.ID), "第一卷大纲"))
	require.NoError(t, app.DeleteVolume(novel.ID, first.ID))
	_, err = git.ReadFile(novel.ID, git.VolumePath(first.ID))
	assert.True(t, errors.Is(err, os.ErrNotExist), "err = %v, want os.ErrNotExist", err)
	volumes, err = app.GetVolumes(novel.ID)
	require.NoError(t, err)
	require.Len(t, volumes, 1)
	assert.Equal(t, second.ID, volumes[0].ID)
	require.NoError(t, app.DeleteVolume(novel.ID, second.ID))
	volumes, err = app.GetVolumes(novel.ID)
	require.NoError(t, err)
	assert.Empty(t, volumes)
}

func TestDeleteVolumeRejectsNonEmptyOrForeignVolume(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	otherNovel := createTestNovel(t, app)

	volumeName := "第一卷"
	v, err := app.PlaceVolume(PlaceVolumeInput{NovelID: novel.ID, Name: &volumeName})
	require.NoError(t, err)
	_, err = app.CreateChapter(CreateChapterInput{NovelID: novel.ID, Title: "卷内章节"})
	require.NoError(t, err)
	require.NoError(t, git.WriteFile(novel.ID, git.VolumePath(v.ID), "不可删除的卷纲"))

	err = app.DeleteVolume(novel.ID, v.ID)
	assert.ErrorIs(t, err, volume.ErrHasChapters)
	content, err := git.ReadFile(novel.ID, git.VolumePath(v.ID))
	require.NoError(t, err)
	assert.Equal(t, "不可删除的卷纲", content)

	err = app.UpdateVolume(otherNovel.ID, v.ID, "越权改名")
	assert.True(t, errors.Is(err, volume.ErrNotFound), "err = %v, want ErrNotFound", err)
}

func TestDeleteVolumeRollsBackWhenOutlineCleanupFails(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	volumeName := "第一卷"
	v, err := app.PlaceVolume(PlaceVolumeInput{NovelID: novel.ID, Name: &volumeName})
	require.NoError(t, err)

	outlinePath, err := git.ResolvePath(git.VolumePath(v.ID), novel.ID)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(outlinePath, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(outlinePath, "blocked"), []byte("blocked"), 0o644))

	err = app.DeleteVolume(novel.ID, v.ID)
	require.Error(t, err)
	_, err = app.volume.GetByID(app.ctx, nil, novel.ID, v.ID)
	require.NoError(t, err)
}
