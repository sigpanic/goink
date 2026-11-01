//go:build cgo

package mcp_tools_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/sigpanic/goink/internal/agentcfg"
	"github.com/sigpanic/goink/internal/mcp_tools"
	"github.com/sigpanic/goink/internal/storage"
	"github.com/sigpanic/goink/internal/volume"
	"github.com/stretchr/testify/require"
)

func TestVolumeToolsDiscoverReadAndRun(t *testing.T) {
	db, tc, ctx := setupRWEnv(t)
	r := newTestRegistry(t)
	allowed := agentcfg.Allowlist(agentcfg.MainAgent)
	call := func(name, raw string) *mcp_tools.ToolResult {
		return r.Execute(ctx, name, json.RawMessage(raw), tc, allowed)
	}
	for _, name := range []string{"get_volumes", "place_volume", "update_volume"} {
		found := call("search_mcp", fmt.Sprintf(`{"queries":[%q]}`, name))
		require.True(t, found.Success, found.Error)
		require.Contains(t, found.Data["content"], "`"+name+"`")
		schema := call("read", fmt.Sprintf(`{"path":"mcp/schemas/%s.json"}`, name))
		require.True(t, schema.Success, schema.Error)
		var document struct {
			Name        string          `json:"name"`
			InputSchema json.RawMessage `json:"inputSchema"`
		}
		require.NoError(t, json.Unmarshal([]byte(schema.Data["content"].(string)), &document))
		require.Equal(t, name, document.Name)
		require.NotContains(t, string(document.InputSchema), "novel_id")
		require.NotContains(t, string(document.InputSchema), "sort_order")
		for _, tool := range r.Definitions(nil) {
			require.NotEqual(t, name, tool.Name)
		}
	}
	empty := call("run_mcp", `{"tool_name":"get_volumes","args":{}}`)
	require.True(t, empty.Success, empty.Error)
	require.Contains(t, empty.Data["content"], "暂无分卷")
	foreignID := seedVolume(t, db, 2, "其他小说", 1)
	var ids []int64
	for _, name := range []string{"起点", "终点"} {
		created := call("run_mcp", fmt.Sprintf(`{"tool_name":"place_volume","args":{"name":%q}}`, name))
		require.True(t, created.Success, created.Error)
		ids = append(ids, created.Data["volume_id"].(int64))
	}
	inserted := call("run_mcp", fmt.Sprintf(`{"tool_name":"place_volume","args":{"name":"转折","before_volume_id":%d}}`, ids[1]))
	require.True(t, inserted.Success, inserted.Error)
	volumes, err := volume.NewStore(db, nil).ListByNovel(ctx, nil, tc.NovelID)
	require.NoError(t, err)
	require.Len(t, volumes, 3)
	require.Equal(t, []string{"起点", "转折", "终点"}, []string{volumes[0].Name, volumes[1].Name, volumes[2].Name})
	result := call("run_mcp", `{"tool_name":"get_volumes","args":{}}`)
	require.True(t, result.Success, result.Error)
	content := result.Data["content"].(string)
	for i, v := range volumes {
		require.Contains(t, content, fmt.Sprintf("第%d卷 %s [volume_id:%d]", i+1, v.Name, v.ID))
		require.Contains(t, content, fmt.Sprintf("volumes/id_%d.md", v.ID))
	}
	require.NotContains(t, content, "sort_order")
	require.NotContains(t, content, "其他小说")
	foreign, err := volume.NewStore(db, nil).GetByID(ctx, nil, 2, foreignID)
	require.NoError(t, err)
	require.Equal(t, 1, foreign.SortOrder)

	for _, raw := range []string{
		fmt.Sprintf(`{"source_volume_id":%d,"before_volume_id":%d}`, ids[1], ids[0]),
		fmt.Sprintf(`{"source_volume_id":%d,"before_volume_id":%d}`, ids[1], ids[1]),
		fmt.Sprintf(`{"source_volume_id":%d,"before_volume_id":null}`, ids[0]),
		fmt.Sprintf(`{"source_volume_id":%d}`, ids[0]),
	} {
		moved := call("run_mcp", `{"tool_name":"place_volume","args":`+raw+`}`)
		require.True(t, moved.Success, moved.Error)
	}
	for range 2 {
		renamed := call("run_mcp", fmt.Sprintf(`{"tool_name":"update_volume","args":{"volume_id":%d,"name":"序卷"}}`, ids[0]))
		require.True(t, renamed.Success, renamed.Error)
	}
	final, err := volume.NewStore(db, nil).ListByNovel(ctx, nil, tc.NovelID)
	require.NoError(t, err)
	require.Len(t, final, 3)
	require.Equal(t, []int64{ids[1], inserted.Data["volume_id"].(int64), ids[0]}, []int64{final[0].ID, final[1].ID, final[2].ID})
	require.Equal(t, "序卷", final[2].Name)
	require.Equal(t, volumes[0].OutlineFilePath, final[2].OutlineFilePath)
}

func TestVolumeToolsRejectInvalidChanges(t *testing.T) {
	for _, tt := range []struct {
		name string
		tool string
		args string
	}{
		{"empty", "place_volume", `{}`},
		{"null inputs", "place_volume", `{"source_volume_id":null,"name":null}`},
		{"blank name", "place_volume", `{"name":"  "}`},
		{"both inputs", "place_volume", `{"name":"新卷","source_volume_id":10}`},
		{"existing name", "place_volume", `{"name":"已有卷"}`},
		{"foreign anchor", "place_volume", `{"name":"新卷","before_volume_id":20}`},
		{"missing anchor", "place_volume", `{"name":"新卷","before_volume_id":99}`},
		{"zero anchor", "place_volume", `{"name":"新卷","before_volume_id":0}`},
		{"foreign source", "place_volume", `{"source_volume_id":20}`},
		{"missing source", "place_volume", `{"source_volume_id":99}`},
		{"zero source", "place_volume", `{"source_volume_id":0}`},
		{"foreign move anchor", "place_volume", `{"source_volume_id":10,"before_volume_id":20}`},
		{"novel injection", "place_volume", `{"name":"新卷","novel_id":2}`},
		{"batch rejected", "place_volume", `{"volumes":[{"name":"新卷"}]}`},
		{"rename duplicate", "update_volume", `{"volume_id":10,"name":"另一卷"}`},
		{"rename foreign", "update_volume", `{"volume_id":20,"name":"越权"}`},
		{"rename missing", "update_volume", `{"volume_id":99,"name":"新名"}`},
		{"rename empty", "update_volume", `{"volume_id":10,"name":""}`},
		{"rename blank", "update_volume", `{"volume_id":10,"name":"  "}`},
		{"rename missing name", "update_volume", `{"volume_id":10}`},
		{"rename missing id", "update_volume", `{"name":"新名"}`},
		{"rename zero id", "update_volume", `{"volume_id":0,"name":"新名"}`},
		{"rename novel injection", "update_volume", `{"volume_id":10,"name":"新名","novel_id":2}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db, tc, ctx := setupRWEnv(t)
			require.NoError(t, db.Create(&volume.Volume{ID: 10, NovelID: 1, Name: "已有卷", SortOrder: 1}).Error)
			require.NoError(t, db.Create(&volume.Volume{ID: 11, NovelID: 1, Name: "另一卷", SortOrder: 2}).Error)
			require.NoError(t, db.Create(&volume.Volume{ID: 20, NovelID: 2, Name: "外部卷", SortOrder: 1}).Error)
			before, err := volume.NewStore(db, nil).ListByNovel(ctx, nil, 1)
			require.NoError(t, err)
			result := newTestRegistry(t).Execute(ctx, "run_mcp", json.RawMessage(`{"tool_name":"`+tt.tool+`","args":`+tt.args+`}`), tc, nil)
			require.False(t, result.Success)
			after, err := volume.NewStore(db, nil).ListByNovel(ctx, nil, 1)
			require.NoError(t, err)
			require.Equal(t, before, after)
			foreign, err := volume.NewStore(db, nil).GetByID(ctx, nil, 2, 20)
			require.NoError(t, err)
			require.Equal(t, "外部卷", foreign.Name)
			require.Equal(t, 1, foreign.SortOrder)
		})
	}
}

func TestPlaceVolumeTransactionAndOperationLog(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprintf("fail=%t", fail), func(t *testing.T) {
			db, tc, ctx := setupRWEnv(t)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			anchor := seedVolume(t, db, tc.NovelID, "末卷", 1)
			require.NoError(t, db.AutoMigrate(&storage.OperationLogRecord{}))
			require.NoError(t, storage.RegisterOplogHooks(db))
			ctx = storage.WithTurn(ctx, "volume-test", 1)
			if fail {
				require.NoError(t, db.Exec(`CREATE TRIGGER reject_order BEFORE UPDATE OF sort_order ON volumes WHEN NEW.sort_order = 2 BEGIN SELECT RAISE(ABORT, 'test failure'); END`).Error)
			}
			result := newTestRegistry(t).Execute(ctx, "run_mcp", json.RawMessage(fmt.Sprintf(`{"tool_name":"place_volume","args":{"name":"第一卷","before_volume_id":%d}}`, anchor)), tc, nil)
			volumes, err := volume.NewStore(db, nil).ListByNovel(ctx, nil, tc.NovelID)
			require.NoError(t, err)
			var logs []storage.OperationLogRecord
			require.NoError(t, db.Where("session_id = ?", "volume-test").Find(&logs).Error)
			if fail {
				require.False(t, result.Success)
				require.Contains(t, result.Error, "放置分卷失败")
				require.Len(t, volumes, 1)
				require.Equal(t, 1, volumes[0].SortOrder)
				require.Empty(t, logs)
				return
			}
			require.True(t, result.Success, result.Error)
			require.Len(t, volumes, 2)
			var createdIDs []int64
			for _, log := range logs {
				if log.Operation == "create" && log.Table == "volumes" {
					var key struct {
						ID int64 `json:"id"`
					}
					require.NoError(t, json.Unmarshal([]byte(log.EntityID), &key))
					createdIDs = append(createdIDs, key.ID)
				}
			}
			require.Equal(t, []int64{result.Data["volume_id"].(int64)}, createdIDs)
		})
	}
}
