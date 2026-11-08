package v130

import (
	"github.com/sigpanic/goink/internal/migrate/upgrade/fixture/seeddata"
)

func Settings() seeddata.SeedCase {
	before := seeddata.SeedData{
		Rows: []seeddata.SeedRow{
			seeddata.Row("app_config", map[string]any{"id": 1, "last_novel_id": 2, "selected_model_key": "升级中转站/vendor/model", "reasoning_effort": "high", "approval_mode": "manual"}),
		},
	}
	return seeddata.SeedCase{
		ID:     "settings-legacy-relay",
		Before: before,
		AssertAfter: func(result *seeddata.UpgradeResult) error {
			return seeddata.AssertRows(result, []seeddata.SeedRow{
				seeddata.Row("app_config", map[string]any{"id": 1, "last_novel_id": 2, "default_adapter_name": "openai_chat", "default_provider_name": "升级中转站", "default_model_id": "vendor/model", "default_reasoning_effort": "high", "approval_mode": "manual"}),
			})
		},
	}
}
