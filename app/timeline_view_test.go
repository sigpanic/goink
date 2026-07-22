package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetChapterPlans_Empty(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	novelID := novel.ID

	plans, err := app.GetChapterPlans(novelID)
	require.NoError(t, err)
	// GetPlans auto-creates 3 empty plan slots (next/near/far)
	require.Len(t, plans, 3)
	for _, p := range plans {
		assert.Empty(t, p.Content, "plan content should be empty before any update")
	}
}

func TestUpdateChapterPlan(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	novelID := novel.ID

	err := app.UpdateChapterPlan(novelID, UpdateChapterPlanInput{
		Scope:   "next",
		Content: "主角发现真相",
	})
	require.NoError(t, err)
}

func TestGetChapterPlans_AfterUpdate(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	novelID := novel.ID

	err := app.UpdateChapterPlan(novelID, UpdateChapterPlanInput{
		Scope:   "next",
		Content: "主角发现真相",
	})
	require.NoError(t, err)

	err = app.UpdateChapterPlan(novelID, UpdateChapterPlanInput{
		Scope:   "near",
		Content: "大决战",
	})
	require.NoError(t, err)

	plans, err := app.GetChapterPlans(novelID)
	require.NoError(t, err)
	require.Len(t, plans, 3)

	planMap := map[string]string{}
	for _, p := range plans {
		planMap[p.Scope] = p.Content
	}
	assert.Equal(t, "主角发现真相", planMap["next"])
	assert.Equal(t, "大决战", planMap["near"])
	assert.Equal(t, "", planMap["far"])
}

func TestCreateTimelineEntry(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	novelID := novel.ID

	entry, err := app.CreateTimelineEntry(novelID, CreateTimelineEntryInput{
		Category:      "foreshadowing",
		Title:         "神秘剑谱",
		Content:       "主角捡到一本神秘剑谱",
		TargetChapter: 5,
		Importance:    4,
		Source:        "user",
	})
	require.NoError(t, err)
	assert.NotZero(t, entry.ID)
	assert.Equal(t, novelID, entry.NovelID)
	assert.Equal(t, "foreshadowing", entry.Category)
	assert.Equal(t, "神秘剑谱", entry.Title)
	assert.Equal(t, "主角捡到一本神秘剑谱", entry.Content)
	assert.Equal(t, 5, entry.TargetChapter)
	assert.Equal(t, 4, entry.Importance)
	assert.Equal(t, "pending", entry.Status)
	assert.Equal(t, "user", entry.Source)
}

func TestCreateTimelineEntry_MissingFields(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	novelID := novel.ID

	_, err := app.CreateTimelineEntry(novelID, CreateTimelineEntryInput{
		Category: "",
		Title:    "",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "标题、类型、目标章节不能为空")
}

func TestGetTimelineEntries(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	novelID := novel.ID

	_, err := app.CreateTimelineEntry(novelID, CreateTimelineEntryInput{
		Category:      "foreshadowing",
		Title:         "伏笔A",
		TargetChapter: 3,
	})
	require.NoError(t, err)

	_, err = app.CreateTimelineEntry(novelID, CreateTimelineEntryInput{
		Category:      "foreshadowing",
		Title:         "伏笔B",
		TargetChapter: 7,
	})
	require.NoError(t, err)

	_, err = app.CreateTimelineEntry(novelID, CreateTimelineEntryInput{
		Category:      "user_directive",
		Title:         "指令C",
		TargetChapter: 10,
	})
	require.NoError(t, err)

	// Query range [1, 5] — should only return entry with target_chapter=3
	entries, err := app.GetTimelineEntries(novelID, 1, 5)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "伏笔A", entries[0].Title)

	// Query all (0, 0 means no filter)
	all, err := app.GetTimelineEntries(novelID, 0, 0)
	require.NoError(t, err)
	assert.Len(t, all, 3)
}

func TestUpdateTimelineEntry(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	novelID := novel.ID

	entry, err := app.CreateTimelineEntry(novelID, CreateTimelineEntryInput{
		Category:      "foreshadowing",
		Title:         "伏笔A",
		Content:       "旧内容",
		TargetChapter: 5,
	})
	require.NoError(t, err)

	err = app.UpdateTimelineEntry(novelID, entry.ID, UpdateTimelineEntryInput{
		Title:      "伏笔A-更新",
		Content:    "新内容",
		Status:     "resolved",
		Importance: 5,
	})
	require.NoError(t, err)

	entries, err := app.GetTimelineEntries(novelID, 0, 0)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "伏笔A-更新", entries[0].Title)
	assert.Equal(t, "新内容", entries[0].Content)
	assert.Equal(t, "resolved", entries[0].Status)
	assert.Equal(t, 5, entries[0].Importance)
}

func TestDeleteTimelineEntry(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	novelID := novel.ID

	entry, err := app.CreateTimelineEntry(novelID, CreateTimelineEntryInput{
		Category:      "foreshadowing",
		Title:         "伏笔A",
		TargetChapter: 5,
	})
	require.NoError(t, err)

	err = app.DeleteTimelineEntry(novelID, entry.ID)
	require.NoError(t, err)

	entries, err := app.GetTimelineEntries(novelID, 0, 0)
	require.NoError(t, err)
	assert.Empty(t, entries)
}
