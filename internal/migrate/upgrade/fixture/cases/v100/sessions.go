package v100

import (
	"github.com/sigpanic/goink/internal/migrate/upgrade/fixture/seeddata"
)

func Sessions() seeddata.SeedCase {
	before := seeddata.SeedData{
		Rows: []seeddata.SeedRow{
			seeddata.Row("sessions", map[string]any{"session_id": "upgrade-session", "novel_id": 1, "title": "历史对话", "active_version": 2, "model": "vendor/model", "reasoning_effort": "high"}),
		},
	}
	return seeddata.SeedCase{
		ID:     "sessions-legacy",
		Before: before,
		AssertAfter: func(result *seeddata.UpgradeResult) error {
			return seeddata.AssertRows(result, []seeddata.SeedRow{
				seeddata.Row("sessions", map[string]any{"session_id": "upgrade-session", "novel_id": 1, "title": "历史对话", "active_version": 2, "adapter_name": "", "provider_name": "", "model_id": "", "reasoning_effort": ""}),
			})
		},
	}
}
