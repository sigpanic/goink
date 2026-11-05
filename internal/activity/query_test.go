package activity

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/config"
)

func TestQueryDailyStatisticsRange(t *testing.T) {
	s, db := newTestStore(t)
	ctx := context.Background()
	for _, date := range []string{"2023-12-31", "2024-01-01", "2024-02-29", "2024-12-31", "2025-01-01"} {
		require.NoError(t, db.Create(&DailyActivity{Date: date, ActivityDelta: ActivityDelta{ToolCalls: 1}}).Error)
		for _, dimension := range [][3]string{{"p", "m", "chat"}, {"p", "m", "review"}, {"q", "m", "chat"}} {
			require.NoError(t, db.Create(&DailyLLMUsage{
				Date: date, Provider: dimension[0], Model: dimension[1], Purpose: dimension[2],
				UsageCount: 1, TokenUsage: TokenUsage{TotalTokens: 3_000_000_000},
			}).Error)
		}
	}
	rows, err := s.GetDailyActivity(ctx, "2024-01-01", "2025-01-01")
	require.NoError(t, err)
	require.Equal(t, []DailyActivity{
		{Date: "2024-01-01", ActivityDelta: ActivityDelta{ToolCalls: 1}},
		{Date: "2024-02-29", ActivityDelta: ActivityDelta{ToolCalls: 1}},
		{Date: "2024-12-31", ActivityDelta: ActivityDelta{ToolCalls: 1}},
	}, rows)
	usage, err := s.GetDailyLLMUsage(ctx, "2024-01-01", "2025-01-01")
	require.NoError(t, err)
	require.Len(t, usage, 9)
	require.Equal(t, "2024-01-01", usage[0].Date)
	require.Equal(t, "chat", usage[0].Purpose)
	require.Equal(t, "review", usage[1].Purpose)
	require.Equal(t, "q", usage[2].Provider)
	require.Equal(t, "2024-12-31", usage[8].Date)
	require.EqualValues(t, 3_000_000_000, usage[8].TotalTokens)

	rows, err = s.GetDailyActivity(ctx, "2022-01-01", "2023-01-01")
	require.NoError(t, err)
	require.NotNil(t, rows)
	require.Empty(t, rows)
	usage, err = s.GetDailyLLMUsage(ctx, "2022-01-01", "2023-01-01")
	require.NoError(t, err)
	require.NotNil(t, usage)
	require.Empty(t, usage)
}

func TestGetTrackingOriginIsReadOnly(t *testing.T) {
	s, db := newTestStore(t)
	ctx := context.Background()
	_, err := s.GetTrackingOrigin(ctx)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	var count int64
	require.NoError(t, db.Model(&config.AppSettings{}).Count(&count).Error)
	require.Zero(t, count)
	s.InitTracking(ctx)
	origin, err := s.GetTrackingOrigin(ctx)
	require.NoError(t, err)
	require.Equal(t, "2027-01-01", origin.TrackingStartDate)
	require.True(t, s.now().Equal(*origin.TrackingStartedAt))
}

func TestStatisticsQueriesReturnErrors(t *testing.T) {
	s, db := newTestStore(t)
	require.NoError(t, db.Migrator().DropTable(&config.AppSettings{}, &DailyActivity{}, &DailyLLMUsage{}))
	_, err := s.GetTrackingOrigin(context.Background())
	require.Error(t, err)
	_, err = s.GetDailyActivity(context.Background(), "2024-01-01", "2025-01-01")
	require.Error(t, err)
	_, err = s.GetDailyLLMUsage(context.Background(), "2024-01-01", "2025-01-01")
	require.Error(t, err)
}
