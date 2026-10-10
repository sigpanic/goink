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
	"github.com/sigpanic/goink/internal/character"
	"github.com/sigpanic/goink/internal/config"
	"github.com/sigpanic/goink/internal/location"
	"github.com/sigpanic/goink/internal/session"
	"github.com/sigpanic/goink/internal/storage"
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
