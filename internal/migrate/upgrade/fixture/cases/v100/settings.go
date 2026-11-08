package v100

import (
	"github.com/sigpanic/goink/internal/migrate/upgrade/fixture/seeddata"
)

// 在真实历史 schema 中模拟用户保存选择，仍只通过公开 Run 执行升级。
func Settings() seeddata.SeedCase {
	before := seeddata.SeedData{
		Rows: []seeddata.SeedRow{
			seeddata.Row("app_config", map[string]any{"id": 1, "last_novel_id": 2, "selected_model_key": "deepseek/fixture-deepseek", "reasoning_effort": "high", "approval_mode": "manual"}),
		},
	}
	return seeddata.SeedCase{
		ID:     "settings-legacy",
		Before: before,
		AssertAfter: func(result *seeddata.UpgradeResult) error {
			return seeddata.AssertRows(result, []seeddata.SeedRow{
				seeddata.Row("app_config", map[string]any{"id": 1, "last_novel_id": 2, "default_adapter_name": "openai_chat", "default_provider_name": "DeepSeek", "default_model_id": "fixture-deepseek", "default_reasoning_effort": "high", "approval_mode": "manual"}),
			})
		},
	}
}
