package seeddata

type SeedCase struct {
	ID          string
	Before      SeedData
	AssertAfter func(*UpgradeResult) error
}

func Row(table string, values map[string]any) SeedRow {
	switch table {
	case "app_config", "activity_daily", "llm_usage_daily":
	default:
		values["created_at"] = "2026-01-01 00:00:00+00:00"
	}
	switch table {
	case "novels", "chapters", "characters", "time_entries", "story_arcs", "arc_nodes", "sessions", "locations", "location_relations", "style_samples", "setting_items", "volumes", "character_groups":
		values["updated_at"] = "2026-01-01 00:00:00+00:00"
	}
	return SeedRow{Table: table, Values: values}
}
