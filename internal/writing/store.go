package writing

import (
	"context"
	"log/slog"
	"time"

	"gorm.io/gorm"
)

// Store 管理 writing_log 持久化。
type Store struct {
	DB     *gorm.DB
	logger *slog.Logger
}

// NewStore 创建写作日志存储。
func NewStore(db *gorm.DB, logger *slog.Logger) *Store {
	return &Store{DB: db, logger: logger}
}

// LogDelta 记录一次字数变化。delta 为 0 时跳过。
func (s *Store) LogDelta(ctx context.Context, novelID, chapterID int64, delta int) {
	if delta == 0 {
		return
	}
	record := WritingLog{
		Date:      time.Now().Format("2006-01-02"),
		NovelID:   novelID,
		ChapterID: &chapterID,
		WordDelta: delta,
	}
	if err := s.DB.WithContext(ctx).Create(&record).Error; err != nil {
		s.logger.Warn("记录写作日志失败", "novel_id", novelID, "chapter_id", chapterID, "delta", delta, "err", err)
	}
}

// LogChanges 记录成功保存的增删量；等长替换也保留，统计失败只告警。
func (s *Store) LogChanges(ctx context.Context, novelID, chapterID int64, added, deleted int) {
	if added == 0 && deleted == 0 {
		return
	}
	record := WritingLog{
		Date:         time.Now().Format("2006-01-02"),
		NovelID:      novelID,
		ChapterID:    &chapterID,
		WordDelta:    added - deleted,
		WordsAdded:   &added,
		WordsDeleted: &deleted,
	}
	if err := s.DB.WithContext(ctx).Create(&record).Error; err != nil {
		s.logger.Warn("记录写作日志失败", "novel_id", novelID, "chapter_id", chapterID, "added", added, "deleted", deleted, "err", err)
	}
}

// 历史记录保留正向净变化口径；新记录使用 diff 新增量。
const addedWordsExpr = "COALESCE(words_added, CASE WHEN word_delta > 0 THEN word_delta ELSE 0 END)"

const deletedWordsExpr = "COALESCE(words_deleted, CASE WHEN word_delta < 0 THEN -word_delta ELSE 0 END)"
const editedWordsExpr = "(" + addedWordsExpr + " + " + deletedWordsExpr + ")"

// GetDailyActivity 返回最近 months 个月的每日字数汇总。
func (s *Store) GetDailyActivity(ctx context.Context, months int) ([]DailyActivity, error) {
	if months <= 0 {
		months = 12
	}
	cutoff := time.Now().AddDate(0, -months, 0).Format("2006-01-02")

	var results []DailyActivity
	err := s.DB.WithContext(ctx).
		Model(&WritingLog{}).
		Select("date, SUM(word_delta) as words_net, SUM("+addedWordsExpr+") as words_added, SUM("+deletedWordsExpr+") as words_deleted").
		Where("date >= ? AND "+editedWordsExpr+" > 0", cutoff).
		Group("date").
		Order("date ASC").
		Scan(&results).Error
	if err != nil {
		return nil, err
	}
	return results, nil
}

// GetWritingStats 返回全局写作统计。
func (s *Store) GetWritingStats(ctx context.Context, novelCount, chapterCount int64) (*WritingStats, error) {
	// 总字数：正数 delta 之和
	// 新记录使用新增量，正数 delta 仅作为历史记录的回退口径。
	var totalWords int64
	s.DB.WithContext(ctx).
		Model(&WritingLog{}).
		Select("COALESCE(SUM(" + addedWordsExpr + "), 0)").
		Where(addedWordsExpr + " > 0").
		Scan(&totalWords)

	// 活跃天数
	var totalDays int64
	s.DB.WithContext(ctx).
		Model(&WritingLog{}).
		Select("COUNT(DISTINCT date)").
		Where(editedWordsExpr + " > 0").
		Scan(&totalDays)

	currentStreak, longestStreak := s.computeStreaks(ctx)

	return &WritingStats{
		TotalWords:      int(totalWords),
		TotalDaysActive: int(totalDays),
		CurrentStreak:   currentStreak,
		LongestStreak:   longestStreak,
		TotalNovels:     novelCount,
		TotalChapters:   chapterCount,
	}, nil
}

// computeStreaks 计算当前连续天数和最长连续天数。
func (s *Store) computeStreaks(ctx context.Context) (current, longest int) {
	var dates []string
	s.DB.WithContext(ctx).
		Model(&WritingLog{}).
		Select("DISTINCT date").
		Where(editedWordsExpr+" > 0").
		Order("date ASC").
		Pluck("date", &dates)

	if len(dates) == 0 {
		return 0, 0
	}

	var parsed []time.Time
	for _, d := range dates {
		t, err := time.ParseInLocation("2006-01-02", d, time.Local)
		if err != nil {
			continue
		}
		parsed = append(parsed, t)
	}

	longest = 1
	running := 1
	for i := 1; i < len(parsed); i++ {
		diff := parsed[i].Sub(parsed[i-1]).Hours()
		if diff >= 24 && diff < 48 {
			running++
			if running > longest {
				longest = running
			}
		} else if diff >= 48 {
			running = 1
		}
	}

	// 当前连续：从最后一天往回数
	today, _ := time.ParseInLocation("2006-01-02", time.Now().Format("2006-01-02"), time.Local)
	yesterday := today.AddDate(0, 0, -1)
	lastDate := parsed[len(parsed)-1]
	if lastDate.Equal(today) || lastDate.Equal(yesterday) {
		current = 1
		for i := len(parsed) - 1; i > 0; i-- {
			diff := parsed[i].Sub(parsed[i-1]).Hours()
			if diff >= 24 && diff < 48 {
				current++
			} else {
				break
			}
		}
	}

	return
}
