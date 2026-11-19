package v120

import (
	"github.com/sigpanic/goink/internal/migrate/upgrade/fixture/seeddata"
)

func References() seeddata.SeedCase {
	before := seeddata.SeedData{
		Rows: []seeddata.SeedRow{
			seeddata.Row("time_entries", map[string]any{"id": 1, "novel_id": 1, "category": "foreshadowing", "status": "pending", "title": "伏笔", "importance": 3, "target_chapter": 999, "source_chapter": 1, "resolved_chapter": 2}),
			seeddata.Row("time_entries", map[string]any{"id": 2, "novel_id": 2, "category": "foreshadowing", "status": "pending", "title": "伏笔", "importance": 3, "target_chapter": 999, "source_chapter": 1, "resolved_chapter": 0}),
			seeddata.Row("story_arcs", map[string]any{"id": 1, "novel_id": 1, "name": "主线", "arc_type": "main", "status": "active"}),
			seeddata.Row("arc_nodes", map[string]any{"id": 1, "novel_id": 1, "story_arc_id": 1, "title": "弧线节点", "status": "completed", "target_chapter": 999, "actual_chapter": 2}),
			seeddata.Row("reader_perspectives", map[string]any{"id": 1, "novel_id": 1, "type": "suspense", "content": "读者认知", "planted_chapter": 1, "revealed_chapter": 0}),
			seeddata.Row("character_relations", map[string]any{"id": 1, "novel_id": 1, "source_character_id": 31, "target_character_id": 32, "relation_describe": "关系历史", "is_current": true, "chapter_number": 2}),
			seeddata.Row("reader_perspectives", map[string]any{"id": 2, "novel_id": 2, "type": "suspense", "content": "读者认知", "planted_chapter": 99, "revealed_chapter": 1}),
			seeddata.Row("character_relations", map[string]any{"id": 2, "novel_id": 1, "source_character_id": 31, "target_character_id": 32, "relation_describe": "关系历史", "is_current": false, "chapter_number": 0}),
		},
	}
	return seeddata.SeedCase{
		ID:     "references-v120",
		Before: before,
		AssertAfter: func(result *seeddata.UpgradeResult) error {
			return seeddata.AssertRows(result, []seeddata.SeedRow{
				seeddata.Row("time_entries", map[string]any{"id": 1, "novel_id": 1, "category": "foreshadowing", "status": "pending", "title": "伏笔", "importance": 3, "target_reading_number": 999, "source_chapter_id": 11, "resolved_chapter_id": 12}),
				seeddata.Row("time_entries", map[string]any{"id": 2, "novel_id": 2, "category": "foreshadowing", "status": "pending", "title": "伏笔", "importance": 3, "target_reading_number": 999, "source_chapter_id": 21, "resolved_chapter_id": nil}),
				seeddata.Row("story_arcs", map[string]any{"id": 1, "novel_id": 1, "name": "主线", "arc_type": "main", "status": "active"}),
				seeddata.Row("arc_nodes", map[string]any{"id": 1, "novel_id": 1, "story_arc_id": 1, "title": "弧线节点", "status": "completed", "target_reading_number": 999, "actual_chapter_id": 12}),
				seeddata.Row("reader_perspectives", map[string]any{"id": 1, "novel_id": 1, "type": "suspense", "content": "读者认知", "planted_chapter_id": 11, "revealed_chapter_id": nil}),
				seeddata.Row("character_relations", map[string]any{"id": 1, "novel_id": 1, "source_character_id": 31, "target_character_id": 32, "relation_describe": "关系历史", "is_current": true, "chapter_id": 12}),
				seeddata.Row("reader_perspectives", map[string]any{"id": 2, "novel_id": 2, "type": "suspense", "content": "读者认知", "planted_chapter_id": nil, "revealed_chapter_id": 21}),
				seeddata.Row("character_relations", map[string]any{"id": 2, "novel_id": 1, "source_character_id": 31, "target_character_id": 32, "relation_describe": "关系历史", "is_current": false, "chapter_id": nil}),
			})
		},
	}
}
