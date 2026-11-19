package v180

import (
	"github.com/sigpanic/goink/internal/migrate/upgrade/fixture/seeddata"
)

func Sessions() seeddata.SeedCase {
	before := seeddata.SeedData{
		Rows: []seeddata.SeedRow{
			seeddata.Row("sessions", map[string]any{"session_id": "upgrade-session", "novel_id": 1, "title": "历史对话", "active_version": 2, "adapter_name": "openai_chat", "provider_name": "升级中转站", "model_id": "vendor/model", "reasoning_effort": "high"}),
		},
	}
	return seeddata.SeedCase{
		ID:     "sessions-native",
		Before: before,
		AssertAfter: func(result *seeddata.UpgradeResult) error {
			return seeddata.AssertRows(result, before.Rows)
		},
	}
}
