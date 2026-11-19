package v130

import (
	"github.com/sigpanic/goink/internal/migrate/upgrade/fixture/seeddata"
)

func SettingItems() seeddata.SeedCase {
	before := seeddata.SeedData{
		Rows: []seeddata.SeedRow{
			seeddata.Row("setting_items", map[string]any{"id": 1, "novel_id": 1, "category": "世界观", "content": "灵气随潮汐涨落"}),
		},
	}
	return seeddata.SeedCase{
		ID:     "setting-items",
		Before: before,
		AssertAfter: func(result *seeddata.UpgradeResult) error {
			return seeddata.AssertRows(result, before.Rows)
		},
	}
}
