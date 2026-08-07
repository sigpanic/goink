package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetStoryArcs_Empty(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	novelID := novel.ID

	arcs, err := app.GetStoryArcs(novelID)
	require.NoError(t, err)
	assert.Empty(t, arcs)
}

func TestCreateStoryArc(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	novelID := novel.ID

	arc, err := app.CreateStoryArc(novelID, CreateStoryArcInput{
		Name:        "Revenge Arc",
		ArcType:     "main",
		Description: "The protagonist seeks revenge",
		Importance:  5,
	})
	require.NoError(t, err)
	assert.Equal(t, "Revenge Arc", arc.Name)
	assert.Equal(t, "main", arc.ArcType)
	assert.Equal(t, "The protagonist seeks revenge", arc.Description)
	assert.Equal(t, 5, arc.Importance)
	assert.Equal(t, "active", arc.Status)
	assert.Equal(t, novelID, arc.NovelID)
}

func TestCreateStoryArc_MissingFields(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	novelID := novel.ID

	_, err := app.CreateStoryArc(novelID, CreateStoryArcInput{
		Name:    "",
		ArcType: "main",
	})
	assert.Error(t, err)

	_, err = app.CreateStoryArc(novelID, CreateStoryArcInput{
		Name:    "Some Arc",
		ArcType: "",
	})
	assert.Error(t, err)
}

func TestUpdateStoryArc(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	novelID := novel.ID

	arc, err := app.CreateStoryArc(novelID, CreateStoryArcInput{
		Name:    "Original Arc",
		ArcType: "sub",
	})
	require.NoError(t, err)

	err = app.UpdateStoryArc(novelID, arc.ID, UpdateStoryArcInput{
		Name:   "Updated Arc",
		Status: "paused",
	})
	require.NoError(t, err)

	arcs, err := app.GetStoryArcs(novelID)
	require.NoError(t, err)
	require.Len(t, arcs, 1)
	assert.Equal(t, "Updated Arc", arcs[0].Name)
	assert.Equal(t, "paused", arcs[0].Status)
}

func TestDeleteStoryArc(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	novelID := novel.ID

	arc, err := app.CreateStoryArc(novelID, CreateStoryArcInput{
		Name:    "Doomed Arc",
		ArcType: "background",
	})
	require.NoError(t, err)

	// Create an associated node
	_, err = app.CreateArcNode(novelID, CreateArcNodeInput{
		StoryArcID:    arc.ID,
		Title:         "Node to cascade delete",
		TargetChapter: 5,
	})
	require.NoError(t, err)

	err = app.DeleteStoryArc(novelID, arc.ID)
	require.NoError(t, err)

	// Arc should be gone
	arcs, err := app.GetStoryArcs(novelID)
	require.NoError(t, err)
	assert.Empty(t, arcs)

	// Associated nodes should also be gone
	nodes, err := app.GetArcNodes(novelID)
	require.NoError(t, err)
	assert.Empty(t, nodes)
}

func TestCreateArcNode(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	novelID := novel.ID

	arc, err := app.CreateStoryArc(novelID, CreateStoryArcInput{
		Name:    "Arc for nodes",
		ArcType: "character",
	})
	require.NoError(t, err)

	node, err := app.CreateArcNode(novelID, CreateArcNodeInput{
		StoryArcID:    arc.ID,
		Title:         "Discover enemy identity",
		Description:   "The protagonist learns who the real enemy is",
		TargetChapter: 10,
	})
	require.NoError(t, err)
	assert.Equal(t, arc.ID, node.StoryArcID)
	assert.Equal(t, "Discover enemy identity", node.Title)
	assert.Equal(t, 10, node.TargetChapter)
	assert.Equal(t, "pending", node.Status)
	assert.Equal(t, novelID, node.NovelID)
}

func TestCreateArcNode_MissingFields(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	novelID := novel.ID

	arc, err := app.CreateStoryArc(novelID, CreateStoryArcInput{
		Name:    "Arc",
		ArcType: "main",
	})
	require.NoError(t, err)

	// Missing title
	_, err = app.CreateArcNode(novelID, CreateArcNodeInput{
		StoryArcID:    arc.ID,
		TargetChapter: 5,
	})
	assert.Error(t, err)

	// Missing story_arc_id
	_, err = app.CreateArcNode(novelID, CreateArcNodeInput{
		Title:         "Node",
		TargetChapter: 5,
	})
	assert.Error(t, err)

	// Missing target_chapter (zero value)
	_, err = app.CreateArcNode(novelID, CreateArcNodeInput{
		StoryArcID: arc.ID,
		Title:      "Node",
	})
	assert.Error(t, err)
}

func TestUpdateArcNode(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	novelID := novel.ID

	arc, err := app.CreateStoryArc(novelID, CreateStoryArcInput{
		Name:    "Arc",
		ArcType: "main",
	})
	require.NoError(t, err)

	node, err := app.CreateArcNode(novelID, CreateArcNodeInput{
		StoryArcID:    arc.ID,
		Title:         "Original node",
		TargetChapter: 3,
	})
	require.NoError(t, err)

	err = app.UpdateArcNode(novelID, node.ID, UpdateArcNodeInput{
		Title:         "Updated node",
		ActualChapter: 4,
		Status:        "completed",
	})
	require.NoError(t, err)

	nodes, err := app.GetArcNodes(novelID)
	require.NoError(t, err)
	require.Len(t, nodes, 1)
	assert.Equal(t, "Updated node", nodes[0].Title)
	assert.Equal(t, 4, nodes[0].ActualChapter)
	assert.Equal(t, "completed", nodes[0].Status)
}

func TestDeleteArcNode(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	novelID := novel.ID

	arc, err := app.CreateStoryArc(novelID, CreateStoryArcInput{
		Name:    "Arc",
		ArcType: "sub",
	})
	require.NoError(t, err)

	node, err := app.CreateArcNode(novelID, CreateArcNodeInput{
		StoryArcID:    arc.ID,
		Title:         "Delete me",
		TargetChapter: 7,
	})
	require.NoError(t, err)

	err = app.DeleteArcNode(novelID, node.ID)
	require.NoError(t, err)

	nodes, err := app.GetArcNodes(novelID)
	require.NoError(t, err)
	assert.Empty(t, nodes)

	// Arc should still exist
	arcs, err := app.GetStoryArcs(novelID)
	require.NoError(t, err)
	assert.Len(t, arcs, 1)
}

func TestGetArcNodes_All(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	novelID := novel.ID

	arc, err := app.CreateStoryArc(novelID, CreateStoryArcInput{
		Name:    "Arc",
		ArcType: "main",
	})
	require.NoError(t, err)

	_, err = app.CreateArcNode(novelID, CreateArcNodeInput{
		StoryArcID:    arc.ID,
		Title:         "Early node",
		TargetChapter: 2,
	})
	require.NoError(t, err)

	_, err = app.CreateArcNode(novelID, CreateArcNodeInput{
		StoryArcID:    arc.ID,
		Title:         "Mid node",
		TargetChapter: 5,
	})
	require.NoError(t, err)

	_, err = app.CreateArcNode(novelID, CreateArcNodeInput{
		StoryArcID:    arc.ID,
		Title:         "Late node",
		TargetChapter: 10,
	})
	require.NoError(t, err)

	// 4b: GetArcNodes 改为全量查询（废弃 ListNodesByChapterRange，不再支持章节范围查）
	nodes, err := app.GetArcNodes(novelID)
	require.NoError(t, err)
	assert.Len(t, nodes, 3)
}
