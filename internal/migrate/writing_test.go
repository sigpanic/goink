//go:build cgo

package migrate_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/sigpanic/goink/internal/migrate"
	"github.com/sigpanic/goink/internal/writing"
)

func TestWritingDailyMigrationPreservesCounts(t *testing.T) {
	for _, withDiff := range []bool{false, true} {
		name := "legacy deltas"
		if withDiff {
			name = "mixed diff and legacy"
		}
		t.Run(name, func(t *testing.T) {
			setupMigrationIntegrationEnv(t)
			db := openMigrationIntegrationDB(t)
			require.NoError(t, db.Exec(`CREATE TABLE writing_log (
				id INTEGER PRIMARY KEY, date TEXT NOT NULL, novel_id INTEGER NOT NULL,
				chapter_id INTEGER, word_delta INTEGER NOT NULL, created_at DATETIME
			)`).Error)
			today := time.Now().Format(time.DateOnly)
			yesterday := time.Now().AddDate(0, 0, -1).Format(time.DateOnly)
			require.NoError(t, db.Exec(`INSERT INTO writing_log (date, novel_id, chapter_id, word_delta, created_at) VALUES
				(?, 1, 10, 100, '2026-01-01 01:00:00'), (?, 1, 11, -50, '2026-01-01 02:00:00'),
				(?, 2, 20, 70, '2026-01-01 03:00:00'), (?, 1, NULL, -80, '2026-01-01 04:00:00')`,
				today, today, today, yesterday).Error)
			if withDiff {
				require.NoError(t, db.Exec(`ALTER TABLE writing_log ADD COLUMN words_added INTEGER`).Error)
				require.NoError(t, db.Exec(`ALTER TABLE writing_log ADD COLUMN words_deleted INTEGER`).Error)
				require.NoError(t, db.Exec(`INSERT INTO writing_log
					(date, novel_id, chapter_id, word_delta, words_added, words_deleted) VALUES
					(?, 1, 10, 50, 250, 200), (?, 1, 10, 0, 120, 120), (?, 1, 10, -30, 0, 30)`,
					today, today, today).Error)
			}
			var before []writing.DailyActivity
			added, deleted := "MAX(word_delta, 0)", "MAX(-word_delta, 0)"
			if withDiff {
				added, deleted = "COALESCE(words_added, "+added+")", "COALESCE(words_deleted, "+deleted+")"
			}
			require.NoError(t, db.Table("writing_log").Select("date, SUM(word_delta) AS words_net, SUM("+added+") AS words_added, SUM("+deleted+") AS words_deleted").Group("date").Order("date").Scan(&before).Error)

			require.NoError(t, migrate.Run(db, slog.Default()))
			require.False(t, db.Migrator().HasColumn("writing_log", "chapter_id"))
			require.True(t, db.Migrator().HasIndex("writing_log", "uk_writing_date_novel"))
			store := writing.NewStore(db, slog.Default())
			after, err := store.GetDailyActivity(context.Background(), 1)
			require.NoError(t, err)
			require.Equal(t, before, after)
			var records []writing.WritingLog
			require.NoError(t, db.Order("date, novel_id").Find(&records).Error)
			require.Len(t, records, 3)
			require.Equal(t, 70, records[2].WordsAdded)
			require.Equal(t, 80, records[0].WordsDeleted)
			require.Equal(t, time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC), records[1].CreatedAt)
			stats, err := store.GetWritingStats(context.Background(), 2, 3)
			require.NoError(t, err)
			require.Equal(t, before[0].WordsAdded+before[1].WordsAdded, stats.TotalWords)
			require.Equal(t, 2, stats.TotalDaysActive)
			require.Equal(t, 2, stats.CurrentStreak)

			store.LogChanges(context.Background(), 1, 12, 30, 20)
			after[1].WordsAdded += 30
			after[1].WordsDeleted += 20
			after[1].WordsNet += 10
			for range 2 {
				require.NoError(t, migrate.Run(db, slog.Default()))
				actual, err := store.GetDailyActivity(context.Background(), 1)
				require.NoError(t, err)
				require.Equal(t, after, actual)
				require.NoError(t, db.Exec("DELETE FROM migrate_state WHERE migration = 'v1.6.1-writing-daily'").Error)
			}
			var count int64
			require.NoError(t, db.Model(&writing.WritingLog{}).Count(&count).Error)
			require.EqualValues(t, 3, count)
		})
	}
}
