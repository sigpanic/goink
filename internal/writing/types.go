package writing

import "time"

// WritingLog 记录每次保存的字数变化。正数表示新增，负数表示删除。
// 多行同一天的不同保存各自独立，查询时按 date GROUP BY SUM(word_delta)。
type WritingLog struct {
	ID            int64     `gorm:"column:id;primaryKey;autoIncrement"`
	Date          string    `gorm:"column:date;not null;index:idx_writing_date;size:10"` // "2006-01-02"
	NovelID       int64     `gorm:"column:novel_id;not null;default:0;index"`
	ChapterNumber int       `gorm:"column:chapter_number;not null;default:0;index"`
	WordDelta     int       `gorm:"column:word_delta;not null"`
	CreatedAt     time.Time `gorm:"column:created_at;autoCreateTime"`
}

func (WritingLog) TableName() string { return "writing_log" }

// DailyActivity 单天汇总字数。
type DailyActivity struct {
	Date  string `json:"date"`  // "2006-01-02"
	Words int    `json:"words"` // 当天 SUM(word_delta)
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
