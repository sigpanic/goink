package v100

import (
	"github.com/sigpanic/goink/internal/migrate/upgrade/fixture/seeddata"
)

func Characters() seeddata.SeedCase {
	before := seeddata.SeedData{
		Rows: []seeddata.SeedRow{
			seeddata.Row("characters", map[string]any{"id": 31, "novel_id": 1, "name": "角色31"}),
			seeddata.Row("characters", map[string]any{"id": 32, "novel_id": 1, "name": "角色32"}),
		},
	}
	return seeddata.SeedCase{
		ID:     "characters",
		Before: before,
		AssertAfter: func(result *seeddata.UpgradeResult) error {
			return seeddata.AssertRows(result, before.Rows)
		},
	}
}
