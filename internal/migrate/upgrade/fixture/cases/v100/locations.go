package v100

import (
	"github.com/sigpanic/goink/internal/migrate/upgrade/fixture/seeddata"
)

func Locations() seeddata.SeedCase {
	before := seeddata.SeedData{
		Rows: []seeddata.SeedRow{
			seeddata.Row("locations", map[string]any{"id": 41, "novel_id": 1, "name": "山门", "location_type": "建筑", "description": "中文地点"}),
			seeddata.Row("locations", map[string]any{"id": 42, "novel_id": 1, "name": "藏书阁", "location_type": "建筑", "description": "中文地点"}),
			seeddata.Row("location_relations", map[string]any{"id": 1, "novel_id": 1, "location_a": 41, "location_b": 42, "relation_type": "相邻", "description": "地点关系"}),
		},
	}
	return seeddata.SeedCase{
		ID:     "locations",
		Before: before,
		AssertAfter: func(result *seeddata.UpgradeResult) error {
			return seeddata.AssertRows(result, before.Rows)
		},
	}
}
