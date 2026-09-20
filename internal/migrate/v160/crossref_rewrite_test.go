//go:build cgo

package v160_test

import (
	"log/slog"
	"testing"

	"github.com/sigpanic/goink/internal/chapter"
	"github.com/sigpanic/goink/internal/character"
	"github.com/sigpanic/goink/internal/migrate"
	"github.com/sigpanic/goink/internal/novel"
	"github.com/sigpanic/goink/internal/reader"
	"github.com/sigpanic/goink/internal/storyarc"
	"github.com/sigpanic/goink/internal/timeline"
	"github.com/sigpanic/goink/internal/writing"
)

// TestCrossrefRewriteMigration 模拟"1.1-1.4 已跑、1.5 未跑"的老用户库：
// schema 已是迁移期状态（新列已存在但全 NULL / 0），数据仍是旧 num 引用。
// 用真实 model AutoMigrate 建表 + 真实旧数据，完整跑 migrate.Run 链路，
// 验证 5 张交叉引用表 num→id 重写，覆盖正常反查 / 孤儿 / num<=0 / NULL / 跨 novel / 幂等重跑。
func TestCrossrefRewriteMigration(t *testing.T) {
	setupMigrateEnv(t)
	db := openMigrateDB(t)
	log := slog.Default()

	// 真实 schema（迁移期双字段共存状态：新列已由 1.2 加好）
	models := []any{
		&novel.Novel{}, &chapter.Chapter{},
		&timeline.TimelineEntry{}, &storyarc.StoryArc{}, &storyarc.ArcNode{},
		&reader.ReaderPerspective{}, &writing.WritingLog{}, &character.CharacterRelation{},
	}
	for _, m := range models {
		if err := db.AutoMigrate(m); err != nil {
			t.Fatalf("AutoMigrate %T: %v", m, err)
		}
	}

	exec := func(sql string) {
		t.Helper()
		if err := db.Exec(sql).Error; err != nil {
			t.Fatalf("exec: %s\n%v", sql, err)
		}
	}
	// 模拟 v1.6.0 前的持久化列；当前 model 只声明迁移后的字段。
	for _, sql := range []string{
		`ALTER TABLE chapters ADD COLUMN chapter_number INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE time_entries ADD COLUMN target_chapter INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE time_entries ADD COLUMN source_chapter INTEGER DEFAULT 0`,
		`ALTER TABLE time_entries ADD COLUMN resolved_chapter INTEGER DEFAULT 0`,
		`ALTER TABLE arc_nodes ADD COLUMN target_chapter INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE arc_nodes ADD COLUMN actual_chapter INTEGER DEFAULT 0`,
		`ALTER TABLE reader_perspectives ADD COLUMN planted_chapter INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE reader_perspectives ADD COLUMN revealed_chapter INTEGER DEFAULT 0`,
		`ALTER TABLE writing_log ADD COLUMN chapter_number INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE character_relations ADD COLUMN chapter_number INTEGER`,
	} {
		exec(sql)
	}

	// 老数据：novel1 章 1/2/3（id 1/2/3），novel2 章 1（id 4，全局自增）
	exec(`INSERT INTO novels (id, title) VALUES (1,'n1'),(2,'n2')`)
	exec(`INSERT INTO chapters (id, novel_id, chapter_number, sort_order, title) VALUES
		(1,1,1,0,'c1'),(2,1,2,0,'c2'),(3,1,3,0,'c3'),(4,2,1,0,'c1')`)

	// time_entries：target 保留为未来计划阅读位置；source/resolved 反查为稳定 ID。
	exec(`INSERT INTO time_entries (id, novel_id, category, status, title, target_chapter, importance, source_chapter, resolved_chapter) VALUES
		(1,1,'foreshadowing','pending','f1',2,3,1,0),
		(2,1,'foreshadowing','resolved','f2',99,3,3,2),
		(3,2,'foreshadowing','pending','f3',1,3,1,0)`)
	// arc_nodes：target 保留为未来计划阅读位置，actual 反查为稳定 ID。
	exec(`INSERT INTO arc_nodes (id, novel_id, story_arc_id, title, target_chapter, actual_chapter, status) VALUES
		(1,1,1,'a1',3,0,'pending'),
		(2,1,1,'a2',99,1,'pending')`)
	// reader_perspectives：行1 planted=1→id1, revealed=0 保持 NULL；行2 孤儿 planted=99→NULL, revealed=1→id4（novel2）
	exec(`INSERT INTO reader_perspectives (id, novel_id, type, content, planted_chapter, revealed_chapter) VALUES
		(1,1,'known','p1',1,0),
		(2,2,'suspense','p2',99,1)`)
	// writing_log：行1 num=2→id2；行2 num=0（未定义）保持 NULL
	exec(`INSERT INTO writing_log (id, date, novel_id, chapter_number, word_delta) VALUES
		(1,'2026-01-01',1,2,100),
		(2,'2026-01-01',1,0,-50)`)
	// character_relations：行1 num=3→id3；行2 num=NULL 保持 NULL
	exec(`INSERT INTO character_relations (id, novel_id, source_character_id, target_character_id, relation_describe, chapter_number, is_current) VALUES
		(1,1,1,2,'朋友',3,1),
		(2,1,1,2,'旧识',NULL,0)`)

	// 完整迁移链路（含 1.5 数据重写 + 1.6 文件迁移）
	if err := migrate.Run(db, log); err != nil {
		t.Fatalf("migrate.Run: %v", err)
	}

	count := func(q string) int {
		t.Helper()
		var n int
		if err := db.Raw(q).Scan(&n).Error; err != nil {
			t.Fatalf("query: %s\n%v", q, err)
		}
		return n
	}

	// time_entries
	if n := count(`SELECT COUNT(*) FROM time_entries WHERE id=1 AND target_reading_number=2 AND source_chapter_id=1 AND resolved_chapter_id IS NULL`); n != 1 {
		t.Fatalf("time_entries 行1 错误: n=%d", n)
	}
	if n := count(`SELECT COUNT(*) FROM time_entries WHERE id=2 AND target_reading_number=99 AND source_chapter_id=3 AND resolved_chapter_id=2`); n != 1 {
		t.Fatalf("time_entries 行2（孤儿/填充）错误: n=%d", n)
	}
	if n := count(`SELECT COUNT(*) FROM time_entries WHERE id=3 AND target_reading_number=1 AND source_chapter_id=4`); n != 1 {
		t.Fatalf("time_entries 行3（跨 novel）错误: n=%d", n)
	}
	// arc_nodes
	if n := count(`SELECT COUNT(*) FROM arc_nodes WHERE id=1 AND target_reading_number=3 AND actual_chapter_id IS NULL`); n != 1 {
		t.Fatalf("arc_nodes 行1 错误: n=%d", n)
	}
	if n := count(`SELECT COUNT(*) FROM arc_nodes WHERE id=2 AND target_reading_number=99 AND actual_chapter_id=1`); n != 1 {
		t.Fatalf("arc_nodes 行2（孤儿）错误: n=%d", n)
	}
	// reader_perspectives
	if n := count(`SELECT COUNT(*) FROM reader_perspectives WHERE id=1 AND planted_chapter_id=1 AND revealed_chapter_id IS NULL`); n != 1 {
		t.Fatalf("reader_perspectives 行1 错误: n=%d", n)
	}
	if n := count(`SELECT COUNT(*) FROM reader_perspectives WHERE id=2 AND planted_chapter_id IS NULL AND revealed_chapter_id=4`); n != 1 {
		t.Fatalf("reader_perspectives 行2（孤儿/跨 novel）错误: n=%d", n)
	}
	// writing_log
	if n := count(`SELECT COUNT(*) FROM writing_log WHERE id=1 AND chapter_id=2`); n != 1 {
		t.Fatalf("writing_log 行1 错误: n=%d", n)
	}
	if n := count(`SELECT COUNT(*) FROM writing_log WHERE id=2 AND chapter_id IS NULL`); n != 1 {
		t.Fatalf("writing_log 行2（num=0）错误: n=%d", n)
	}
	// character_relations
	if n := count(`SELECT COUNT(*) FROM character_relations WHERE id=1 AND chapter_id=3`); n != 1 {
		t.Fatalf("character_relations 行1 错误: n=%d", n)
	}
	if n := count(`SELECT COUNT(*) FROM character_relations WHERE id=2 AND chapter_id IS NULL`); n != 1 {
		t.Fatalf("character_relations 行2（NULL）错误: n=%d", n)
	}

	// 幂等重跑：已填充的不重复改，结果不变
	if err := migrate.Run(db, log); err != nil {
		t.Fatalf("幂等重跑 migrate.Run: %v", err)
	}
	if n := count(`SELECT COUNT(*) FROM time_entries WHERE target_reading_number=2`); n != 1 {
		t.Fatalf("幂等重跑后数据被重复修改: n=%d", n)
	}
	if n := count(`SELECT COUNT(*) FROM reader_perspectives WHERE revealed_chapter_id=4`); n != 1 {
		t.Fatalf("幂等重跑后 reader_perspectives 错误: n=%d", n)
	}
}
