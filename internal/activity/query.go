package activity

import (
	"context"
	"time"

	"github.com/sigpanic/goink/internal/config"
)

// TrackingOrigin 保留首次启用时的 UTC 时间和本地日期，不随查询时区重新归属。
type TrackingOrigin struct {
	TrackingStartedAt *time.Time
	TrackingStartDate string
}

// GetTrackingOrigin 只读统计起点，不在报告查询时初始化或修复配置。
func (s *Store) GetTrackingOrigin(ctx context.Context) (*TrackingOrigin, error) {
	var origin TrackingOrigin
	if err := s.db.WithContext(ctx).Model(&config.AppSettings{}).
		Select("tracking_started_at, tracking_start_date").
		Where("id = ?", 1).Take(&origin).Error; err != nil {
		return nil, err
	}
	return &origin, nil
}

// GetDailyActivity 返回本地日期 [start, end) 内的已有活动记录，按日期排序。
func (s *Store) GetDailyActivity(ctx context.Context, start, end string) ([]DailyActivity, error) {
	rows := make([]DailyActivity, 0)
	err := s.db.WithContext(ctx).Where("date >= ? AND date < ?", start, end).
		Order("date ASC").Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// GetDailyLLMUsage 返回本地日期 [start, end) 内的用量，保留服务商、模型及用途维度。
func (s *Store) GetDailyLLMUsage(ctx context.Context, start, end string) ([]DailyLLMUsage, error) {
	rows := make([]DailyLLMUsage, 0)
	err := s.db.WithContext(ctx).Where("date >= ? AND date < ?", start, end).
		Order("date ASC, provider ASC, model ASC, purpose ASC").Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}
