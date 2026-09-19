package chapter

import "time"

// Chapter 是章节元数据，正文和大纲以文件形式存储在 Git 仓库中。
// DB 仅保存索引和统计信息，不存实际内容。
//
// 阅读顺序由 VolumeID/SortOrder 决定，展示的“第几章”按当前顺序实时计算，
// 不作为章节记录的持久化字段。
type Chapter struct {
	ID        int64     `gorm:"column:id;primaryKey;autoIncrement"  json:"id"`
	NovelID   int64     `gorm:"column:novel_id;not null;index"      json:"novel_id"`
	VolumeID  *int64    `gorm:"column:volume_id;index"              json:"volume_id"` // 可空外键 → volumes.id，NULL=未分卷
	SortOrder int       `gorm:"column:sort_order;default:0"        json:"sort_order"` // 所属分组内的内部排序键
	Title     string    `gorm:"column:title"                       json:"title"`
	Summary   string    `gorm:"column:summary"                     json:"summary"` // AI 生成的章节简介
	WordCount int       `gorm:"column:word_count;default:0"        json:"word_count"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"   json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime"   json:"updated_at"`
	FilePath  string    `gorm:"-"                                  json:"file_path"` // 不存 DB，由 git.ChapterPath 计算
}

// TableName 指定 GORM 表名。
func (Chapter) TableName() string { return "chapters" }
