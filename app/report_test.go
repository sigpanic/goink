package app

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/sigpanic/goink/internal/activity"
	"github.com/sigpanic/goink/internal/apperr"
	"github.com/sigpanic/goink/internal/config"
	"github.com/sigpanic/goink/internal/writing"
)

func TestGetAnnualReportReturnsReportFields(t *testing.T) {
	a := setupTestApp(t)
	now := time.Now()
	date := now.Format(time.DateOnly)
	n := createTestNovel(t, a)
	a.activity.AddActivity(a.ctx, activity.ActivityDelta{CharactersCreated: 2, CreativeSeconds: 60, ConversationTurns: 1, ToolCalls: 3})
	a.activity.AddLLMUsage(a.ctx, "provider", "model", "chat", activity.TokenUsage{InputTokens: 100, OutputTokens: 20, TotalTokens: 125})
	a.writing.LogChanges(a.ctx, n.ID, 1, 250, 200)

	result := a.GetAnnualReport(now.Year())
	require.Equal(t, apperr.CodeOK, result.ErrCode)
	require.Empty(t, result.ErrMsg)
	r := result.Data
	require.Equal(t, now.Year(), r.Year)
	require.Equal(t, a.settings.TrackingStartDate, r.TrackingStartDate)
	require.True(t, a.settings.TrackingStartedAt.Equal(*r.TrackingStartedAt))
	require.NotNil(t, r.Coverage)
	require.Equal(t, date, r.Coverage.Start)
	require.Equal(t, now.AddDate(0, 0, 1).Format(time.DateOnly), r.Coverage.End)
	require.EqualValues(t, 2, r.Activity.CharactersCreated)
	require.EqualValues(t, 60, r.Activity.CreativeSeconds)
	require.EqualValues(t, 1, r.Activity.ConversationTurns)
	require.EqualValues(t, 3, r.Activity.ToolCalls)
	require.Equal(t, 250, r.Writing.WordsAdded)
	require.Equal(t, 200, r.Writing.WordsDeleted)
	require.Equal(t, 50, r.Writing.WordsNet)
	require.Equal(t, 1, r.Writing.ActiveDays)
	require.Equal(t, 1, r.Writing.LongestStreak)
	require.EqualValues(t, 125, r.Usage.TotalTokens)
	require.EqualValues(t, 1, r.UsageCount)
	require.Len(t, r.DailyActivity, 1)
	require.Len(t, r.DailyWriting, 1)
	require.Len(t, r.DailyLLMUsage, 1)
	require.Len(t, r.Monthly, 12)
	require.Equal(t, 250, r.Monthly[int(now.Month())-1].WordsAdded)
	require.EqualValues(t, 60, r.Monthly[int(now.Month())-1].CreativeSeconds)
	require.Len(t, r.Novels, 1)
	require.Equal(t, n.Title, r.Novels[0].Title)
	require.Len(t, r.Models, 1)
	require.Equal(t, "provider", r.Models[0].Provider)
	require.Equal(t, "model", r.Models[0].Model)

	payload, err := json.Marshal(result)
	require.NoError(t, err)
	var wire struct {
		Data struct {
			Year              int
			TrackingStartedAt time.Time
			TrackingStartDate string
			Coverage          struct{ Start, End string }
			Activity          struct{ CreativeSeconds int64 }
			Models            []struct {
				Model       string
				TotalTokens int64
			}
		} `json:"data"`
		ErrCode string `json:"err_code"`
	}
	require.NoError(t, json.Unmarshal(payload, &wire))
	require.Empty(t, wire.ErrCode)
	require.Equal(t, now.Year(), wire.Data.Year)
	require.Equal(t, date, wire.Data.TrackingStartDate)
	require.True(t, r.TrackingStartedAt.Equal(wire.Data.TrackingStartedAt))
	require.Equal(t, date, wire.Data.Coverage.Start)
	require.Equal(t, r.Coverage.End, wire.Data.Coverage.End)
	require.EqualValues(t, 60, wire.Data.Activity.CreativeSeconds)
	require.Len(t, wire.Data.Models, 1)
	require.Equal(t, "model", wire.Data.Models[0].Model)
	require.EqualValues(t, 125, wire.Data.Models[0].TotalTokens)
	require.NotContains(t, string(payload), "approval_mode")
	require.NotContains(t, string(payload), "default_model_selection")
}

func TestGetAnnualReportDistinguishesEmptyAndUnavailable(t *testing.T) {
	a := setupTestApp(t)
	now := time.Now()
	result := a.GetAnnualReport(now.Year())
	require.Equal(t, apperr.CodeOK, result.ErrCode)
	require.NotNil(t, result.Data.Coverage)
	require.Zero(t, result.Data.Writing)
	require.Empty(t, result.Data.DailyWriting)

	result = a.GetAnnualReport(now.Year() - 1)
	require.Equal(t, apperr.CodeOK, result.ErrCode)
	require.Nil(t, result.Data.Coverage)
	require.Equal(t, a.settings.TrackingStartDate, result.Data.TrackingStartDate)
	require.Empty(t, result.Data.Models)
	payload, err := json.Marshal(result)
	require.NoError(t, err)
	require.Contains(t, string(payload), `"Coverage":null`)
	require.Contains(t, string(payload), `"DailyWriting":[]`)
	require.Contains(t, string(payload), `"Models":[]`)

	require.NoError(t, a.db.Model(&config.AppSettings{}).Where("id = ?", 1).Update("tracking_started_at", nil).Error)
	result = a.GetAnnualReport(now.Year())
	require.Equal(t, apperr.CodeInternal, result.ErrCode)
	require.Zero(t, result.Data)
	require.NotEmpty(t, result.ErrMsg)
}

func TestGetAnnualReportAdaptsErrors(t *testing.T) {
	a := setupTestApp(t)
	for _, year := range []int{-1, 0, time.Now().Year() + 1} {
		result := a.GetAnnualReport(year)
		require.Equal(t, apperr.CodeInvalid, result.ErrCode)
		require.Zero(t, result.Data)
		require.Contains(t, result.ErrMsg, "报告年份")
	}
	result := (&App{}).GetAnnualReport(time.Now().Year())
	require.Equal(t, apperr.CodeInternal, result.ErrCode)
	require.Zero(t, result.Data)
	require.Contains(t, result.ErrMsg, "尚未就绪")

	require.NoError(t, a.db.Migrator().DropTable(&writing.WritingLog{}))
	result = a.GetAnnualReport(time.Now().Year())
	require.Equal(t, apperr.CodeInternal, result.ErrCode)
	require.Zero(t, result.Data)
	require.Contains(t, result.ErrMsg, "daily writing")
}
