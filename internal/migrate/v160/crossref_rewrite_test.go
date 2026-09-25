//go:build cgo

package v160_test

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/sigpanic/goink/internal/migrate"
)

// TestFullLegacyMigration 从 v1.6.0 前的实际表结构开始，只调用公开入口 migrate.Run：
// 旧 *_chapter_id 仍存章节号；迁移须完成历史字段改名、num→id 重写、文件改名、删列，
// 并留下可供当前 model 使用的 schema。
func TestFullLegacyMigration(t *testing.T) {
	dataDir := setupMigrateEnv(t)
	db := openMigrateDB(t)
	log := slog.Default()

	// 真正的旧版 schema，不预先添加 v1.6.0 的任何新列。
	models := []any{
		&legacyNovel{}, &legacyChapter{},
		&legacyTimelineEntry{}, &legacyStoryArc{}, &legacyArcNode{},
		&legacyReaderPerspective{}, &legacyWritingLog{}, &legacyCharacterRelation{},
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
	// 老数据：novel1 章 1/2/3（id 1/2/3），novel2 章 1（id 4，全局自增）
	exec(`INSERT INTO novels (id, title) VALUES (1,'n1'),(2,'n2')`)
	exec(`INSERT INTO chapters (id, novel_id, chapter_number, title) VALUES
		(1,1,1,'c1'),(2,1,2,'c2'),(3,1,3,'c3'),(4,2,1,'c1')`)

	// time_entries：旧 source/resolved 的 _id 后缀是历史误名，存的仍是章节号。
	exec(`INSERT INTO time_entries (id, novel_id, category, status, title, target_chapter, importance, source_chapter_id, resolved_chapter_id) VALUES
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
	// writing_log：旧 chapter_id 实际存章节号；行2 num=0（未定义）保持 NULL。
	exec(`INSERT INTO writing_log (id, date, novel_id, chapter_id, word_delta) VALUES
		(1,'2026-01-01',1,2,100),
		(2,'2026-01-01',1,0,-50)`)
	// character_relations：旧 chapter_id 实际存章节号；行1 num=3→id3；行2 num=0 保持 NULL。
	exec(`INSERT INTO character_relations (id, novel_id, source_character_id, target_character_id, relation_describe, chapter_id, is_current) VALUES
		(1,1,1,2,'朋友',3,1),
		(2,1,1,2,'旧识',0,0)`)

	// 真实小说仓库中的旧文件名，验证数据迁移与文件迁移在同一次 Run 中共同完成。
	novelDir := filepath.Join(dataDir, "novels", "1")
	for rel, content := range map[string]string{
		"chapters/001.md": "chapter one",
		"outlines/001.md": "outline one",
	} {
		path := filepath.Join(novelDir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

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
	// 文件改名与旧列/索引清理：数据已经完成转换后，最终 schema 不再保留 num 作为回退来源。
	if _, err := os.Stat(filepath.Join(novelDir, "chapters", "id_1.md")); err != nil {
		t.Fatalf("章节文件未改名: %v", err)
	}
	if _, err := os.Stat(filepath.Join(novelDir, "outlines", "id_1.md")); err != nil {
		t.Fatalf("大纲文件未改名: %v", err)
	}
	for _, legacy := range []struct{ table, column string }{
		{"chapters", "chapter_number"},
		{"time_entries", "target_chapter"}, {"time_entries", "source_chapter"}, {"time_entries", "resolved_chapter"},
		{"arc_nodes", "target_chapter"}, {"arc_nodes", "actual_chapter"},
		{"reader_perspectives", "planted_chapter"}, {"reader_perspectives", "revealed_chapter"},
		{"writing_log", "chapter_number"}, {"character_relations", "chapter_number"},
	} {
		if db.Migrator().HasColumn(legacy.table, legacy.column) {
			t.Fatalf("旧列仍存在: %s.%s", legacy.table, legacy.column)
		}
	}
	for _, index := range []struct{ table, name string }{
		{"chapters", "uk_novel_chapter"},
	} {
		if db.Migrator().HasIndex(index.table, index.name) {
			t.Fatalf("旧索引仍存在: %s.%s", index.table, index.name)
		}
	}
	if !db.Migrator().HasIndex("writing_log", "idx_writing_log_chapter_id") {
		t.Fatal("writing_log.chapter_id 的当前索引应在迁移后存在")
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

	// migrate_state 丢失后仍不能把最终的 chapter_id 误改成旧章节号列。
	if err := db.Exec("DELETE FROM migrate_state").Error; err != nil {
		t.Fatal(err)
	}
	if err := migrate.Run(db, log); err != nil {
		t.Fatalf("状态丢失后 migrate.Run: %v", err)
	}
	if !db.Migrator().HasColumn("time_entries", "source_chapter_id") ||
		db.Migrator().HasColumn("time_entries", "source_chapter") ||
		!db.Migrator().HasColumn("writing_log", "chapter_id") ||
		db.Migrator().HasColumn("writing_log", "chapter_number") {
		t.Fatal("状态丢失后最终 id 列不应被改回旧 num 列")
	}
}

// 下列结构体精确保留 v1.6.0 之前的章节关联命名：source_chapter_id、
// writing_log.chapter_id 和 character_relations.chapter_id 的值都是章节号，
// 用于验证 migrate.Run 最开始的历史字段改名没有被绕过。
type legacyNovel struct {
	ID    int64  `gorm:"column:id;primaryKey;autoIncrement"`
	Title string `gorm:"column:title;not null;index"`
}

func (legacyNovel) TableName() string { return "novels" }

type legacyChapter struct {
	ID            int64  `gorm:"column:id;primaryKey;autoIncrement"`
	NovelID       int64  `gorm:"column:novel_id;not null;uniqueIndex:uk_novel_chapter;index"`
	ChapterNumber int    `gorm:"column:chapter_number;not null;uniqueIndex:uk_novel_chapter"`
	Title         string `gorm:"column:title"`
}

func (legacyChapter) TableName() string { return "chapters" }

type legacyTimelineEntry struct {
	ID                int64  `gorm:"column:id;primaryKey;autoIncrement"`
	NovelID           int64  `gorm:"column:novel_id;not null;index"`
	Category          string `gorm:"column:category;not null;index"`
	Status            string `gorm:"column:status;not null;index"`
	Title             string `gorm:"column:title;not null"`
	TargetChapter     int    `gorm:"column:target_chapter;not null"`
	Importance        int    `gorm:"column:importance;default:3"`
	SourceChapterID   int64  `gorm:"column:source_chapter_id"`
	ResolvedChapterID int64  `gorm:"column:resolved_chapter_id"`
}

func (legacyTimelineEntry) TableName() string { return "time_entries" }

type legacyStoryArc struct {
	ID      int64  `gorm:"column:id;primaryKey;autoIncrement"`
	NovelID int64  `gorm:"column:novel_id;not null;index"`
	Name    string `gorm:"column:name;not null"`
	ArcType string `gorm:"column:arc_type;not null;index"`
	Status  string `gorm:"column:status;not null;index"`
}

func (legacyStoryArc) TableName() string { return "story_arcs" }

type legacyArcNode struct {
	ID            int64  `gorm:"column:id;primaryKey;autoIncrement"`
	NovelID       int64  `gorm:"column:novel_id;not null;index"`
	StoryArcID    int64  `gorm:"column:story_arc_id;not null;index"`
	Title         string `gorm:"column:title;not null"`
	TargetChapter int    `gorm:"column:target_chapter;default:0"`
	ActualChapter int    `gorm:"column:actual_chapter;default:0"`
	Status        string `gorm:"column:status;not null;default:pending"`
}

func (legacyArcNode) TableName() string { return "arc_nodes" }

type legacyReaderPerspective struct {
	ID              int64  `gorm:"column:id;primaryKey;autoIncrement"`
	NovelID         int64  `gorm:"column:novel_id;not null;index"`
	Type            string `gorm:"column:type;not null;index"`
	Content         string `gorm:"column:content;not null"`
	PlantedChapter  int    `gorm:"column:planted_chapter;not null"`
	RevealedChapter int    `gorm:"column:revealed_chapter;default:0"`
}

func (legacyReaderPerspective) TableName() string { return "reader_perspectives" }

type legacyWritingLog struct {
	ID        int64  `gorm:"column:id;primaryKey;autoIncrement"`
	Date      string `gorm:"column:date;not null;index:idx_writing_date;size:10"`
	NovelID   int64  `gorm:"column:novel_id;not null;default:0;index"`
	ChapterID int64  `gorm:"column:chapter_id;not null;default:0;index"`
	WordDelta int    `gorm:"column:word_delta;not null"`
}

func (legacyWritingLog) TableName() string { return "writing_log" }

type legacyCharacterRelation struct {
	ID                int64  `gorm:"column:id;primaryKey;autoIncrement"`
	NovelID           int64  `gorm:"column:novel_id;not null;index"`
	SourceCharacterID int64  `gorm:"column:source_character_id;not null;index"`
	TargetCharacterID int64  `gorm:"column:target_character_id;not null;index"`
	RelationDescribe  string `gorm:"column:relation_describe;not null"`
	ChapterID         int64  `gorm:"column:chapter_id"`
	IsCurrent         bool   `gorm:"column:is_current;not null;index"`
}

func (legacyCharacterRelation) TableName() string { return "character_relations" }
