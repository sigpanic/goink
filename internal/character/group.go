package character

import "time"

// Group 是小说内的角色分类；Description 说明分类用途，不承载阵营演变历史。
type Group struct {
	ID          int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	NovelID     int64     `gorm:"column:novel_id;not null;uniqueIndex:uk_character_group_name" json:"novel_id"`
	Name        string    `gorm:"column:name;not null;uniqueIndex:uk_character_group_name" json:"name"`
	Description string    `gorm:"column:description" json:"description"`
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

func (Group) TableName() string { return "character_groups" }

// GroupMember 独立记录每条归属，便于逐行审计和回退。
type GroupMember struct {
	ID          int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	NovelID     int64     `gorm:"column:novel_id;not null;index" json:"novel_id"`
	GroupID     int64     `gorm:"column:group_id;not null;uniqueIndex:uk_character_group_member" json:"group_id"`
	CharacterID int64     `gorm:"column:character_id;not null;uniqueIndex:uk_character_group_member;index" json:"character_id"`
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
}

func (GroupMember) TableName() string { return "character_group_members" }

// GroupView 的人数由查询计算，不写入分组记录或操作日志。
type GroupView struct {
	Group
	MemberCount int64 `gorm:"column:member_count;->;-:migration" json:"member_count"`
}
