package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetLocations_Empty(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	novelID := novel.ID

	locs, err := app.GetLocations(novelID)
	require.NoError(t, err)
	assert.Empty(t, locs)
}

func TestCreateLocation(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	novelID := novel.ID

	loc, err := app.CreateLocation(novelID, CreateLocationInput{
		Name:         "迷雾森林",
		LocationType: "森林",
		Description:  "常年笼罩在浓雾中的古老森林",
		DetailJSON:   `{"气候":"常年阴雨","氛围":"压抑诡异"}`,
		Tags:         `["危险","神秘"]`,
	})
	require.NoError(t, err)
	assert.NotZero(t, loc.ID)
	assert.Equal(t, novelID, loc.NovelID)
	assert.Equal(t, "迷雾森林", loc.Name)
	assert.Equal(t, "森林", loc.LocationType)
	assert.Equal(t, "常年笼罩在浓雾中的古老森林", loc.Description)
	assert.Equal(t, `{"气候":"常年阴雨","氛围":"压抑诡异"}`, loc.DetailJSON)
	assert.Nil(t, loc.ParentLocationID)
	assert.Equal(t, `["危险","神秘"]`, loc.Tags)
}

func TestCreateLocation_EmptyName(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	novelID := novel.ID

	_, err := app.CreateLocation(novelID, CreateLocationInput{
		Name: "",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "地点名称不能为空")
}

func TestGetLocations_AfterCreate(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	novelID := novel.ID

	_, err := app.CreateLocation(novelID, CreateLocationInput{Name: "迷雾森林"})
	require.NoError(t, err)
	_, err = app.CreateLocation(novelID, CreateLocationInput{Name: "黑铁城堡"})
	require.NoError(t, err)

	locs, err := app.GetLocations(novelID)
	require.NoError(t, err)
	assert.Len(t, locs, 2)

	names := map[string]bool{}
	for _, l := range locs {
		names[l.Name] = true
	}
	assert.True(t, names["迷雾森林"])
	assert.True(t, names["黑铁城堡"])
}

func TestUpdateLocation(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	novelID := novel.ID

	loc, err := app.CreateLocation(novelID, CreateLocationInput{
		Name:        "旧名",
		Description: "旧描述",
	})
	require.NoError(t, err)

	err = app.UpdateLocation(novelID, loc.ID, UpdateLocationInput{
		Name:        "新名",
		Description: "新描述",
	})
	require.NoError(t, err)

	locs, err := app.GetLocations(novelID)
	require.NoError(t, err)
	require.Len(t, locs, 1)
	assert.Equal(t, "新名", locs[0].Name)
	assert.Equal(t, "新描述", locs[0].Description)
}

func TestUpdateLocation_ClearParent(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	novelID := novel.ID

	parent, err := app.CreateLocation(novelID, CreateLocationInput{
		Name: "王宫",
	})
	require.NoError(t, err)

	child, err := app.CreateLocation(novelID, CreateLocationInput{
		Name:             "大殿",
		ParentLocationID: &parent.ID,
	})
	require.NoError(t, err)
	assert.NotNil(t, child.ParentLocationID)
	assert.Equal(t, parent.ID, *child.ParentLocationID)

	// Clear the parent by passing nil ParentLocationID (PUT semantics)
	err = app.UpdateLocation(novelID, child.ID, UpdateLocationInput{
		Name:             child.Name,
		LocationType:     child.LocationType,
		Description:      child.Description,
		ParentLocationID: nil,
		Tags:             child.Tags,
	})
	require.NoError(t, err)

	locs, err := app.GetLocations(novelID)
	require.NoError(t, err)

	for _, l := range locs {
		if l.ID == child.ID {
			assert.Nil(t, l.ParentLocationID, "parent_location_id should be nil after ClearParent")
			return
		}
	}
	t.Fatal("child location not found")
}

func TestDeleteLocation(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	novelID := novel.ID

	parent, err := app.CreateLocation(novelID, CreateLocationInput{
		Name: "王宫",
	})
	require.NoError(t, err)

	child, err := app.CreateLocation(novelID, CreateLocationInput{
		Name:             "大殿",
		ParentLocationID: &parent.ID,
	})
	require.NoError(t, err)

	// Delete the parent — child's parent_location_id should become nil
	err = app.DeleteLocation(novelID, parent.ID)
	require.NoError(t, err)

	locs, err := app.GetLocations(novelID)
	require.NoError(t, err)
	require.Len(t, locs, 1)
	assert.Equal(t, child.ID, locs[0].ID)
	assert.Nil(t, locs[0].ParentLocationID, "child's parent_location_id should be nil after parent deletion")
}

func TestGetLocationRelations(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	novelID := novel.ID

	rels, err := app.GetLocationRelations(novelID)
	require.NoError(t, err)
	assert.Empty(t, rels)
}
