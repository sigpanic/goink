//go:build cgo

package mcp_tools_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/activity"
	"github.com/sigpanic/goink/internal/chapter"
	"github.com/sigpanic/goink/internal/character"
	"github.com/sigpanic/goink/internal/config"
	"github.com/sigpanic/goink/internal/location"
	"github.com/sigpanic/goink/internal/preference"
	"github.com/sigpanic/goink/internal/reader"
	"github.com/sigpanic/goink/internal/session"
	"github.com/sigpanic/goink/internal/setting"
	"github.com/sigpanic/goink/internal/storage"
	"github.com/sigpanic/goink/internal/storyarc"
	"github.com/sigpanic/goink/internal/timeline"
)

func requireToolCreationActivity(t *testing.T, db *gorm.DB, want activity.ActivityDelta) {
	t.Helper()
	var rows []activity.DailyActivity
	require.NoError(t, db.Find(&rows).Error)
	var got activity.ActivityDelta
	for _, row := range rows {
		got.ChaptersCreated += row.ChaptersCreated
		got.CharactersCreated += row.CharactersCreated
		got.LocationsCreated += row.LocationsCreated
	}
	require.Equal(t, want, got)
}

func TestToolCreationActivityBatchAndRollback(t *testing.T) {
	for _, tt := range []struct {
		name, table, args, updateArgs string
		want                          activity.ActivityDelta
	}{
		{"character", "characters", `{"characters":[{"name":"甲"},{"name":"乙"}]}`, `{"character_id":1,"name":"新名字"}`, activity.ActivityDelta{CharactersCreated: 2}},
		{"location", "locations", `{"locations":[{"name":"甲"},{"name":"乙"}]}`, `{"location_id":1,"name":"新名字"}`, activity.ActivityDelta{LocationsCreated: 2}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db, tc, ctx := setupRWEnv(t)
			require.NoError(t, db.AutoMigrate(&character.Character{}, &location.Location{}, &storage.OperationLogRecord{}, &session.Session{}, &session.Message{}))
			require.NoError(t, storage.RegisterOplogHooks(db))
			reg := newTestRegistry(t)
			turnCtx := storage.WithTurn(ctx, "activity-test", 1)
			res := reg.Execute(turnCtx, "create_"+tt.name, json.RawMessage(tt.args), tc, nil)
			require.True(t, res.Success, "%+v", res)
			requireToolCreationActivity(t, db, tt.want)
			res = reg.Execute(turnCtx, "update_"+tt.name, json.RawMessage(tt.updateArgs), tc, nil)
			require.True(t, res.Success, "%+v", res)
			requireToolCreationActivity(t, db, tt.want)

			require.NoError(t, storage.RollbackTo(ctx, db, "activity-test", 1, 1))
			var count int64
			require.NoError(t, db.Table(tt.table).Count(&count).Error)
			require.Zero(t, count)
			requireToolCreationActivity(t, db, tt.want)

			require.NoError(t, db.Exec(fmt.Sprintf(`CREATE TRIGGER reject_second BEFORE INSERT ON %s
				WHEN NEW.name = '乙' BEGIN SELECT RAISE(ABORT, 'second item rejected'); END`, tt.table)).Error)
			res = reg.Execute(ctx, "create_"+tt.name, json.RawMessage(tt.args), tc, nil)
			require.False(t, res.Success)
			require.NoError(t, db.Table(tt.table).Count(&count).Error)
			require.Zero(t, count)
			requireToolCreationActivity(t, db, tt.want)
		})
	}
}

func TestToolChapterCreationActivity(t *testing.T) {
	for _, prefix := range []string{"chapters", "outlines"} {
		t.Run(prefix, func(t *testing.T) {
			db, tc, ctx := setupRWEnv(t)
			res := execEdit(t, ctx, tc, editArgs(prefix+"/new.md", "full_replace", "新章节"))
			require.True(t, res.Success, "%+v", res)
			id := res.Data["chapter_id"].(int64)
			res = execEdit(t, ctx, tc, editArgs(fmt.Sprintf("chapters/id_%d.md", id), "full_replace", "补写正文"))
			require.True(t, res.Success, "%+v", res)
			requireToolCreationActivity(t, db, activity.ActivityDelta{ChaptersCreated: 1})
		})
	}
}

func TestToolChapterCreationFailureDoesNotCount(t *testing.T) {
	db, tc, ctx := setupRWEnv(t)
	require.NoError(t, os.MkdirAll(config.NovelDirPath(tc.NovelID), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(config.NovelDirPath(tc.NovelID), "chapters"), []byte("blocked"), 0o644))
	res := execEdit(t, ctx, tc, editArgs("chapters/new.md", "full_replace", "创建失败"))
	require.False(t, res.Success)
	require.Zero(t, chapterCount(t, db, tc.NovelID))
	requireToolCreationActivity(t, db, activity.ActivityDelta{})
}

func TestToolCreationSucceedsWhenActivityWriteFails(t *testing.T) {
	db, tc, ctx := setupRWEnv(t)
	require.NoError(t, db.AutoMigrate(&character.Character{}, &location.Location{}))
	require.NoError(t, db.Exec(`CREATE TRIGGER reject_activity BEFORE INSERT ON activity_daily
		BEGIN SELECT RAISE(ABORT, 'statistics unavailable'); END`).Error)
	reg := newTestRegistry(t)
	for _, tt := range []struct{ name, args string }{
		{"create_character", `{"characters":[{"name":"甲"},{"name":"乙"}]}`},
		{"create_location", `{"locations":[{"name":"城池"}]}`},
	} {
		res := reg.Execute(ctx, tt.name, json.RawMessage(tt.args), tc, nil)
		require.True(t, res.Success, "%+v", res)
	}
	res := execEdit(t, ctx, tc, editArgs("chapters/new.md", "full_replace", "正文"))
	require.True(t, res.Success, "%+v", res)
	requireToolCreationActivity(t, db, activity.ActivityDelta{})
}

func TestToolUpsertActivityCountsOnlyCommittedCreations(t *testing.T) {
	for _, tt := range []struct {
		name, table, list, id string
		want                  activity.ActivityDelta
	}{
		{"upsert_setting", "setting_items", "settings", "setting_id", activity.ActivityDelta{SettingsCreated: 2, ToolCalls: 3}},
		{"upsert_preference", "preference_items", "preferences", "preference_id", activity.ActivityDelta{PreferencesCreated: 2, ToolCalls: 3}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db, tc, ctx := setupRWEnv(t)
			require.NoError(t, db.AutoMigrate(&setting.SettingItem{}, &preference.PreferenceItem{}))
			reg := newTestRegistry(t)
			res := reg.Execute(ctx, tt.name, json.RawMessage(fmt.Sprintf(`{"%s":[{"category":"风格","content":"初稿"}]}`, tt.list)), tc, nil)
			require.True(t, res.Success, "%+v", res)
			res = reg.Execute(ctx, tt.name, json.RawMessage(fmt.Sprintf(`{"%s":[{"%s":1,"category":"风格","content":"修改"},{"category":"节奏","content":"新增"}]}`, tt.list, tt.id)), tc, nil)
			require.True(t, res.Success, "%+v", res)
			res = reg.Execute(ctx, tt.name, json.RawMessage(fmt.Sprintf(`{"%s":[{"category":"回滚","content":"未提交"},{"%s":999,"category":"失败","content":"不存在"}]}`, tt.list, tt.id)), tc, nil)
			require.False(t, res.Success)
			var count int64
			require.NoError(t, db.Table(tt.table).Count(&count).Error)
			require.Equal(t, int64(2), count)
			var rows []activity.DailyActivity
			require.NoError(t, db.Find(&rows).Error)
			require.Len(t, rows, 1)
			require.Equal(t, tt.want, rows[0].ActivityDelta)
		})
	}
}

func TestToolSkillActivitySkipsOverwrites(t *testing.T) {
	db, tc, ctx := setupRWEnv(t)
	path := "skills/test.md"
	content := "---\nname: test\n---\n正文"
	res := execEdit(t, ctx, tc, editArgs(path, "full_replace", content))
	require.True(t, res.Success, "%+v", res)
	content += "修改"
	res = execEdit(t, ctx, tc, editArgs(path, "full_replace", content))
	require.True(t, res.Success, "%+v", res)
	res = execEdit(t, ctx, tc, editArgs(path, "full_replace", content))
	require.True(t, res.Success, "%+v", res)
	res = execEdit(t, ctx, tc, editArgs(path, "full_replace", "invalid skill"))
	require.False(t, res.Success)
	var rows []activity.DailyActivity
	require.NoError(t, db.Find(&rows).Error)
	require.Len(t, rows, 1)
	require.Equal(t, activity.ActivityDelta{SkillsCreated: 1, ToolCalls: 4}, rows[0].ActivityDelta)
}

func TestToolEntityActivityDistinguishesCreationFromUpdate(t *testing.T) {
	db, tc, ctx := setupRWEnv(t)
	require.NoError(t, db.AutoMigrate(&storyarc.StoryArc{}, &storyarc.ArcNode{}, &timeline.TimelineEntry{},
		&reader.ReaderPerspective{}, &character.Character{}, &character.CharacterRelation{},
		&location.Location{}, &location.LocationRelation{}))
	require.NoError(t, db.Create(&chapter.Chapter{ID: 1, NovelID: tc.NovelID, Title: "章节"}).Error)
	for _, name := range []string{"甲", "乙"} {
		require.NoError(t, db.Create(&character.Character{NovelID: tc.NovelID, Name: name}).Error)
		require.NoError(t, db.Create(&location.Location{NovelID: tc.NovelID, Name: name}).Error)
	}
	reg := newTestRegistry(t)
	for _, tt := range []struct{ name, args string }{
		{"create_story_arc", `{"story_arcs":[{"name":"主线","arc_type":"main"},{"name":"支线","arc_type":"sub"}]}`},
		{"create_arc_node", `{"arc_nodes":[{"arc_id":1,"title":"转折","target_reading_number":1},{"arc_id":2,"title":"相逢","target_reading_number":2}]}`},
		{"create_timeline_entry", `{"entries":[{"category":"foreshadowing","title":"伏笔","target_reading_number":1}]}`},
		{"create_reader_perspective_entry", `{"entries":[{"type":"known","content":"身份","planted_chapter_id":1}]}`},
		{"update_character_relationship", `{"source_character_id":1,"target_character_id":2,"relation_describe":"朋友"}`},
		{"update_character_relationship", `{"source_character_id":1,"target_character_id":2,"relation_describe":"对手"}`},
		{"create_location_relation", `{"relations":[{"location_a_id":1,"location_b_id":2,"relation_type":"相邻"}]}`},
		{"place_volume", `{"name":"第一卷"}`},
		{"place_volume", `{"source_volume_id":1}`},
	} {
		res := reg.Execute(ctx, tt.name, json.RawMessage(tt.args), tc, nil)
		require.True(t, res.Success, "%s: %+v", tt.name, res)
	}
	var rows []activity.DailyActivity
	require.NoError(t, db.Find(&rows).Error)
	require.Len(t, rows, 1)
	require.Equal(t, activity.ActivityDelta{
		StoryArcsCreated: 2, ArcNodesCreated: 2, TimelineEntriesCreated: 1,
		ReaderPerspectivesCreated: 1, CharacterRelationsCreated: 1, LocationRelationsCreated: 1, VolumesCreated: 1, ToolCalls: 9,
	}, rows[0].ActivityDelta)
}
