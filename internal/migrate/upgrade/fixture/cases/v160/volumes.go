package v160

import (
	"github.com/sigpanic/goink/internal/migrate/upgrade/fixture/seeddata"
)

func Volumes() seeddata.SeedCase {
	before := seeddata.SeedData{
		Rows: []seeddata.SeedRow{
			seeddata.Row("volumes", map[string]any{"id": 1, "novel_id": 1, "name": "第一卷", "sort_order": 1}),
		},
	}
	return seeddata.SeedCase{
		ID:     "volumes",
		Before: before,
		AssertAfter: func(result *seeddata.UpgradeResult) error {
			return seeddata.AssertRows(result, before.Rows)
		},
	}
}
