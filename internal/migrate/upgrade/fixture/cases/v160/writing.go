package v160

import (
	"github.com/sigpanic/goink/internal/migrate/upgrade/fixture/seeddata"
)

func Writing() seeddata.SeedCase {
	before := seeddata.SeedData{
		Rows: []seeddata.SeedRow{
			seeddata.Row("writing_log", map[string]any{"id": 1, "date": "2026-01-01", "novel_id": 1, "chapter_id": 11, "word_delta": 100}),
			seeddata.Row("writing_log", map[string]any{"id": 2, "date": "2026-01-01", "novel_id": 1, "chapter_id": 11, "word_delta": -50}),
			seeddata.Row("writing_log", map[string]any{"id": 3, "date": "2026-01-01", "novel_id": 1, "chapter_id": 11, "word_delta": 0}),
		},
	}
	return seeddata.SeedCase{
		ID:     "writing-id",
		Before: before,
		AssertAfter: func(result *seeddata.UpgradeResult) error {
			return seeddata.AssertRows(result, []seeddata.SeedRow{
				seeddata.Row("writing_log", map[string]any{"date": "2026-01-01", "novel_id": 1, "word_delta": 50, "words_added": 100, "words_deleted": 50}),
			})
		},
	}
}
