package writing

import "time"

// WritingLog 记录每次保存的字数变化。正数表示新增，负数表示删除。
// 多行同一天的不同保存各自独立，查询时按 date GROUP BY SUM(word_delta)。
// 新记录另存增删量；累计新增字数优先汇总 WordsAdded，WordDelta 保留净变化。
type WritingLog struct {
	ID           int64     `gorm:"column:id;primaryKey;autoIncrement"`
	Date         string    `gorm:"column:date;not null;index:idx_writing_date;size:10"` // "2006-01-02"
	NovelID      int64     `gorm:"column:novel_id;not null;default:0;index"`
	ChapterID    *int64    `gorm:"column:chapter_id;index"` // chapters.id；nullable 兼容迁移反查失败的历史记录，删章节后允许孤儿
	WordDelta    int       `gorm:"column:word_delta;not null"`
	WordsAdded   *int      `gorm:"column:words_added"` // NULL 表示历史记录未做 diff，0 表示没有新增
	WordsDeleted *int      `gorm:"column:words_deleted"`
	CreatedAt    time.Time `gorm:"column:created_at;autoCreateTime"`
}

func (WritingLog) TableName() string { return "writing_log" }

// DailyActivity 单天汇总字数。
// 新记录汇总新增量，历史记录仍按正向净变化累计。
type DailyActivity struct {
	Date         string `json:"date"`      // "2006-01-02"
	WordsNet     int    `json:"words_net"` // 当天 SUM(word_delta)
	WordsAdded   int    `json:"words_added"`
	WordsDeleted int    `json:"words_deleted"`
}

// WritingStats 全局写作统计。
type WritingStats struct {
	TotalWords      int   `json:"total_words"`
	TotalDaysActive int   `json:"total_days_active"`
	CurrentStreak   int   `json:"current_streak"`
	LongestStreak   int   `json:"longest_streak"`
	TotalNovels     int64 `json:"total_novels"`
	TotalChapters   int64 `json:"total_chapters"`
}
