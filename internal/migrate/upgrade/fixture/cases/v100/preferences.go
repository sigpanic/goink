package v100

import (
	"github.com/sigpanic/goink/internal/migrate/upgrade/fixture/seeddata"
)

func Preferences() seeddata.SeedCase {
	before := seeddata.SeedData{
		Rows: []seeddata.SeedRow{
			seeddata.Row("preference_items", map[string]any{"id": 1, "novel_id": 0, "is_global": true, "category": "文风", "content": "使用短句"}),
			seeddata.Row("preference_items", map[string]any{"id": 2, "novel_id": 1, "is_global": false, "category": "文风", "content": "使用短句"}),
		},
	}
	return seeddata.SeedCase{
		ID:     "preferences",
		Before: before,
		AssertAfter: func(result *seeddata.UpgradeResult) error {
			return seeddata.AssertRows(result, before.Rows)
		},
	}
}
