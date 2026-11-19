package v100

import (
	"github.com/sigpanic/goink/internal/migrate/upgrade/fixture/seeddata"
)

func Novels() seeddata.SeedCase {
	before := seeddata.SeedData{
		Rows: []seeddata.SeedRow{
			seeddata.Row("novels", map[string]any{"id": 1, "title": "升级样本1", "genre": "玄幻", "description": "保留中文与 UTF-8"}),
			seeddata.Row("novels", map[string]any{"id": 2, "title": "升级样本2", "genre": "玄幻", "description": "保留中文与 UTF-8"}),
		},
	}
	return seeddata.SeedCase{
		ID:     "novels",
		Before: before,
		AssertAfter: func(result *seeddata.UpgradeResult) error {
			return seeddata.AssertRows(result, before.Rows)
		},
	}
}
