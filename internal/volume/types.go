package volume

import "time"

// Volume 是小说分卷的元数据。
// 卷纲正文以文件形式存储在 Git 仓库 volumes/{id}.md，DB 仅保存索引。
//
// 设计原则：
//   - sort_order 是内部排序键，仅用于 ORDER BY，对用户/AI 不可见
//   - "第 N 卷" 实时按 (novel_id, sort_order) ASC 排序后位次生成，不存 DB
//   - 卷的增删移动只改 sort_order，不影响 chapter 文件路径（chapter 用 chapter.id 命名）
//
// 约束：
//   - (novel_id, sort_order) 唯一索引：同一小说内卷顺序不可重复
//   - (novel_id, name) 唯一索引：同一小说内卷名不可重复
type Volume struct {
	ID        int64     `gorm:"column:id;primaryKey;autoIncrement"                              json:"id"`
	NovelID   int64     `gorm:"column:novel_id;not null;uniqueIndex:uk_novel_volume_sort;uniqueIndex:uk_novel_volume_name;index" json:"novel_id"`
	Name      string    `gorm:"column:name;not null;uniqueIndex:uk_novel_volume_name"          json:"name"`
	SortOrder int       `gorm:"column:sort_order;not null;uniqueIndex:uk_novel_volume_sort"   json:"sort_order"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"                              json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime"                              json:"updated_at"`
}

// TableName 指定 GORM 表名。
func (Volume) TableName() string { return "volumes" }
