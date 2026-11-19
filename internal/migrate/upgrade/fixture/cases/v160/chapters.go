package v160

import (
	"github.com/sigpanic/goink/internal/migrate/upgrade/fixture/seeddata"
)

func Chapters() seeddata.SeedCase {
	before := seeddata.SeedData{
		Rows: []seeddata.SeedRow{
			seeddata.Row("chapters", map[string]any{"id": 11, "novel_id": 1, "title": "章节11", "summary": "章节摘要", "word_count": 8, "sort_order": 1, "volume_id": nil}),
			seeddata.Row("chapters", map[string]any{"id": 12, "novel_id": 1, "title": "章节12", "summary": "章节摘要", "word_count": 8, "sort_order": 2, "volume_id": 1}),
			seeddata.Row("chapters", map[string]any{"id": 21, "novel_id": 2, "title": "章节21", "summary": "章节摘要", "word_count": 8, "sort_order": 1, "volume_id": nil}),
		},
		Files: map[string]string{
			"novels/1/chapters/id_11.md": "chapters：小说1，章节11\n",
			"novels/1/outlines/id_11.md": "outlines：小说1，章节11\n",
			"novels/1/chapters/id_12.md": "chapters：小说1，章节12\n",
			"novels/1/outlines/id_12.md": "outlines：小说1，章节12\n",
			"novels/2/chapters/id_21.md": "chapters：小说2，章节21\n",
			"novels/2/outlines/id_21.md": "outlines：小说2，章节21\n",
		},
	}
	return seeddata.SeedCase{
		ID:     "chapters-id",
		Before: before,
		AssertAfter: func(result *seeddata.UpgradeResult) error {
			if err := seeddata.AssertRows(result, before.Rows); err != nil {
				return err
			}
			return seeddata.AssertFiles(result, before.Files)
		},
	}
}
