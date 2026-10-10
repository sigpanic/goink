package mcp_tools_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/character"
	"github.com/sigpanic/goink/internal/mcp_tools"
)

func characterQueryFixture(t *testing.T) (*mcp_tools.Registry, mcp_tools.ToolContext) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&character.Character{}, &character.Group{}, &character.GroupMember{}))
	for _, ch := range []character.Character{
		{ID: 1, NovelID: 1, Name: "张三", Personality: `{"traits":["勇敢"]}`, Abilities: `["剑术"]`},
		{ID: 2, NovelID: 1, Name: "李四"},
		{ID: 3, NovelID: 1, Name: "王五"},
		{ID: 4, NovelID: 2, Name: "其他小说角色"},
	} {
		ch.UpdatedAt = time.Unix(ch.ID, 0)
		require.NoError(t, db.Create(&ch).Error)
	}
	for _, group := range []character.Group{
		{ID: 10, NovelID: 1, Name: "A 主角团", Description: "长期同行的伙伴"},
		{ID: 20, NovelID: 1, Name: "B 青云宗", Description: "修行宗门"},
		{ID: 30, NovelID: 1, Name: "C 空组", Description: "预留分类"},
		{ID: 40, NovelID: 2, Name: "其他小说分组", Description: "不应泄露"},
	} {
		require.NoError(t, db.Create(&group).Error)
	}
	for _, member := range []character.GroupMember{
		{NovelID: 1, CharacterID: 1, GroupID: 10},
		{NovelID: 1, CharacterID: 1, GroupID: 20},
		{NovelID: 1, CharacterID: 2, GroupID: 20},
		{NovelID: 2, CharacterID: 4, GroupID: 40},
	} {
		require.NoError(t, db.Create(&member).Error)
	}
	return newTestRegistry(t), mcp_tools.ToolContext{DB: db, NovelID: 1}
}

func queryCharacters(t *testing.T, reg *mcp_tools.Registry, tc mcp_tools.ToolContext, args string) map[string]any {
	t.Helper()
	result := reg.Execute(context.Background(), "get_characters", json.RawMessage(args), tc, nil)
	require.True(t, result.Success, "%+v", result)
	return result.Data
}

func TestGetCharactersGroupsAndFilters(t *testing.T) {
	reg, tc := characterQueryFixture(t)
	for _, tt := range []struct {
		name  string
		args  string
		ids   []int64
		total int64
	}{
		{"default", `{}`, []int64{3, 2, 1}, 3},
		{"explicit mode", `{"mode":"characters"}`, []int64{3, 2, 1}, 3},
		{"group", `{"group_id":20}`, []int64{2, 1}, 2},
		{"ungrouped", `{"group_id":0}`, []int64{3}, 1},
		{"search within group", `{"group_id":20,"search":"张"}`, []int64{1}, 1},
		{"group page", `{"group_id":20,"page":2,"size":1}`, []int64{1}, 2},
		{"empty group", `{"group_id":30}`, []int64{}, 0},
		{"foreign group", `{"group_id":40}`, []int64{}, 0},
		{"missing group", `{"group_id":999}`, []int64{}, 0},
		{"past last page", `{"page":10,"size":1}`, []int64{}, 3},
	} {
		t.Run(tt.name, func(t *testing.T) {
			data := queryCharacters(t, reg, tc, tt.args)
			require.Equal(t, tt.total, data["total"])
			items := data["characters"].([]map[string]any)
			ids := make([]int64, 0, len(items))
			for _, item := range items {
				id := item["id"].(int64)
				ids = append(ids, id)
				switch id {
				case 1:
					require.Equal(t, []string{"A 主角团 [group_id:10]", "B 青云宗 [group_id:20]"}, item["groups"])
					require.Equal(t, map[string]any{"traits": []any{"勇敢"}}, item["personality"])
					require.Equal(t, []any{"剑术"}, item["abilities"])
				case 2:
					require.Equal(t, []string{"B 青云宗 [group_id:20]"}, item["groups"])
				case 3:
					require.Equal(t, []string{}, item["groups"])
				}
			}
			require.Equal(t, tt.ids, ids)
		})
	}
}

func TestGetCharactersGroupMode(t *testing.T) {
	reg, tc := characterQueryFixture(t)
	data := queryCharacters(t, reg, tc, `{"mode":"groups"}`)
	require.Equal(t, int64(3), data["total"])
	require.Equal(t, 1, data["page"])
	require.Equal(t, 50, data["size"])
	require.NotContains(t, data, "characters")
	require.Equal(t, "- A 主角团 [group_id:10]（1 名角色）：长期同行的伙伴\n- B 青云宗 [group_id:20]（2 名角色）：修行宗门\n- C 空组 [group_id:30]（0 名角色）：预留分类", data["content"])

	for _, search := range []string{"青云", "修行"} {
		args, err := json.Marshal(map[string]any{"mode": "groups", "search": search})
		require.NoError(t, err)
		data = queryCharacters(t, reg, tc, string(args))
		require.Equal(t, int64(1), data["total"])
		require.Contains(t, data["content"], "[group_id:20]")
	}
	data = queryCharacters(t, reg, tc, `{"mode":"groups","page":2,"size":1}`)
	require.Equal(t, int64(3), data["total"])
	require.Equal(t, 2, data["page"])
	require.Equal(t, 3, data["total_pages"])
	require.Equal(t, "- B 青云宗 [group_id:20]（2 名角色）：修行宗门", data["content"])
	data = queryCharacters(t, reg, tc, `{"mode":"groups","search":"不存在"}`)
	require.Equal(t, int64(0), data["total"])
	require.Equal(t, "本页无匹配的角色分组。", data["content"])
	data = queryCharacters(t, reg, tc, `{"mode":"groups","size":100}`)
	require.Equal(t, 100, data["size"])

	tc.NovelID = 2
	data = queryCharacters(t, reg, tc, `{"mode":"groups"}`)
	require.Equal(t, int64(1), data["total"])
	require.Equal(t, "- 其他小说分组 [group_id:40]（1 名角色）：不应泄露", data["content"])
}

func TestGetCharactersRejectsInvalidGroupParameters(t *testing.T) {
	reg := newTestRegistry(t)
	for _, args := range []string{
		`{"mode":"unknown"}`,
		`{"group_id":-1}`,
		`{"mode":"groups","group_id":0}`,
		`{"mode":"groups","group_id":10}`,
	} {
		t.Run(args, func(t *testing.T) {
			result := reg.Execute(context.Background(), "get_characters", json.RawMessage(args), mcp_tools.ToolContext{}, nil)
			require.False(t, result.Success)
			require.NotEmpty(t, result.Error)
			require.NotEqual(t, mcp_tools.ErrKindSystem, result.ErrKind)
		})
	}
}
