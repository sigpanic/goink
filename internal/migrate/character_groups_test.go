package migrate

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/character"
)

func TestCharacterGroupsMigrationPreservesExistingCharacters(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, Run(db, slog.Default()))
	require.NoError(t, db.Migrator().DropTable(&character.GroupMember{}, &character.Group{}))
	ch := character.Character{NovelID: 1, Name: "已有角色", Description: "原描述", Personality: `{"role":"主角"}`}
	require.NoError(t, db.Create(&ch).Error)
	for range 2 {
		require.NoError(t, Run(db, slog.Default()))
	}
	var restored character.Character
	require.NoError(t, db.First(&restored, ch.ID).Error)
	require.Equal(t, ch.Name, restored.Name)
	require.Equal(t, ch.Description, restored.Description)
	require.Equal(t, ch.Personality, restored.Personality)
	for _, model := range []any{&character.Group{}, &character.GroupMember{}} {
		var count int64
		require.NoError(t, db.Model(model).Count(&count).Error)
		require.Zero(t, count)
	}
}
