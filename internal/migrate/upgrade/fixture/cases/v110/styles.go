package v110

import (
	"github.com/sigpanic/goink/internal/migrate/upgrade/fixture/seeddata"
)

func Styles() seeddata.SeedCase {
	before := seeddata.SeedData{
		Rows: []seeddata.SeedRow{
			seeddata.Row("style_samples", map[string]any{"id": 1, "novel_id": 0, "is_global": true, "name": "短句样本", "content": "风起。云散。", "preview": "风起。", "tags": "[\"短句\"]", "word_count": 4}),
		},
	}
	return seeddata.SeedCase{
		ID:     "styles",
		Before: before,
		AssertAfter: func(result *seeddata.UpgradeResult) error {
			return seeddata.AssertRows(result, before.Rows)
		},
	}
}
