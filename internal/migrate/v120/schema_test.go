package v120

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestNeedsMigrationRejectsMixedColumnSchema(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		`CREATE TABLE chapters (id INTEGER PRIMARY KEY, chapter_number INTEGER NOT NULL)`,
		`CREATE TABLE time_entries (id INTEGER PRIMARY KEY, source_chapter_id INTEGER, source_chapter INTEGER)`,
	} {
		if err := db.Exec(query).Error; err != nil {
			t.Fatal(err)
		}
	}

	if _, err := needsMigration(db); err == nil {
		t.Fatal("新旧字段同时存在时应拒绝猜测 schema")
	}
}

func TestNeedsMigrationScansPastLegacyColumnsForMixedSchema(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		`CREATE TABLE chapters (id INTEGER PRIMARY KEY, chapter_number INTEGER NOT NULL)`,
		`CREATE TABLE time_entries (id INTEGER PRIMARY KEY, source_chapter_id INTEGER)`,
		`CREATE TABLE writing_log (id INTEGER PRIMARY KEY, chapter_id INTEGER, chapter_number INTEGER)`,
	} {
		if err := db.Exec(query).Error; err != nil {
			t.Fatal(err)
		}
	}

	if _, err := needsMigration(db); err == nil {
		t.Fatal("后续表新旧字段同时存在时也应拒绝猜测 schema")
	}
}
