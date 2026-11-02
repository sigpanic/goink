package report

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/sigpanic/goink/internal/activity"
	"github.com/sigpanic/goink/internal/writing"
)

var (
	ErrInvalidYear         = errors.New("invalid report year")
	ErrTrackingUnavailable = errors.New("creative statistics tracking origin is unavailable")
)

// Service 协调活动、写作及作品查询，不改变采集入口或个人页累计统计。
type Service struct {
	activity *activity.Store
	writing  *writing.Store
	now      func() time.Time
}

func NewService(activityStore *activity.Store, writingStore *writing.Store) *Service {
	return &Service{activity: activityStore, writing: writingStore, now: time.Now}
}

// GetAnnual 按当地自然年查询，以统计起点和今天裁剪所有指标的统一覆盖区间。
func (s *Service) GetAnnual(ctx context.Context, year int) (*AnnualReport, error) {
	now := s.now()
	if year < 1 || year > now.Year() {
		return nil, ErrInvalidYear
	}
	origin, err := s.activity.GetTrackingOrigin(ctx)
	if err != nil {
		return nil, fmt.Errorf("report: tracking origin: %w", err)
	}
	if origin.TrackingStartedAt == nil {
		return nil, ErrTrackingUnavailable
	}
	if _, err := time.Parse(time.DateOnly, origin.TrackingStartDate); err != nil {
		return nil, fmt.Errorf("%w: invalid local start date", ErrTrackingUnavailable)
	}

	report := &AnnualReport{
		Year: year, TrackingOrigin: *origin,
		DailyActivity: make([]activity.DailyActivity, 0),
		DailyWriting:  make([]writing.DailyActivity, 0),
		DailyLLMUsage: make([]activity.DailyLLMUsage, 0),
		Monthly:       make([]MonthlySummary, 12),
		Novels:        make([]writing.NovelWords, 0),
		Models:        make([]ModelUsage, 0),
	}
	for i := range report.Monthly {
		report.Monthly[i].Month = i + 1
	}
	start := max(time.Date(year, 1, 1, 0, 0, 0, 0, now.Location()).Format(time.DateOnly), origin.TrackingStartDate)
	end := min(time.Date(year+1, 1, 1, 0, 0, 0, 0, now.Location()).Format(time.DateOnly), now.AddDate(0, 0, 1).Format(time.DateOnly))
	if start >= end {
		return report, nil
	}
	report.Coverage = &DateRange{Start: start, End: end}

	report.DailyActivity, err = s.activity.GetDailyActivity(ctx, start, end)
	if err != nil {
		return nil, fmt.Errorf("report: daily activity: %w", err)
	}
	report.DailyWriting, err = s.writing.GetDailyActivityRange(ctx, start, end)
	if err != nil {
		return nil, fmt.Errorf("report: daily writing: %w", err)
	}
	report.DailyLLMUsage, err = s.activity.GetDailyLLMUsage(ctx, start, end)
	if err != nil {
		return nil, fmt.Errorf("report: daily model usage: %w", err)
	}
	report.Novels, err = s.writing.GetNovelRanking(ctx, start, end)
	if err != nil {
		return nil, fmt.Errorf("report: novel ranking: %w", err)
	}
	if err := report.summarize(); err != nil {
		return nil, err
	}
	return report, nil
}

func (r *AnnualReport) summarize() error {
	for _, day := range r.DailyActivity {
		date, err := time.Parse(time.DateOnly, day.Date)
		if err != nil {
			return fmt.Errorf("report: activity date: %w", err)
		}
		r.Activity.NovelsCreated += day.NovelsCreated
		r.Activity.ChaptersCreated += day.ChaptersCreated
		r.Activity.CharactersCreated += day.CharactersCreated
		r.Activity.LocationsCreated += day.LocationsCreated
		r.Activity.CreativeSeconds += day.CreativeSeconds
		r.Activity.ConversationTurns += day.ConversationTurns
		r.Activity.ToolCalls += day.ToolCalls
		r.Monthly[int(date.Month())-1].CreativeSeconds += day.CreativeSeconds
	}
	var previous time.Time
	streak := 0
	for _, day := range r.DailyWriting {
		date, err := time.Parse(time.DateOnly, day.Date)
		if err != nil {
			return fmt.Errorf("report: writing date: %w", err)
		}
		r.Writing.WordsAdded += day.WordsAdded
		r.Writing.WordsDeleted += day.WordsDeleted
		r.Writing.WordsNet += day.WordsNet
		r.Writing.ActiveDays++
		if date.Equal(previous.AddDate(0, 0, 1)) {
			streak++
		} else {
			streak = 1
		}
		r.Writing.LongestStreak = max(r.Writing.LongestStreak, streak)
		previous = date
		r.Monthly[int(date.Month())-1].WordsAdded += day.WordsAdded
	}

	type modelKey struct{ provider, model string }
	models := make(map[modelKey]*ModelUsage)
	for _, day := range r.DailyLLMUsage {
		addTokens(&r.Usage, day.TokenUsage)
		r.UsageCount += day.UsageCount
		key := modelKey{day.Provider, day.Model}
		model := models[key]
		if model == nil {
			model = &ModelUsage{Provider: day.Provider, Model: day.Model}
			models[key] = model
		}
		addTokens(&model.TokenUsage, day.TokenUsage)
		model.UsageCount += day.UsageCount
	}
	for _, model := range models {
		r.Models = append(r.Models, *model)
	}
	sort.Slice(r.Models, func(i, j int) bool {
		a, b := r.Models[i], r.Models[j]
		if a.UsageCount != b.UsageCount {
			return a.UsageCount > b.UsageCount
		}
		if a.Provider != b.Provider {
			return a.Provider < b.Provider
		}
		return a.Model < b.Model
	})
	return nil
}

func addTokens(total *activity.TokenUsage, usage activity.TokenUsage) {
	total.InputTokens += usage.InputTokens
	total.OutputTokens += usage.OutputTokens
	total.TotalTokens += usage.TotalTokens
	total.CacheHitTokens += usage.CacheHitTokens
	total.CacheMissTokens += usage.CacheMissTokens
	total.ReasoningTokens += usage.ReasoningTokens
}
