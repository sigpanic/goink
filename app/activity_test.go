package app

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sigpanic/goink/internal/activity"
	"github.com/sigpanic/goink/internal/config"
)

func TestSettingsSavePreservesActivityOrigin(t *testing.T) {
	a := setupTestApp(t)
	require.NotNil(t, a.activity)
	require.NotNil(t, a.settings.TrackingStartedAt)
	startedAt := *a.settings.TrackingStartedAt
	startDate := a.settings.TrackingStartDate
	require.NoError(t, a.SaveUserName("创作者"))
	saved, err := config.LoadSettings(a.db)
	require.NoError(t, err)
	require.NotNil(t, saved.TrackingStartedAt)
	require.True(t, startedAt.Equal(*saved.TrackingStartedAt))
	require.Equal(t, startDate, saved.TrackingStartDate)
	require.Equal(t, "创作者", saved.UserName)
	payload, err := json.Marshal(saved)
	require.NoError(t, err)
	require.NotContains(t, string(payload), "tracking_")
	require.NotContains(t, string(payload), "Tracking")
}

func requireCreationActivity(t *testing.T, a *App, want activity.ActivityDelta) {
	t.Helper()
	var rows []activity.DailyActivity
	require.NoError(t, a.db.Find(&rows).Error)
	var got activity.ActivityDelta
	for _, row := range rows {
		got.NovelsCreated += row.NovelsCreated
		got.ChaptersCreated += row.ChaptersCreated
		got.CharactersCreated += row.CharactersCreated
		got.LocationsCreated += row.LocationsCreated
	}
	require.Equal(t, want, got)
}

func TestCreationActivitySurvivesUpdatesMovesAndDeletion(t *testing.T) {
	a := setupTestApp(t)
	n, err := a.CreateNovel(CreateNovelInput{Title: "年度创作"})
	require.NoError(t, err)
	ch, err := a.CreateChapter(CreateChapterInput{NovelID: n.ID, Title: "第一章"})
	require.NoError(t, err)
	title := "第二章"
	_, err = a.PlaceChapter(PlaceChapterInput{NovelID: n.ID, Title: &title})
	require.NoError(t, err)
	char, err := a.CreateCharacter(n.ID, CreateCharacterInput{Name: "主角"})
	require.NoError(t, err)
	loc, err := a.CreateLocation(n.ID, CreateLocationInput{Name: "城池"})
	require.NoError(t, err)
	want := activity.ActivityDelta{NovelsCreated: 1, ChaptersCreated: 2, CharactersCreated: 1, LocationsCreated: 1}
	requireCreationActivity(t, a, want)

	require.NoError(t, a.UpdateChapterTitle(n.ID, ch.ID, "新的标题"))
	_, err = a.PlaceChapter(PlaceChapterInput{NovelID: n.ID, SourceChapterID: &ch.ID})
	require.NoError(t, err)
	require.NoError(t, a.UpdateCharacter(n.ID, char.ID, UpdateCharacterInput{Name: "新名字"}))
	require.NoError(t, a.UpdateLocation(n.ID, loc.ID, UpdateLocationInput{Name: "新城池"}))
	requireCreationActivity(t, a, want)

	require.NoError(t, a.DeleteCharacter(n.ID, char.ID))
	require.NoError(t, a.DeleteLocation(n.ID, loc.ID))
	deleted, err := a.DeleteChapter(n.ID, ch.ID)
	require.NoError(t, err)
	require.True(t, deleted.Deleted)
	requireCreationActivity(t, a, want)
	require.NoError(t, a.DeleteNovel(n.ID))
	requireCreationActivity(t, a, want)
}

func TestCreationActivitySkipsFailures(t *testing.T) {
	a := setupTestApp(t)
	n := createTestNovel(t, a)
	_, err := a.CreateCharacter(n.ID, CreateCharacterInput{})
	require.Error(t, err)
	_, err = a.CreateLocation(n.ID, CreateLocationInput{})
	require.Error(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(config.NovelDirPath(n.ID), "chapters"), []byte("blocked"), 0o644))
	_, err = a.CreateChapter(CreateChapterInput{NovelID: n.ID, Title: "写入失败"})
	require.Error(t, err)
	title := "写入失败"
	_, err = a.PlaceChapter(PlaceChapterInput{NovelID: n.ID, Title: &title})
	require.Error(t, err)
	t.Setenv("PATH", t.TempDir())
	_, err = a.CreateNovel(CreateNovelInput{Title: "Git 初始化失败"})
	require.Error(t, err)
	requireCreationActivity(t, a, activity.ActivityDelta{})
}

func TestCreationSucceedsWhenActivityWriteFails(t *testing.T) {
	a := setupTestApp(t)
	require.NoError(t, a.db.Exec(`CREATE TRIGGER reject_activity BEFORE INSERT ON activity_daily
		BEGIN SELECT RAISE(ABORT, 'statistics unavailable'); END`).Error)
	n, err := a.CreateNovel(CreateNovelInput{Title: "统计不可用"})
	require.NoError(t, err)
	_, err = a.CreateChapter(CreateChapterInput{NovelID: n.ID, Title: "章节"})
	require.NoError(t, err)
	title := "插入章节"
	_, err = a.PlaceChapter(PlaceChapterInput{NovelID: n.ID, Title: &title})
	require.NoError(t, err)
	_, err = a.CreateCharacter(n.ID, CreateCharacterInput{Name: "主角"})
	require.NoError(t, err)
	_, err = a.CreateLocation(n.ID, CreateLocationInput{Name: "城池"})
	require.NoError(t, err)
	requireCreationActivity(t, a, activity.ActivityDelta{})
}

func TestRecordCreativeActivity(t *testing.T) {
	a := setupTestApp(t)
	var expected int64
	for _, tc := range []struct {
		seconds int64
		added   int64
	}{
		{0, 0}, {-1, 0}, {math.MinInt64, 0},
		{1, 1}, {30, 30}, {60, 60}, {61, 60}, {math.MaxInt64, 60},
	} {
		a.RecordCreativeActivity(tc.seconds)
		expected += tc.added
		var rows []activity.DailyActivity
		require.NoError(t, a.db.Find(&rows).Error)
		if expected == 0 {
			require.Empty(t, rows)
			continue
		}
		require.Len(t, rows, 1)
		require.Equal(t, activity.ActivityDelta{CreativeSeconds: expected}, rows[0].ActivityDelta)
	}
}

func TestRecordCreativeActivitySkipsUnavailableStorage(t *testing.T) {
	require.NotPanics(t, func() { (&App{}).RecordCreativeActivity(60) })
	a := setupTestApp(t)
	require.NoError(t, a.db.Exec(`CREATE TRIGGER reject_activity BEFORE INSERT ON activity_daily
		BEGIN SELECT RAISE(ABORT, 'statistics unavailable'); END`).Error)
	require.NotPanics(t, func() { a.RecordCreativeActivity(60) })
	var count int64
	require.NoError(t, a.db.Model(&activity.DailyActivity{}).Count(&count).Error)
	require.Zero(t, count)
}
