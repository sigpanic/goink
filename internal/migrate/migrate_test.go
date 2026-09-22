package migrate

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestIsNewDatabaseRequiresNoBusinessTables(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}

	isNew, err := isNewDatabase(db)
	if err != nil {
		t.Fatal(err)
	}
	if !isNew {
		t.Fatal("空数据库应视为新库")
	}

	if err := db.Exec(`CREATE TABLE writing_log (id INTEGER PRIMARY KEY)`).Error; err != nil {
		t.Fatal(err)
	}
	isNew, err = isNewDatabase(db)
	if err != nil {
		t.Fatal(err)
	}
	if isNew {
		t.Fatal("任一业务表存在时不应视为新库")
	}
}
