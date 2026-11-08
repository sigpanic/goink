package v161

import (
	"github.com/sigpanic/goink/internal/migrate/upgrade/fixture/seeddata"
)

func Writing() seeddata.SeedCase {
	before := seeddata.SeedData{
		Rows: []seeddata.SeedRow{
			seeddata.Row("writing_log", map[string]any{"id": 1, "date": "2026-01-01", "novel_id": 1, "word_delta": 50, "words_added": 160, "words_deleted": 110}),
		},
	}
	return seeddata.SeedCase{
		ID:     "writing-daily",
		Before: before,
		AssertAfter: func(result *seeddata.UpgradeResult) error {
			return seeddata.AssertRows(result, []seeddata.SeedRow{
				seeddata.Row("writing_log", map[string]any{"date": "2026-01-01", "novel_id": 1, "word_delta": 50, "words_added": 160, "words_deleted": 110}),
			})
		},
	}
}
