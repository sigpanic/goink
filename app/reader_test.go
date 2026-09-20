package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetReaderPerspectives_Empty(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	novelID := novel.ID

	perspectives, err := app.GetReaderPerspectives(novelID)
	require.NoError(t, err)
	assert.Empty(t, perspectives)
}

func TestCreateReaderPerspective(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	novelID := novel.ID
	chapter := createTestChapter(t, app, novelID)

	p, err := app.CreateReaderPerspective(novelID, CreateReaderPerspectiveInput{
		Type:             "known",
		Content:          "The protagonist is an orphan",
		PlantedChapterID: chapter.ID,
		RelatedTruth:     "Parents are alive and in hiding",
	})
	require.NoError(t, err)
	assert.Equal(t, "known", p.Type)
	assert.Equal(t, "The protagonist is an orphan", p.Content)
	require.NotNil(t, p.PlantedChapterID)
	assert.Equal(t, chapter.ID, *p.PlantedChapterID)
	assert.Equal(t, "Parents are alive and in hiding", p.RelatedTruth)
	assert.Equal(t, novelID, p.NovelID)
}

func TestCreateReaderPerspective_EmptyFields(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	novelID := novel.ID

	_, err := app.CreateReaderPerspective(novelID, CreateReaderPerspectiveInput{
		Type:    "",
		Content: "Some content",
	})
	assert.Error(t, err)

	_, err = app.CreateReaderPerspective(novelID, CreateReaderPerspectiveInput{
		Type:    "suspense",
		Content: "",
	})
	assert.Error(t, err)
}

func TestGetReaderPerspectives_AfterCreate(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	novelID := novel.ID
	chapter1 := createTestChapter(t, app, novelID)
	chapter2 := createTestChapter(t, app, novelID)

	_, err := app.CreateReaderPerspective(novelID, CreateReaderPerspectiveInput{
		Type:             "known",
		Content:          "Reader knows the hero's name",
		PlantedChapterID: chapter1.ID,
	})
	require.NoError(t, err)

	_, err = app.CreateReaderPerspective(novelID, CreateReaderPerspectiveInput{
		Type:             "suspense",
		Content:          "Who is the traitor?",
		PlantedChapterID: chapter2.ID,
	})
	require.NoError(t, err)

	perspectives, err := app.GetReaderPerspectives(novelID)
	require.NoError(t, err)
	assert.Len(t, perspectives, 2)
}

func TestUpdateReaderPerspective(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	novelID := novel.ID
	planted := createTestChapter(t, app, novelID)
	revealed := createTestChapter(t, app, novelID)

	p, err := app.CreateReaderPerspective(novelID, CreateReaderPerspectiveInput{
		Type:             "suspense",
		Content:          "Who killed the king?",
		PlantedChapterID: planted.ID,
	})
	require.NoError(t, err)

	err = app.UpdateReaderPerspective(p.ID, novelID, UpdateReaderPerspectiveInput{
		PlantedChapterID:  int64Ptr(planted.ID),
		RevealedChapterID: int64Ptr(revealed.ID),
		Content:           "Who killed the king? (resolved)",
	})
	require.NoError(t, err)

	perspectives, err := app.GetReaderPerspectives(novelID)
	require.NoError(t, err)
	require.Len(t, perspectives, 1)
	require.NotNil(t, perspectives[0].RevealedChapterID)
	assert.Equal(t, revealed.ID, *perspectives[0].RevealedChapterID)
	assert.Equal(t, "Who killed the king? (resolved)", perspectives[0].Content)
}

func TestDeleteReaderPerspective(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	novelID := novel.ID
	chapter := createTestChapter(t, app, novelID)

	p, err := app.CreateReaderPerspective(novelID, CreateReaderPerspectiveInput{
		Type:             "misconception",
		Content:          "Reader thinks the mentor is good",
		PlantedChapterID: chapter.ID,
	})
	require.NoError(t, err)

	err = app.DeleteReaderPerspective(p.ID, novelID)
	require.NoError(t, err)

	perspectives, err := app.GetReaderPerspectives(novelID)
	require.NoError(t, err)
	assert.Empty(t, perspectives)
}

func TestCreateReaderPerspective_RejectsForeignChapter(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	otherNovel := createTestNovel(t, app)
	otherChapter := createTestChapter(t, app, otherNovel.ID)

	_, err := app.CreateReaderPerspective(novel.ID, CreateReaderPerspectiveInput{
		Type:             "known",
		Content:          "跨小说章节",
		PlantedChapterID: otherChapter.ID,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "不属于当前小说")
}
