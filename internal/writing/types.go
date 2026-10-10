package writing

import "time"

// WritingLog 按本地日期和小说累计保存版本间的增删量，WordDelta 为当天净增。
type WritingLog struct {
	ID           int64     `gorm:"column:id;primaryKey;autoIncrement"`
	Date         string    `gorm:"column:date;not null;uniqueIndex:uk_writing_date_novel;size:10"` // "2006-01-02"
	NovelID      int64     `gorm:"column:novel_id;not null;default:0;uniqueIndex:uk_writing_date_novel;index"`
	WordDelta    int       `gorm:"column:word_delta;not null"`
	WordsAdded   int       `gorm:"column:words_added;not null;default:0"`
	WordsDeleted int       `gorm:"column:words_deleted;not null;default:0"`
	CreatedAt    time.Time `gorm:"column:created_at;autoCreateTime"`
}

func (WritingLog) TableName() string { return "writing_log" }

// DailyActivity 单天汇总字数。
// 同时返回新增、删除和可为负数的净增。
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
