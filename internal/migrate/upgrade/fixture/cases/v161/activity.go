package v161

import (
	"github.com/sigpanic/goink/internal/migrate/upgrade/fixture/seeddata"
)

func Activity() seeddata.SeedCase {
	before := seeddata.SeedData{
		Rows: []seeddata.SeedRow{
			seeddata.Row("activity_daily", map[string]any{"date": "2026-01-01", "novels_created": 2, "chapters_created": 3, "characters_created": 2, "locations_created": 2, "creative_seconds": 600, "conversation_turns": 1, "tool_calls": 2}),
			seeddata.Row("llm_usage_daily", map[string]any{"date": "2026-01-01", "provider": "升级中转站", "model": "vendor/model", "purpose": "chat", "usage_count": 1, "input_tokens": 100, "output_tokens": 50, "total_tokens": 150, "cache_hit_tokens": 0, "cache_miss_tokens": 100, "reasoning_tokens": 0}),
		},
	}
	return seeddata.SeedCase{
		ID:     "activity",
		Before: before,
		AssertAfter: func(result *seeddata.UpgradeResult) error {
			return seeddata.AssertRows(result, before.Rows)
		},
	}
}
