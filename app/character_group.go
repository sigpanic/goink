package app

import (
	"github.com/sigpanic/goink/internal/activity"
	"github.com/sigpanic/goink/internal/character"
	"github.com/sigpanic/goink/internal/storage"
)

// CharacterGroupInput 是分组创建/更新参数；更新采用 PUT 语义。
type CharacterGroupInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type CharacterGroupMembershipInput struct {
	CharacterIDs   []int64 `json:"character_ids"`
	AddGroupIDs    []int64 `json:"add_group_ids"`
	RemoveGroupIDs []int64 `json:"remove_group_ids"`
}

type GetCharacterGroupsInput struct {
	ListInput
}

type GetCharacterGroupMembersInput struct {
	ListInput
	GroupID     int64 `json:"group_id"`
	CharacterID int64 `json:"character_id"`
}

func (a *App) GetCharacterGroups(input GetCharacterGroupsInput) (*storage.PageResult[character.GroupView], error) {
	return a.character.ListGroups(a.ctx, input.NovelID, character.ListGroupsOptions{
		PageParams: input.PageParams,
		Search:     input.Search,
		Order:      "name ASC, id ASC",
	})
}

func (a *App) GetCharacterGroupMembers(input GetCharacterGroupMembersInput) (*storage.PageResult[character.GroupMember], error) {
	return a.character.ListGroupMembers(a.ctx, input.NovelID, character.ListGroupMembersOptions{
		PageParams:  input.PageParams,
		GroupID:     input.GroupID,
		CharacterID: input.CharacterID,
		Search:      input.Search,
		Order:       "group_id ASC, character_id ASC",
	})
}

func (a *App) CreateCharacterGroup(novelID int64, input CharacterGroupInput) (*character.Group, error) {
	group, err := a.character.CreateGroup(a.ctx, novelID, input.Name, input.Description)
	if err == nil && a.activity != nil {
		a.activity.AddActivity(a.ctx, activity.ActivityDelta{CharacterGroupsCreated: 1})
	}
	return group, err
}

func (a *App) UpdateCharacterGroup(novelID, groupID int64, input CharacterGroupInput) error {
	return a.character.UpdateGroup(a.ctx, novelID, groupID, input.Name, input.Description)
}

func (a *App) DeleteCharacterGroup(novelID, groupID int64) error {
	return a.character.DeleteGroup(a.ctx, novelID, groupID)
}

func (a *App) UpdateCharacterGroupMemberships(novelID int64, input CharacterGroupMembershipInput) error {
	return a.character.UpdateGroupMemberships(a.ctx, novelID, input.CharacterIDs, input.AddGroupIDs, input.RemoveGroupIDs)
}
