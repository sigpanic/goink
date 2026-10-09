package character

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/storage"
)

var (
	ErrGroupNotFound     = errors.New("分组不存在或不属于当前小说")
	ErrGroupNameTaken    = errors.New("当前小说已存在同名分组")
	ErrCharacterNotFound = errors.New("角色不存在或不属于当前小说")
)

// ListGroupsOptions 是 ListGroups 的可选参数。
type ListGroupsOptions struct {
	PageParams storage.PageParams
	Search     string // 空字符串=不过滤，按 name LIKE OR description LIKE 模糊匹配
	Order      string // 空字符串=默认 name ASC, id ASC；调用方可显式指定排序
}

// ListGroups 分页列出某小说的分组及成员数量，支持名称/描述搜索。
func (s *Store) ListGroups(ctx context.Context, novelID int64, opts ListGroupsOptions) (*storage.PageResult[GroupView], error) {
	pp := opts.PageParams
	pp.Normalize()

	q := s.DB.WithContext(ctx).Model(&Group{}).Where("novel_id = ?", novelID)
	if opts.Search != "" {
		q = q.Where("name LIKE ? OR description LIKE ?", "%"+opts.Search+"%", "%"+opts.Search+"%")
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("character groups: count: %w", err)
	}

	order := opts.Order
	if order == "" {
		order = "name ASC, id ASC"
	}
	var groups []GroupView
	if err := q.Select("character_groups.*, (SELECT COUNT(*) FROM character_group_members m WHERE m.group_id = character_groups.id AND m.novel_id = character_groups.novel_id) AS member_count").
		Order(order).Offset(pp.Offset()).Limit(pp.Size).Scan(&groups).Error; err != nil {
		return nil, fmt.Errorf("character groups: list: %w", err)
	}

	s.logger.Debug("character groups: listed", "novel_id", novelID, "total", total, "page", pp.Page)
	return storage.NewPageResult(groups, total, pp.Page, pp.Size), nil
}

// ListGroupMembersOptions 是 ListGroupMembers 的可选参数。
type ListGroupMembersOptions struct {
	PageParams  storage.PageParams
	GroupID     int64  // 0=不过滤，正整数=指定分组
	CharacterID int64  // 0=不过滤，正整数=指定角色
	Search      string // 空字符串=不过滤，按角色 name LIKE 模糊匹配
	Order       string // 空字符串=默认 group_id ASC, character_id ASC；按成员表字段排序
}

// ListGroupMembers 分页列出某小说的成员归属，支持分组/角色过滤和角色名搜索。
func (s *Store) ListGroupMembers(ctx context.Context, novelID int64, opts ListGroupMembersOptions) (*storage.PageResult[GroupMember], error) {
	pp := opts.PageParams
	pp.Normalize()

	q := s.DB.WithContext(ctx).Model(&GroupMember{}).Where("novel_id = ?", novelID)
	if opts.GroupID > 0 {
		q = q.Where("group_id = ?", opts.GroupID)
	}
	if opts.CharacterID > 0 {
		q = q.Where("character_id = ?", opts.CharacterID)
	}
	if opts.Search != "" {
		characters := s.DB.WithContext(ctx).Model(&Character{}).
			Select("id").Where("novel_id = ? AND name LIKE ?", novelID, "%"+opts.Search+"%")
		q = q.Where("character_id IN (?)", characters)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("character groups: count members: %w", err)
	}

	order := opts.Order
	if order == "" {
		order = "group_id ASC, character_id ASC"
	}
	var members []GroupMember
	if err := q.Order(order).Offset(pp.Offset()).Limit(pp.Size).Find(&members).Error; err != nil {
		return nil, fmt.Errorf("character groups: list members: %w", err)
	}

	s.logger.Debug("character groups: listed members", "novel_id", novelID, "total", total, "page", pp.Page)
	return storage.NewPageResult(members, total, pp.Page, pp.Size), nil
}

func validateGroupName(tx *gorm.DB, novelID, groupID int64, name string) error {
	if name == "" {
		return errors.New("分组名称不能为空")
	}
	var count int64
	if err := tx.Model(&Group{}).Where("novel_id = ? AND name = ? AND id <> ?", novelID, name, groupID).Count(&count).Error; err != nil {
		return fmt.Errorf("character groups: check name: %w", err)
	}
	if count > 0 {
		return ErrGroupNameTaken
	}
	return nil
}

func findGroup(tx *gorm.DB, novelID, groupID int64) (*Group, error) {
	var group Group
	if err := tx.Where("id = ? AND novel_id = ?", groupID, novelID).First(&group).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrGroupNotFound
		}
		return nil, fmt.Errorf("character groups: get: %w", err)
	}
	return &group, nil
}

func (s *Store) CreateGroup(ctx context.Context, novelID int64, name, description string) (*Group, error) {
	group := Group{NovelID: novelID, Name: strings.TrimSpace(name), Description: description}
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Table("novels").Where("id = ?", novelID).Count(&count).Error; err != nil {
			return fmt.Errorf("character groups: check novel: %w", err)
		}
		if count == 0 {
			return errors.New("小说不存在")
		}
		if err := validateGroupName(tx, novelID, 0, group.Name); err != nil {
			return err
		}
		if err := tx.Create(&group).Error; err != nil {
			return fmt.Errorf("character groups: create: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &group, nil
}

// UpdateGroup 使用 PUT 语义，允许清空描述。
func (s *Store) UpdateGroup(ctx context.Context, novelID, groupID int64, name, description string) error {
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		group, err := findGroup(tx, novelID, groupID)
		if err != nil {
			return err
		}
		name = strings.TrimSpace(name)
		if err := validateGroupName(tx, novelID, groupID, name); err != nil {
			return err
		}
		if group.Name == name && group.Description == description {
			return nil
		}
		group.Name, group.Description = name, description
		if err := tx.Save(group).Error; err != nil {
			return fmt.Errorf("character groups: update: %w", err)
		}
		return nil
	})
}

func deleteGroupMembers(tx *gorm.DB) error {
	var members []GroupMember
	if err := tx.Find(&members).Error; err != nil {
		return fmt.Errorf("character groups: query members for deletion: %w", err)
	}
	for i := range members {
		if err := tx.Session(&gorm.Session{NewDB: true}).Delete(&members[i]).Error; err != nil {
			return fmt.Errorf("character groups: delete member: %w", err)
		}
	}
	return nil
}

// DeleteGroup 仅解除归属，角色和其他组的归属保持不变。
func (s *Store) DeleteGroup(ctx context.Context, novelID, groupID int64) error {
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		group, err := findGroup(tx, novelID, groupID)
		if err != nil {
			return err
		}
		opCtx, err := storage.WithEntityTurn(ctx, tx, group.TableName(), group.ID)
		if err != nil {
			return err
		}
		tx = tx.WithContext(opCtx)
		if err := deleteGroupMembers(tx.Where("novel_id = ? AND group_id = ?", novelID, groupID)); err != nil {
			return err
		}
		if err := tx.Delete(group).Error; err != nil {
			return fmt.Errorf("character groups: delete: %w", err)
		}
		return nil
	})
}

func uniqueIDs(ids []int64) ([]int64, error) {
	result := make([]int64, 0, len(ids))
	seen := make(map[int64]bool, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return nil, errors.New("ID 必须为正整数")
		}
		if !seen[id] {
			seen[id] = true
			result = append(result, id)
		}
	}
	return result, nil
}

// UpdateGroupMemberships 对多个角色增量调整归属，全部校验后在同一事务内逐行写入。
// 重复加入或移出视为无操作；不会覆盖调用方没有指定的其他归属。
func (s *Store) UpdateGroupMemberships(ctx context.Context, novelID int64, characterIDs, addGroupIDs, removeGroupIDs []int64) error {
	characters, err := uniqueIDs(characterIDs)
	if err != nil {
		return err
	}
	adds, err := uniqueIDs(addGroupIDs)
	if err != nil {
		return err
	}
	removes, err := uniqueIDs(removeGroupIDs)
	if err != nil {
		return err
	}
	if len(characters) == 0 || len(adds)+len(removes) == 0 {
		return errors.New("必须指定角色和要加入或移出的分组")
	}
	removeSet := make(map[int64]bool, len(removes))
	for _, id := range removes {
		removeSet[id] = true
	}
	for _, id := range adds {
		if removeSet[id] {
			return errors.New("同一分组不能同时加入和移出")
		}
	}
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&Character{}).Where("novel_id = ? AND id IN ?", novelID, characters).Count(&count).Error; err != nil {
			return fmt.Errorf("character groups: check characters: %w", err)
		}
		if count != int64(len(characters)) {
			return ErrCharacterNotFound
		}
		groups := append(append([]int64{}, adds...), removes...)
		if err := tx.Model(&Group{}).Where("novel_id = ? AND id IN ?", novelID, groups).Count(&count).Error; err != nil {
			return fmt.Errorf("character groups: check groups: %w", err)
		}
		if count != int64(len(groups)) {
			return ErrGroupNotFound
		}
		var members []GroupMember
		if err := tx.Where("novel_id = ? AND character_id IN ? AND group_id IN ?", novelID, characters, groups).Find(&members).Error; err != nil {
			return fmt.Errorf("character groups: query memberships: %w", err)
		}
		existing := make(map[[2]int64]GroupMember, len(members))
		for _, member := range members {
			existing[[2]int64{member.CharacterID, member.GroupID}] = member
		}
		for _, characterID := range characters {
			opCtx, err := storage.WithEntityTurn(ctx, tx, "characters", characterID)
			if err != nil {
				return err
			}
			op := tx.WithContext(opCtx)
			for _, groupID := range removes {
				if member, ok := existing[[2]int64{characterID, groupID}]; ok {
					if err := op.Delete(&member).Error; err != nil {
						return fmt.Errorf("character groups: remove membership: %w", err)
					}
				}
			}
			for _, groupID := range adds {
				if _, ok := existing[[2]int64{characterID, groupID}]; ok {
					continue
				}
				member := GroupMember{NovelID: novelID, GroupID: groupID, CharacterID: characterID}
				if err := op.Create(&member).Error; err != nil {
					return fmt.Errorf("character groups: add membership: %w", err)
				}
			}
		}
		return nil
	})
}

// DeleteCharacter 逐行删除附属记录，保证删除及回退包含分组归属和关系历史。
func (s *Store) DeleteCharacter(ctx context.Context, novelID, characterID int64) error {
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var ch Character
		if err := tx.Where("novel_id = ? AND id = ?", novelID, characterID).First(&ch).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrCharacterNotFound
			}
			return fmt.Errorf("character store: get for deletion: %w", err)
		}
		opCtx, err := storage.WithEntityTurn(ctx, tx, ch.TableName(), ch.ID)
		if err != nil {
			return err
		}
		tx = tx.WithContext(opCtx)
		if err := deleteGroupMembers(tx.Where("novel_id = ? AND character_id = ?", novelID, characterID)); err != nil {
			return err
		}
		var relations []CharacterRelation
		if err := tx.Where("novel_id = ? AND (source_character_id = ? OR target_character_id = ?)", novelID, characterID, characterID).Find(&relations).Error; err != nil {
			return fmt.Errorf("character store: query relations for deletion: %w", err)
		}
		for i := range relations {
			if err := tx.Delete(&relations[i]).Error; err != nil {
				return fmt.Errorf("character store: delete relation: %w", err)
			}
		}
		if err := tx.Delete(&ch).Error; err != nil {
			return fmt.Errorf("character store: delete: %w", err)
		}
		return nil
	})
}

// CleanupOrphanGroupMembers 在回退事务末尾移除失去角色或分组的归属。
// 手动归属可能不属于被回退的 AI 轮次；此处不生成新日志，避免将回退记成用户操作。
func CleanupOrphanGroupMembers(tx *gorm.DB, novelID int64) error {
	return tx.Exec(`DELETE FROM character_group_members
		WHERE novel_id = ? AND (
		NOT EXISTS (SELECT 1 FROM characters c WHERE c.id = character_group_members.character_id AND c.novel_id = character_group_members.novel_id)
		OR NOT EXISTS (SELECT 1 FROM character_groups g WHERE g.id = character_group_members.group_id AND g.novel_id = character_group_members.novel_id))`, novelID).Error
}
