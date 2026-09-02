package chapter

import "time"

// Chapter 是章节元数据，正文和大纲以文件形式存储在 Git 仓库中。
// DB 仅保存索引和统计信息，不存实际内容。
//
// v1.5.0 迁移期双字段共存：
//   - ChapterNumber（旧）：章节号，迁移完成后由 commit 1.7 删除
//   - VolumeID/SortOrder（新）：分卷外键 + 内部排序键，章节号改由 (volume_id, sort_order) 排序后位次实时生成
//   - ChapterNumber 唯一索引在 commit 1.7 删字段时一并删除
type Chapter struct {
	ID            int64     `gorm:"column:id;primaryKey;autoIncrement"                                    json:"id"`
	NovelID       int64     `gorm:"column:novel_id;not null;uniqueIndex:uk_novel_chapter;index"           json:"novel_id"`
	ChapterNumber int       `gorm:"column:chapter_number;not null;uniqueIndex:uk_novel_chapter"           json:"chapter_number"`
	VolumeID      *int64    `gorm:"column:volume_id;index"                                                 json:"volume_id"`   // 可空外键 → volumes.id，NULL=未分卷；commit 1.2 新增
	SortOrder     int       `gorm:"column:sort_order;default:0"                                           json:"sort_order"`  // 内部排序键，commit 1.6 初始化=ChapterNumber；commit 1.2 新增
	Title         string    `gorm:"column:title"                                                          json:"title"`
	Summary       string    `gorm:"column:summary"                                                        json:"summary"` // AI 生成的章节简介
	WordCount     int       `gorm:"column:word_count;default:0"                                           json:"word_count"`
	CreatedAt     time.Time `gorm:"column:created_at;autoCreateTime"                                      json:"created_at"`
	UpdatedAt     time.Time `gorm:"column:updated_at;autoUpdateTime"                                      json:"updated_at"`
	FilePath      string    `gorm:"-"                                                                       json:"file_path"` // 不存 DB，由 git.ChapterPath 计算
}

// TableName 指定 GORM 表名。
func (Chapter) TableName() string { return "chapters" }
