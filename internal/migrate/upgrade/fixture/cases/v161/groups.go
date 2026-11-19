package v161

import (
	"github.com/sigpanic/goink/internal/migrate/upgrade/fixture/seeddata"
)

func Groups() seeddata.SeedCase {
	before := seeddata.SeedData{
		Rows: []seeddata.SeedRow{
			seeddata.Row("character_groups", map[string]any{"id": 1, "novel_id": 1, "name": "主角团", "description": "角色组"}),
			seeddata.Row("character_group_members", map[string]any{"id": 1, "novel_id": 1, "group_id": 1, "character_id": 31}),
			seeddata.Row("character_group_members", map[string]any{"id": 2, "novel_id": 1, "group_id": 1, "character_id": 32}),
		},
	}
	return seeddata.SeedCase{
		ID:     "groups",
		Before: before,
		AssertAfter: func(result *seeddata.UpgradeResult) error {
			return seeddata.AssertRows(result, before.Rows)
		},
	}
}
