package novel

import "time"

// Novel 是小说索引，记录每部小说的基本信息。
// 正文存储在 Git 仓库中，路径由 config.NovelDirPath(ID) 实时计算。
type Novel struct {
	ID          int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	Title       string    `gorm:"column:title;not null;index"        json:"title"`
	Genre       string    `gorm:"column:genre;index"                 json:"genre"`
	Description string    `gorm:"column:description"                 json:"description"`
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime"   json:"created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at;autoUpdateTime"   json:"updated_at"`
}

// TableName 指定 GORM 表名。
func (Novel) TableName() string { return "novels" }
