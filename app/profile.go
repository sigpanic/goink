package app

import (
	"github.com/sigpanic/goink/internal/chapter"
	"github.com/sigpanic/goink/internal/novel"
	"github.com/sigpanic/goink/internal/writing"
)

// GetWritingActivity 返回最近 months 个月每日写作字数汇总。
func (a *App) GetWritingActivity(months int) ([]writing.DailyActivity, error) {
	return a.writing.GetDailyActivity(a.ctx, months)
}

// GetWritingStats 返回全局写作统计，跨所有小说。
func (a *App) GetWritingStats() (*writing.WritingStats, error) {
	novels, err := a.novel.List(a.ctx, novel.ListNovelsOptions{})
	if err != nil {
		return nil, err
	}

	var chapterCount int64
	a.db.WithContext(a.ctx).Model(&chapter.Chapter{}).Count(&chapterCount)

	return a.writing.GetWritingStats(a.ctx, novels.Total, chapterCount)
}
