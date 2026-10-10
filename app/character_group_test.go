package app

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/character"
	"github.com/sigpanic/goink/internal/mcp_tools"
	"github.com/sigpanic/goink/internal/rollback"
	"github.com/sigpanic/goink/internal/session"
	"github.com/sigpanic/goink/internal/storage"
)

func TestCharacterGroupAPI(t *testing.T) {
	a := setupTestApp(t)
	n := createTestNovel(t, a)
	groups, err := a.GetCharacterGroups(GetCharacterGroupsInput{ListInput: ListInput{NovelID: n.ID, PageParams: storage.PageParams{Size: -1}}})
	require.NoError(t, err)
	require.Empty(t, groups.Items)
	g, err := a.CreateCharacterGroup(n.ID, CharacterGroupInput{Name: "主角团", Description: "长期同行的伙伴"})
	require.NoError(t, err)
	require.Equal(t, "长期同行的伙伴", g.Description)
	c, err := a.CreateCharacter(n.ID, CreateCharacterInput{Name: "张三"})
	require.NoError(t, err)
	require.NoError(t, a.UpdateCharacterGroupMemberships(n.ID, CharacterGroupMembershipInput{
		CharacterIDs: []int64{c.ID}, AddGroupIDs: []int64{g.ID},
	}))
	groups, err = a.GetCharacterGroups(GetCharacterGroupsInput{ListInput: ListInput{NovelID: n.ID, PageParams: storage.PageParams{Size: -1}}})
	require.NoError(t, err)
	require.Len(t, groups.Items, 1)
	require.EqualValues(t, 1, groups.Items[0].MemberCount)
	require.NoError(t, a.UpdateCharacterGroup(n.ID, g.ID, CharacterGroupInput{Name: "核心角色", Description: "推动主线的角色"}))
	groups, err = a.GetCharacterGroups(GetCharacterGroupsInput{ListInput: ListInput{NovelID: n.ID, PageParams: storage.PageParams{Size: -1}}})
	require.NoError(t, err)
	require.Equal(t, "核心角色", groups.Items[0].Name)
	require.Equal(t, "推动主线的角色", groups.Items[0].Description)
	require.NoError(t, a.DeleteCharacterGroup(n.ID, g.ID))
	members, err := a.GetCharacterGroupMembers(GetCharacterGroupMembersInput{ListInput: ListInput{NovelID: n.ID, PageParams: storage.PageParams{Size: -1}}})
	require.NoError(t, err)
	require.Empty(t, members.Items)
	chars, err := a.GetCharacters(n.ID)
	require.NoError(t, err)
	require.Len(t, chars, 1)
}

func TestCharacterGroupDeletionEntrypoints(t *testing.T) {
	for _, entry := range []string{"frontend", "mcp", "novel"} {
		t.Run(entry, func(t *testing.T) {
			a := setupTestApp(t)
			n := createTestNovel(t, a)
			other := createTestNovel(t, a)
			g, err := a.CreateCharacterGroup(n.ID, CharacterGroupInput{Name: "主角团"})
			require.NoError(t, err)
			otherGroup, err := a.CreateCharacterGroup(other.ID, CharacterGroupInput{Name: "主角团"})
			require.NoError(t, err)
			otherCharacter, err := a.CreateCharacter(other.ID, CreateCharacterInput{Name: "李四"})
			require.NoError(t, err)
			require.NoError(t, a.UpdateCharacterGroupMemberships(other.ID, CharacterGroupMembershipInput{CharacterIDs: []int64{otherCharacter.ID}, AddGroupIDs: []int64{otherGroup.ID}}))
			c, err := a.CreateCharacter(n.ID, CreateCharacterInput{Name: "张三"})
			require.NoError(t, err)
			require.NoError(t, a.UpdateCharacterGroupMemberships(n.ID, CharacterGroupMembershipInput{CharacterIDs: []int64{c.ID}, AddGroupIDs: []int64{g.ID}}))
			switch entry {
			case "frontend":
				require.NoError(t, a.DeleteCharacter(n.ID, c.ID))
			case "mcp":
				tool := mcp_tools.DeleteRecordTool{}
				result, err := tool.Execute(a.ctx, &mcp_tools.DeleteRecordArgs{Table: "character", ID: c.ID}, mcp_tools.ToolContext{DB: a.db, NovelID: n.ID})
				require.NoError(t, err)
				require.True(t, result.Success, result.Error)
			case "novel":
				secondGroup, err := a.CreateCharacterGroup(n.ID, CharacterGroupInput{Name: "宗门"})
				require.NoError(t, err)
				secondCharacter, err := a.CreateCharacter(n.ID, CreateCharacterInput{Name: "王五"})
				require.NoError(t, err)
				require.NoError(t, a.UpdateCharacterGroupMemberships(n.ID, CharacterGroupMembershipInput{CharacterIDs: []int64{c.ID, secondCharacter.ID}, AddGroupIDs: []int64{g.ID, secondGroup.ID}}))
				require.NoError(t, a.DeleteNovel(n.ID))
			}
			members, err := a.GetCharacterGroupMembers(GetCharacterGroupMembersInput{ListInput: ListInput{NovelID: n.ID, PageParams: storage.PageParams{Size: -1}}})
			require.NoError(t, err)
			require.Empty(t, members.Items)
			groups, err := a.GetCharacterGroups(GetCharacterGroupsInput{ListInput: ListInput{NovelID: n.ID, PageParams: storage.PageParams{Size: -1}}})
			require.NoError(t, err)
			if entry == "novel" {
				require.Empty(t, groups.Items)
			} else {
				require.Len(t, groups.Items, 1)
				require.Zero(t, groups.Items[0].MemberCount)
			}
			require.NoError(t, a.db.First(&character.Group{}, otherGroup.ID).Error)
			otherMembers, err := a.GetCharacterGroupMembers(GetCharacterGroupMembersInput{ListInput: ListInput{NovelID: other.ID, PageParams: storage.PageParams{Size: -1}}})
			require.NoError(t, err)
			require.Len(t, otherMembers.Items, 1)
			require.Equal(t, otherGroup.ID, otherMembers.Items[0].GroupID)
			require.Equal(t, otherCharacter.ID, otherMembers.Items[0].CharacterID)
		})
	}
}

func TestCharacterGroupRollbackCleansManualMemberships(t *testing.T) {
	for _, parent := range []string{"character", "group"} {
		t.Run(parent, func(t *testing.T) {
			a := setupTestApp(t)
			n := createTestNovel(t, a)
			require.NoError(t, a.db.Create(&session.Session{SessionID: "test", NovelID: n.ID, LastTurnID: 1}).Error)
			turn := storage.WithTurn(a.ctx, "test", 1)
			groupCtx, charCtx := a.ctx, a.ctx
			if parent == "group" {
				groupCtx = turn
			} else {
				charCtx = turn
			}
			g, err := a.character.CreateGroup(groupCtx, n.ID, "主角团", "同伴")
			require.NoError(t, err)
			c := character.Character{NovelID: n.ID, Name: "张三"}
			require.NoError(t, a.db.WithContext(charCtx).Create(&c).Error)
			require.NoError(t, a.UpdateCharacterGroupMemberships(n.ID, CharacterGroupMembershipInput{CharacterIDs: []int64{c.ID}, AddGroupIDs: []int64{g.ID}}))
			require.NoError(t, rollback.RollbackBeforeTurn(context.Background(), a.db, "test", 1, nil, a.turnCommit))
			members, err := a.GetCharacterGroupMembers(GetCharacterGroupMembersInput{ListInput: ListInput{NovelID: n.ID, PageParams: storage.PageParams{Size: -1}}})
			require.NoError(t, err)
			require.Empty(t, members.Items)
			if parent == "character" {
				require.ErrorIs(t, a.db.First(&character.Character{}, c.ID).Error, gorm.ErrRecordNotFound)
				require.NoError(t, a.db.First(&character.Group{}, g.ID).Error)
			} else {
				require.ErrorIs(t, a.db.First(&character.Group{}, g.ID).Error, gorm.ErrRecordNotFound)
				require.NoError(t, a.db.First(&character.Character{}, c.ID).Error)
			}
		})
	}
}

func TestManualGroupDeleteInheritsGroupTurn(t *testing.T) {
	a := setupTestApp(t)
	n := createTestNovel(t, a)
	g, err := a.CreateCharacterGroup(n.ID, CharacterGroupInput{Name: "主角团", Description: "同伴"})
	require.NoError(t, err)
	c, err := a.CreateCharacter(n.ID, CreateCharacterInput{Name: "张三"})
	require.NoError(t, err)
	require.NoError(t, a.UpdateCharacterGroupMemberships(n.ID, CharacterGroupMembershipInput{CharacterIDs: []int64{c.ID}, AddGroupIDs: []int64{g.ID}}))
	require.NoError(t, a.db.Create(&session.Session{SessionID: "test", NovelID: n.ID, LastTurnID: 1}).Error)
	require.NoError(t, a.character.UpdateGroup(storage.WithTurn(a.ctx, "test", 1), n.ID, g.ID, "新组名", "新描述"))
	require.NoError(t, a.DeleteCharacterGroup(n.ID, g.ID))
	require.NoError(t, rollback.RollbackBeforeTurn(a.ctx, a.db, "test", 1, nil, a.turnCommit))
	groups, err := a.GetCharacterGroups(GetCharacterGroupsInput{ListInput: ListInput{NovelID: n.ID, PageParams: storage.PageParams{Size: -1}}})
	require.NoError(t, err)
	require.Len(t, groups.Items, 1)
	require.Equal(t, "主角团", groups.Items[0].Name)
	require.Equal(t, "同伴", groups.Items[0].Description)
	require.EqualValues(t, 1, groups.Items[0].MemberCount)
}

func TestGroupRollbackCleanupIsScopedToNovel(t *testing.T) {
	a := setupTestApp(t)
	n := createTestNovel(t, a)
	other := createTestNovel(t, a)
	require.NoError(t, a.db.Create(&session.Session{SessionID: "test", NovelID: n.ID, LastTurnID: 1}).Error)
	orphan := character.GroupMember{NovelID: other.ID, GroupID: 999, CharacterID: 999}
	require.NoError(t, a.db.Create(&orphan).Error)
	require.NoError(t, rollback.RollbackBeforeTurn(a.ctx, a.db, "test", 1, nil, a.turnCommit))
	require.NoError(t, a.db.First(&character.GroupMember{}, orphan.ID).Error)
}

func TestCharacterGroupQueriesMapListInput(t *testing.T) {
	a := setupTestApp(t)
	n := createTestNovel(t, a)
	g1, err := a.CreateCharacterGroup(n.ID, CharacterGroupInput{Name: "主角团"})
	require.NoError(t, err)
	g2, err := a.CreateCharacterGroup(n.ID, CharacterGroupInput{Name: "青云宗", Description: "主角所属宗门"})
	require.NoError(t, err)
	_, err = a.CreateCharacterGroup(n.ID, CharacterGroupInput{Name: "路人"})
	require.NoError(t, err)
	groups, err := a.GetCharacterGroups(GetCharacterGroupsInput{
		ListInput: ListInput{NovelID: n.ID, PageParams: storage.PageParams{Page: 2, Size: 1}, Search: "主角"},
	})
	require.NoError(t, err)
	require.EqualValues(t, 2, groups.Total)
	require.Equal(t, 2, groups.Page)
	require.Equal(t, 1, groups.Size)
	require.Equal(t, 2, groups.TotalPages)
	require.Len(t, groups.Items, 1)
	require.Equal(t, g2.ID, groups.Items[0].ID)
	c, err := a.CreateCharacter(n.ID, CreateCharacterInput{Name: "张三"})
	require.NoError(t, err)
	require.NoError(t, a.UpdateCharacterGroupMemberships(n.ID, CharacterGroupMembershipInput{CharacterIDs: []int64{c.ID}, AddGroupIDs: []int64{g1.ID, g2.ID}}))
	members, err := a.GetCharacterGroupMembers(GetCharacterGroupMembersInput{
		ListInput:   ListInput{NovelID: n.ID, PageParams: storage.PageParams{Size: 1}, Search: "张"},
		CharacterID: c.ID,
	})
	require.NoError(t, err)
	require.EqualValues(t, 2, members.Total)
	require.Equal(t, 2, members.TotalPages)
	require.Len(t, members.Items, 1)
	require.Equal(t, g1.ID, members.Items[0].GroupID)
	members, err = a.GetCharacterGroupMembers(GetCharacterGroupMembersInput{
		ListInput: ListInput{NovelID: n.ID, PageParams: storage.PageParams{Size: 1}, Search: "张"},
		GroupID:   g1.ID,
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, members.Total)
	require.Len(t, members.Items, 1)
	require.Equal(t, g1.ID, members.Items[0].GroupID)
}

func TestCharacterGroupListInputJSON(t *testing.T) {
	const common = `{"novel_id":7,"page":2,"size":10,"search":"主角"}`
	var groups GetCharacterGroupsInput
	require.NoError(t, json.Unmarshal([]byte(common), &groups))
	require.EqualValues(t, 7, groups.NovelID)
	require.Equal(t, storage.PageParams{Page: 2, Size: 10}, groups.PageParams)
	require.Equal(t, "主角", groups.Search)
	data, err := json.Marshal(groups)
	require.NoError(t, err)
	require.JSONEq(t, common, string(data))

	const filtered = `{"novel_id":7,"page":2,"size":10,"search":"张","group_id":3,"character_id":4}`
	var members GetCharacterGroupMembersInput
	require.NoError(t, json.Unmarshal([]byte(filtered), &members))
	require.EqualValues(t, 7, members.NovelID)
	require.Equal(t, storage.PageParams{Page: 2, Size: 10}, members.PageParams)
	require.Equal(t, "张", members.Search)
	require.EqualValues(t, 3, members.GroupID)
	require.EqualValues(t, 4, members.CharacterID)
	data, err = json.Marshal(members)
	require.NoError(t, err)
	require.JSONEq(t, filtered, string(data))
}
