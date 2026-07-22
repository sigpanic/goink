package app

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// ListStyleSamples
// ---------------------------------------------------------------------------

func TestListStyleSamples_Empty(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)

	result, err := app.ListStyleSamples(ListStyleSamplesInput{
		NovelID: novel.ID,
		Page:    1,
		Size:    20,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(0), result.Total)
	assert.Empty(t, result.Items)
}

// ---------------------------------------------------------------------------
// CreateStyleSample
// ---------------------------------------------------------------------------

func TestCreateStyleSample(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)

	content := strings.Repeat("这是一段很长的测试内容，用于验证预览截断功能是否正常工作。", 10)
	sample, err := app.CreateStyleSample(CreateStyleSampleInput{
		NovelID:  novel.ID,
		IsGlobal: false,
		Name:     "测试素材",
		Content:  content,
		Tags:     []string{"标签A", "标签B"},
	})
	require.NoError(t, err)
	require.NotZero(t, sample.ID)
	assert.Equal(t, novel.ID, sample.NovelID)
	assert.False(t, sample.IsGlobal)
	assert.Equal(t, "测试素材", sample.Name)
	assert.Equal(t, content, sample.Content)
	assert.Greater(t, sample.WordCount, 0)
	assert.Contains(t, sample.Tags, "标签A")
	assert.Contains(t, sample.Tags, "标签B")
	assert.Len(t, sample.Tags, 2)

	// Preview should be truncated to PreviewMaxRunes (120 runes).
	runes := []rune(content)
	if len(runes) > 120 {
		previewRunes := []rune(sample.Preview)
		assert.Equal(t, 121, len(previewRunes), "preview should be 120 runes + ellipsis")
		assert.True(t, strings.HasSuffix(sample.Preview, "…"))
	} else {
		assert.Equal(t, content, sample.Preview)
	}
}

// ---------------------------------------------------------------------------
// GetStyleSample
// ---------------------------------------------------------------------------

func TestGetStyleSample(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)

	created, err := app.CreateStyleSample(CreateStyleSampleInput{
		NovelID: novel.ID,
		Name:    "待查询",
		Content: "完整内容",
		Tags:    []string{"test"},
	})
	require.NoError(t, err)

	got, err := app.GetStyleSample(created.ID)
	require.NoError(t, err)
	assert.Equal(t, created.ID, got.ID)
	assert.Equal(t, "待查询", got.Name)
	assert.Equal(t, "完整内容", got.Content)
}

// ---------------------------------------------------------------------------
// UpdateStyleSample
// ---------------------------------------------------------------------------

func TestUpdateStyleSample(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)

	created, err := app.CreateStyleSample(CreateStyleSampleInput{
		NovelID:  novel.ID,
		Name:     "原始名称",
		Content:  "原始内容",
		Tags:     []string{"旧标签"},
		IsGlobal: false,
	})
	require.NoError(t, err)

	updated, err := app.UpdateStyleSample(UpdateStyleSampleInput{
		ID:       created.ID,
		Name:     "更新后名称",
		Content:  "更新后的新内容",
		Tags:     []string{"新标签", "额外标签"},
		IsGlobal: true,
		NovelID:  novel.ID,
	})
	require.NoError(t, err)
	assert.Equal(t, created.ID, updated.ID)
	assert.Equal(t, "更新后名称", updated.Name)
	assert.Equal(t, "更新后的新内容", updated.Content)
	assert.True(t, updated.IsGlobal)
	assert.Contains(t, updated.Tags, "新标签")
	assert.Contains(t, updated.Tags, "额外标签")

	// WordCount should reflect the new content.
	assert.Greater(t, updated.WordCount, 0)
}

// ---------------------------------------------------------------------------
// DeleteStyleSample
// ---------------------------------------------------------------------------

func TestDeleteStyleSample(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)

	created, err := app.CreateStyleSample(CreateStyleSampleInput{
		NovelID: novel.ID,
		Name:    "待删除",
		Content: "内容",
		Tags:    nil,
	})
	require.NoError(t, err)

	err = app.DeleteStyleSample(DeleteStyleSampleInput{ID: created.ID})
	require.NoError(t, err)

	// Verify the record is gone.
	_, err = app.GetStyleSample(created.ID)
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// ComputeStyleStats
// ---------------------------------------------------------------------------

func TestComputeStyleStats(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)

	var ids []int64
	for i := 0; i < 3; i++ {
		sample, err := app.CreateStyleSample(CreateStyleSampleInput{
			NovelID: novel.ID,
			Name:    string(rune('A' + rune(i))),
			Content: "这是第一句。这是第二句，包含逗号。这是第三句！这是第四句？",
			Tags:    nil,
		})
		require.NoError(t, err)
		ids = append(ids, sample.ID)
	}

	stats, err := app.ComputeStyleStats(ComputeStyleStatsInput{SampleIDs: ids})
	require.NoError(t, err)
	assert.Greater(t, stats.TotalChars, 0)
	assert.Greater(t, stats.TotalWords, 0)
	assert.Greater(t, stats.SentenceCount, 0)
	assert.Greater(t, stats.ParagraphCount, 0)
	assert.Greater(t, stats.CommaDensity, float64(0))
	assert.Greater(t, stats.PeriodDensity, float64(0))
}

func TestComputeStyleStats_NoSamples(t *testing.T) {
	app := setupTestApp(t)

	stats, err := app.ComputeStyleStats(ComputeStyleStatsInput{SampleIDs: []int64{999999}})
	assert.Nil(t, stats)
	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "没有选中的素材")
	}
}
