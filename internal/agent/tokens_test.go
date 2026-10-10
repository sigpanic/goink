package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sigpanic/goink/internal/activity"
	"github.com/sigpanic/goink/internal/llm"
)

func TestRecordTokenUsage(t *testing.T) {
	a := newCompressTestAgent(t, "usage")
	require.NoError(t, a.db.AutoMigrate(&activity.DailyLLMUsage{}))
	require.NoError(t, a.session.UpdateSessionUsage(context.Background(), "usage", `{"total_tokens":9999,"prompt_cache_hit_tokens":9999}`))
	opts := RunOptions{SessionID: "usage", ProviderName: "provider", Model: &llm.ModelInfo{ID: "model"}, AgentType: "main"}
	var raw map[string]any
	require.NoError(t, json.Unmarshal([]byte(`{"prompt_tokens":100,"completion_tokens":20,"total_tokens":125,"prompt_cache_hit_tokens":40,"prompt_cache_miss_tokens":60,"completion_tokens_details":{"reasoning_tokens":5}}`), &raw))
	a.recordTokenUsage(context.Background(), raw, opts)
	a.recordTokenUsage(context.Background(), raw, opts)
	var row activity.DailyLLMUsage
	require.NoError(t, a.db.First(&row).Error)
	require.EqualValues(t, 2, row.UsageCount)
	require.Equal(t, activity.TokenUsage{InputTokens: 200, OutputTokens: 40, TotalTokens: 250, CacheHitTokens: 80, CacheMissTokens: 120, ReasoningTokens: 10}, row.TokenUsage)
	require.Equal(t, "chat", row.Purpose)
	require.Equal(t, float64(40), raw["prompt_cache_hit_tokens"])

	for _, next := range []RunOptions{
		{ProviderName: "other-provider", Model: opts.Model, AgentType: "main"},
		{ProviderName: opts.ProviderName, Model: &llm.ModelInfo{ID: "other-model"}, AgentType: "main"},
		{ProviderName: opts.ProviderName, Model: opts.Model, AgentType: "review"},
	} {
		a.recordTokenUsage(context.Background(), raw, next)
	}
	var rows []activity.DailyLLMUsage
	require.NoError(t, a.db.Find(&rows).Error)
	require.Len(t, rows, 4)
	for _, item := range rows {
		if item.Provider == "provider" && item.Model == "model" && item.Purpose == "chat" {
			continue
		}
		require.EqualValues(t, 1, item.UsageCount)
		require.EqualValues(t, 125, item.TotalTokens)
	}
}

func TestRecordTokenUsageMissingFieldsAndFailure(t *testing.T) {
	for _, tt := range []struct {
		name, raw string
		count     int64
		want      activity.TokenUsage
	}{
		{"missing", `null`, 0, activity.TokenUsage{}},
		{"empty", `{}`, 0, activity.TokenUsage{}},
		{"zero", `{"total_tokens":0}`, 1, activity.TokenUsage{}},
		{"derived total", `{"prompt_tokens":100,"completion_tokens":20,"prompt_tokens_details":{"cached_tokens":40},"completion_tokens_details":{"reasoning_tokens":5}}`, 1, activity.TokenUsage{InputTokens: 100, OutputTokens: 20, TotalTokens: 120, CacheHitTokens: 40, ReasoningTokens: 5}},
		{"partial", `{"prompt_tokens":100}`, 1, activity.TokenUsage{InputTokens: 100}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			a := newCompressTestAgent(t, "usage")
			require.NoError(t, a.db.AutoMigrate(&activity.DailyLLMUsage{}))
			var raw map[string]any
			require.NoError(t, json.Unmarshal([]byte(tt.raw), &raw))
			opts := RunOptions{ProviderName: "provider", Model: &llm.ModelInfo{ID: "model"}, AgentType: "main"}
			a.recordTokenUsage(context.Background(), raw, opts)
			var rows []activity.DailyLLMUsage
			require.NoError(t, a.db.Find(&rows).Error)
			if tt.count == 0 {
				require.Empty(t, rows)
				return
			}
			require.Len(t, rows, 1)
			require.Equal(t, tt.count, rows[0].UsageCount)
			require.Equal(t, tt.want, rows[0].TokenUsage)
			require.NoError(t, a.db.Exec(`CREATE TRIGGER reject_usage BEFORE INSERT ON llm_usage_daily
				BEGIN SELECT RAISE(ABORT, 'statistics unavailable'); END`).Error)
			require.NotPanics(t, func() { a.recordTokenUsage(context.Background(), raw, opts) })
		})
	}
}
