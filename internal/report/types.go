package report

import (
	"github.com/sigpanic/goink/internal/activity"
	"github.com/sigpanic/goink/internal/writing"
)

// DateRange 是本地日历日期的左闭右开区间。
type DateRange struct {
	Start string
	End   string
}

type WritingSummary struct {
	WordsAdded    int
	WordsDeleted  int
	WordsNet      int
	ActiveDays    int
	LongestStreak int
}

// MonthlySummary 固定按 1～12 月排列，未覆盖的月份为零，时长来自活动记录。
type MonthlySummary struct {
	Month           int
	WordsAdded      int
	CreativeSeconds int64
}

// ModelUsage 合并同一服务商、模型的各用途用量；UsageCount 仅表示已记录用量次数。
type ModelUsage struct {
	Provider   string
	Model      string
	UsageCount int64
	activity.TokenUsage
}

// AnnualReport 只汇总覆盖区间内的数据；Coverage 为 nil 表示该年份未被统计覆盖。
// 每日序列只包含已有记录，缺失日期由展示层结合覆盖区间区分为零或未覆盖。
type AnnualReport struct {
	Year int
	activity.TrackingOrigin
	Coverage      *DateRange
	Activity      activity.ActivityDelta
	Writing       WritingSummary
	Usage         activity.TokenUsage
	UsageCount    int64
	DailyActivity []activity.DailyActivity
	DailyWriting  []writing.DailyActivity
	DailyLLMUsage []activity.DailyLLMUsage
	Monthly       []MonthlySummary
	Novels        []writing.NovelWords
	Models        []ModelUsage
}
