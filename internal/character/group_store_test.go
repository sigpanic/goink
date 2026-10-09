package character

import (
	"context"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/novel"
	"github.com/sigpanic/goink/internal/session"
	"github.com/sigpanic/goink/internal/storage"
)

func groupStoreFixture(t *testing.T) (*Store, Character, Character, *Group, *Group) {
	t.Helper()
	db := openCharDB(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&novel.Novel{}, &Group{}, &GroupMember{}, &storage.OperationLogRecord{}, &session.Session{}, &session.Message{}))
	require.NoError(t, storage.RegisterOplogHooks(db))
	for _, id := range []int64{1, 2} {
		require.NoError(t, db.Create(&novel.Novel{ID: id, Title: "小说"}).Error)
	}
	s := NewStore(db, testCharLogger())
	c1, c2 := Character{NovelID: 1, Name: "张三"}, Character{NovelID: 1, Name: "李四"}
	require.NoError(t, db.Create(&c1).Error)
	require.NoError(t, db.Create(&c2).Error)
	g1, err := s.CreateGroup(context.Background(), 1, "主角团", "长期同行的伙伴")
	require.NoError(t, err)
	g2, err := s.CreateGroup(context.Background(), 1, "青云宗", "宗门成员")
	require.NoError(t, err)
	return s, c1, c2, g1, g2
}

func TestGroupsMetadataAndNovelIsolation(t *testing.T) {
	s, _, _, g1, _ := groupStoreFixture(t)
	ctx := context.Background()
	_, err := s.CreateGroup(ctx, 1, " 主角团 ", "重复")
	require.ErrorIs(t, err, ErrGroupNameTaken)
	_, err = s.CreateGroup(ctx, 1, " \t ", "")
	require.Error(t, err)
	_, err = s.CreateGroup(ctx, 999, "新组", "")
	require.Error(t, err)
	other, err := s.CreateGroup(ctx, 2, "主角团", "另一本小说的主角")
	require.NoError(t, err)
	require.ErrorIs(t, s.UpdateGroup(ctx, 1, other.ID, "改名", ""), ErrGroupNotFound)
	require.ErrorIs(t, s.DeleteGroup(ctx, 1, other.ID), ErrGroupNotFound)
	require.ErrorIs(t, s.UpdateGroup(ctx, 1, g1.ID, "青云宗", ""), ErrGroupNameTaken)
	require.NoError(t, s.UpdateGroup(ctx, 1, g1.ID, " 核心角色 ", ""))
	groups, err := s.ListGroups(ctx, 1, ListGroupsOptions{PageParams: storage.PageParams{Size: -1}})
	require.NoError(t, err)
	require.Len(t, groups.Items, 2)
	var found Group
	require.NoError(t, s.DB.First(&found, g1.ID).Error)
	require.Equal(t, "核心角色", found.Name)
	require.Empty(t, found.Description)
	require.Equal(t, g1.CreatedAt.UnixNano(), found.CreatedAt.UnixNano())
	groups, err = s.ListGroups(ctx, 999, ListGroupsOptions{PageParams: storage.PageParams{Size: -1}})
	require.NoError(t, err)
	require.NotNil(t, groups.Items)
	require.Empty(t, groups.Items)
}

func TestGroupMembershipsBatchValidationAndFiltering(t *testing.T) {
	s, c1, c2, g1, g2 := groupStoreFixture(t)
	ctx := context.Background()
	foreign := Character{NovelID: 2, Name: "其他小说角色"}
	require.NoError(t, s.DB.Create(&foreign).Error)
	foreignGroup, err := s.CreateGroup(ctx, 2, "其他组", "")
	require.NoError(t, err)
	for _, tc := range []struct {
		name                 string
		chars, adds, removes []int64
	}{
		{"跨小说角色", []int64{c1.ID, foreign.ID}, []int64{g1.ID}, nil},
		{"跨小说分组", []int64{c1.ID}, []int64{g1.ID, foreignGroup.ID}, nil},
		{"缺失分组", []int64{c1.ID}, []int64{g1.ID, 999}, nil},
		{"加入移出冲突", []int64{c1.ID}, []int64{g1.ID}, []int64{g1.ID}},
		{"无角色", nil, []int64{g1.ID}, nil},
		{"无变更", []int64{c1.ID}, nil, nil},
		{"非法ID", []int64{0}, []int64{g1.ID}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Error(t, s.UpdateGroupMemberships(ctx, 1, tc.chars, tc.adds, tc.removes))
			members, err := s.ListGroupMembers(ctx, 1, ListGroupMembersOptions{PageParams: storage.PageParams{Size: -1}})
			require.NoError(t, err)
			require.Empty(t, members.Items)
		})
	}
	require.NoError(t, s.UpdateGroupMemberships(ctx, 1, []int64{c1.ID, c1.ID, c2.ID}, []int64{g1.ID, g1.ID, g2.ID}, nil))
	require.NoError(t, s.UpdateGroupMemberships(ctx, 1, []int64{c1.ID}, []int64{g1.ID}, nil))
	members, err := s.ListGroupMembers(ctx, 1, ListGroupMembersOptions{PageParams: storage.PageParams{Size: -1}})
	require.NoError(t, err)
	require.Len(t, members.Items, 4)
	groups, err := s.ListGroups(ctx, 1, ListGroupsOptions{PageParams: storage.PageParams{Size: -1}})
	require.NoError(t, err)
	for _, group := range groups.Items {
		require.EqualValues(t, 2, group.MemberCount)
	}
	require.NoError(t, s.UpdateGroupMemberships(ctx, 1, []int64{c2.ID}, nil, []int64{g1.ID, g2.ID}))
	require.NoError(t, s.UpdateGroupMemberships(ctx, 1, []int64{c2.ID}, nil, []int64{g1.ID}))
	for _, tc := range []struct{ groupID, wantID int64 }{{g1.ID, c1.ID}, {0, c2.ID}} {
		result, err := s.ListByNovel(ctx, 1, ListByNovelOptions{GroupID: &tc.groupID, PageParams: storage.PageParams{Size: 1}})
		require.NoError(t, err)
		require.EqualValues(t, 1, result.Total)
		require.Len(t, result.Items, 1)
		require.Equal(t, tc.wantID, result.Items[0].ID)
	}
	duplicate := GroupMember{NovelID: 1, CharacterID: c1.ID, GroupID: g1.ID}
	require.Error(t, s.DB.Create(&duplicate).Error)
}

func TestGroupMembershipFailureRollsBackRowsAndAudit(t *testing.T) {
	s, c1, c2, g1, g2 := groupStoreFixture(t)
	ctx := storage.WithTurn(context.Background(), "test", 1)
	require.NoError(t, s.UpdateGroupMemberships(ctx, 1, []int64{c1.ID, c2.ID}, []int64{g1.ID}, nil))
	before, err := s.ListGroupMembers(ctx, 1, ListGroupMembersOptions{PageParams: storage.PageParams{Size: -1}})
	require.NoError(t, err)
	var logsBefore int64
	require.NoError(t, s.DB.Model(&storage.OperationLogRecord{}).Count(&logsBefore).Error)
	require.NoError(t, s.DB.Exec(`CREATE TRIGGER fail_second_member BEFORE INSERT ON character_group_members
		WHEN NEW.character_id = `+strconv.FormatInt(c2.ID, 10)+` BEGIN SELECT RAISE(ABORT, 'injected failure'); END`).Error)
	require.Error(t, s.UpdateGroupMemberships(ctx, 1, []int64{c1.ID, c2.ID}, []int64{g2.ID}, []int64{g1.ID}))
	after, err := s.ListGroupMembers(ctx, 1, ListGroupMembersOptions{PageParams: storage.PageParams{Size: -1}})
	require.NoError(t, err)
	require.Equal(t, before, after)
	var logsAfter int64
	require.NoError(t, s.DB.Model(&storage.OperationLogRecord{}).Count(&logsAfter).Error)
	require.Equal(t, logsBefore, logsAfter)
}

func TestGroupDeletionAndCharacterDeletionRollback(t *testing.T) {
	for _, deleteCharacter := range []bool{false, true} {
		t.Run(map[bool]string{false: "group", true: "character"}[deleteCharacter], func(t *testing.T) {
			s, c1, c2, g1, g2 := groupStoreFixture(t)
			ctx := context.Background()
			require.NoError(t, s.UpdateGroupMemberships(ctx, 1, []int64{c1.ID, c2.ID}, []int64{g1.ID, g2.ID}, nil))
			relation := CharacterRelation{NovelID: 1, SourceCharacterID: c1.ID, TargetCharacterID: c2.ID, RelationDescribe: "朋友", IsCurrent: true}
			require.NoError(t, s.DB.Create(&relation).Error)
			before, err := s.ListGroupMembers(ctx, 1, ListGroupMembersOptions{PageParams: storage.PageParams{Size: -1}})
			require.NoError(t, err)
			turn := storage.WithTurn(ctx, "test", 2)
			if deleteCharacter {
				require.NoError(t, s.DeleteCharacter(turn, 1, c1.ID))
			} else {
				require.NoError(t, s.DeleteGroup(turn, 1, g1.ID))
			}
			after, err := s.ListGroupMembers(ctx, 1, ListGroupMembersOptions{PageParams: storage.PageParams{Size: -1}})
			require.NoError(t, err)
			require.Len(t, after.Items, 2)
			require.NoError(t, storage.RollbackTo(ctx, s.DB, "test", 2, 2))
			restored, err := s.ListGroupMembers(ctx, 1, ListGroupMembersOptions{PageParams: storage.PageParams{Size: -1}})
			require.NoError(t, err)
			require.Equal(t, before, restored)
			var restoredRelation CharacterRelation
			require.NoError(t, s.DB.First(&restoredRelation, relation.ID).Error)
			require.Equal(t, relation.RelationDescribe, restoredRelation.RelationDescribe)
			var restoredCharacter Character
			require.NoError(t, s.DB.First(&restoredCharacter, c1.ID).Error)
		})
	}
}

func TestGroupDeleteFailureIsAtomic(t *testing.T) {
	s, c1, _, g1, _ := groupStoreFixture(t)
	ctx := context.Background()
	require.NoError(t, s.UpdateGroupMemberships(ctx, 1, []int64{c1.ID}, []int64{g1.ID}, nil))
	require.NoError(t, s.DB.Exec(`CREATE TRIGGER fail_group_delete BEFORE DELETE ON character_groups BEGIN SELECT RAISE(ABORT, 'injected failure'); END`).Error)
	require.Error(t, s.DeleteGroup(ctx, 1, g1.ID))
	members, err := s.ListGroupMembers(ctx, 1, ListGroupMembersOptions{PageParams: storage.PageParams{Size: -1}})
	require.NoError(t, err)
	require.Len(t, members.Items, 1)
	require.NoError(t, s.DB.First(&Group{}, g1.ID).Error)
}

func TestGroupMetadataRollback(t *testing.T) {
	s, _, _, _, _ := groupStoreFixture(t)
	ctx := storage.WithTurn(context.Background(), "test", 1)
	g, err := s.CreateGroup(ctx, 1, "阶段反派", "第三卷登场")
	require.NoError(t, err)
	require.NoError(t, s.UpdateGroup(storage.WithTurn(ctx, "test", 2), 1, g.ID, "宗门敌对势力", ""))
	require.NoError(t, storage.RollbackTo(context.Background(), s.DB, "test", 2, 2))
	var restored Group
	require.NoError(t, s.DB.First(&restored, g.ID).Error)
	require.Equal(t, "阶段反派", restored.Name)
	require.Equal(t, "第三卷登场", restored.Description)
	require.NoError(t, storage.RollbackTo(context.Background(), s.DB, "test", 1, 1))
	require.ErrorIs(t, s.DB.First(&Group{}, g.ID).Error, gorm.ErrRecordNotFound)
}

func TestListGroupsPaginationSearchAndOrder(t *testing.T) {
	s, c1, c2, g1, g2 := groupStoreFixture(t)
	ctx := context.Background()
	g3, err := s.CreateGroup(ctx, 1, "第三组", "主角曾经的敌人")
	require.NoError(t, err)
	g4, err := s.CreateGroup(ctx, 1, "第四组", "路人")
	require.NoError(t, err)
	_, err = s.CreateGroup(ctx, 2, "主角团", "主角曾经的敌人")
	require.NoError(t, err)
	require.NoError(t, s.UpdateGroupMemberships(ctx, 1, []int64{c1.ID, c2.ID}, []int64{g1.ID}, nil))
	for _, tc := range []struct {
		name       string
		opts       ListGroupsOptions
		ids        []int64
		total      int64
		page, size int
		pages      int
	}{
		{"首页归一化", ListGroupsOptions{PageParams: storage.PageParams{Size: 2}, Order: "id ASC"}, []int64{g1.ID, g2.ID}, 4, 1, 2, 2},
		{"降序第二页", ListGroupsOptions{PageParams: storage.PageParams{Page: 2, Size: 2}, Order: "id DESC"}, []int64{g2.ID, g1.ID}, 4, 2, 2, 2},
		{"名称和描述搜索", ListGroupsOptions{PageParams: storage.PageParams{Size: 1}, Search: "主角", Order: "id DESC"}, []int64{g3.ID}, 2, 1, 1, 2},
		{"按人数排序", ListGroupsOptions{PageParams: storage.PageParams{Size: 1}, Order: "member_count DESC, id ASC"}, []int64{g1.ID}, 4, 1, 1, 4},
		{"零大小", ListGroupsOptions{PageParams: storage.PageParams{Size: 0}}, nil, 4, 1, 0, 0},
		{"全量归一化", ListGroupsOptions{PageParams: storage.PageParams{Page: 7, Size: -5}, Order: "id ASC"}, []int64{g1.ID, g2.ID, g3.ID, g4.ID}, 4, 1, -1, 1},
		{"超出页数", ListGroupsOptions{PageParams: storage.PageParams{Page: 3, Size: 2}}, nil, 4, 3, 2, 2},
		{"无搜索结果", ListGroupsOptions{PageParams: storage.PageParams{Size: 2}, Search: "不存在"}, nil, 0, 1, 2, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := s.ListGroups(ctx, 1, tc.opts)
			require.NoError(t, err)
			require.NotNil(t, result.Items)
			require.Equal(t, tc.total, result.Total)
			require.Equal(t, tc.page, result.Page)
			require.Equal(t, tc.size, result.Size)
			require.Equal(t, tc.pages, result.TotalPages)
			require.Len(t, result.Items, len(tc.ids))
			for i, id := range tc.ids {
				require.Equal(t, id, result.Items[i].ID)
				if id == g1.ID {
					require.EqualValues(t, 2, result.Items[i].MemberCount)
				} else {
					require.Zero(t, result.Items[i].MemberCount)
				}
			}
		})
	}
}

func TestListGroupMembersPaginationFiltersSearchAndOrder(t *testing.T) {
	s, c1, c2, g1, g2 := groupStoreFixture(t)
	ctx := context.Background()
	require.NoError(t, s.UpdateGroupMemberships(ctx, 1, []int64{c1.ID, c2.ID}, []int64{g1.ID, g2.ID}, nil))
	foreign := Character{NovelID: 2, Name: "张三丰"}
	require.NoError(t, s.DB.Create(&foreign).Error)
	foreignGroup, err := s.CreateGroup(ctx, 2, "主角团", "")
	require.NoError(t, err)
	require.NoError(t, s.UpdateGroupMemberships(ctx, 2, []int64{foreign.ID}, []int64{foreignGroup.ID}, nil))
	for _, tc := range []struct {
		name       string
		opts       ListGroupMembersOptions
		pairs      [][2]int64
		total      int64
		page, size int
		pages      int
	}{
		{"默认排序第二页", ListGroupMembersOptions{PageParams: storage.PageParams{Page: 2, Size: 2}}, [][2]int64{{g2.ID, c1.ID}, {g2.ID, c2.ID}}, 4, 2, 2, 2},
		{"分组筛选和角色搜索", ListGroupMembersOptions{PageParams: storage.PageParams{Size: 1}, GroupID: g1.ID, Search: "张"}, [][2]int64{{g1.ID, c1.ID}}, 1, 1, 1, 1},
		{"角色筛选和自定义排序", ListGroupMembersOptions{PageParams: storage.PageParams{Size: 1}, CharacterID: c2.ID, Order: "group_id DESC"}, [][2]int64{{g2.ID, c2.ID}}, 2, 1, 1, 2},
		{"全量归一化", ListGroupMembersOptions{PageParams: storage.PageParams{Page: 7, Size: -2}, Search: "张", Order: "group_id DESC"}, [][2]int64{{g2.ID, c1.ID}, {g1.ID, c1.ID}}, 2, 1, -1, 1},
		{"零大小", ListGroupMembersOptions{PageParams: storage.PageParams{}}, nil, 4, 1, 0, 0},
		{"超出页数", ListGroupMembersOptions{PageParams: storage.PageParams{Page: 3, Size: 2}}, nil, 4, 3, 2, 2},
		{"跨小说分组", ListGroupMembersOptions{PageParams: storage.PageParams{Size: 2}, GroupID: foreignGroup.ID}, nil, 0, 1, 2, 0},
		{"跨小说角色", ListGroupMembersOptions{PageParams: storage.PageParams{Size: 2}, CharacterID: foreign.ID}, nil, 0, 1, 2, 0},
		{"无搜索结果", ListGroupMembersOptions{PageParams: storage.PageParams{Size: 2}, Search: "不存在"}, nil, 0, 1, 2, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := s.ListGroupMembers(ctx, 1, tc.opts)
			require.NoError(t, err)
			require.NotNil(t, result.Items)
			require.Equal(t, tc.total, result.Total)
			require.Equal(t, tc.page, result.Page)
			require.Equal(t, tc.size, result.Size)
			require.Equal(t, tc.pages, result.TotalPages)
			require.Len(t, result.Items, len(tc.pairs))
			for i, pair := range tc.pairs {
				require.Equal(t, pair[0], result.Items[i].GroupID)
				require.Equal(t, pair[1], result.Items[i].CharacterID)
			}
		})
	}
}
