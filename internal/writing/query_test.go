package writing

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sigpanic/goink/internal/novel"
)

func TestWritingRangeAndNovelRanking(t *testing.T) {
	db := openTestDB(t)
	require.NoError(t, db.AutoMigrate(&novel.Novel{}))
	require.NoError(t, db.Create(&novel.Novel{ID: 1, Title: "现存作品"}).Error)
	require.NoError(t, db.Create(&novel.Novel{ID: 3, Title: "纯删除作品"}).Error)
	require.NoError(t, db.Create(&[]WritingLog{
		{Date: "2023-12-31", NovelID: 1, WordsAdded: 999, WordDelta: 999},
		{Date: "2024-01-01", NovelID: 1, WordsAdded: 100, WordsDeleted: 20, WordDelta: 80},
		{Date: "2024-01-01", NovelID: 2, WordsAdded: 40, WordsDeleted: 40},
		{Date: "2024-02-29", NovelID: 3, WordsDeleted: 50, WordDelta: -50},
		{Date: "2024-03-01", NovelID: 1},
		{Date: "2024-12-31", NovelID: 2, WordsAdded: 60, WordDelta: 60},
		{Date: "2025-01-01", NovelID: 1, WordsAdded: 999, WordDelta: 999},
	}).Error)
	s := NewStore(db, testLogger())
	daily, err := s.GetDailyActivityRange(context.Background(), "2024-01-01", "2025-01-01")
	require.NoError(t, err)
	require.Equal(t, []DailyActivity{
		{Date: "2024-01-01", WordsAdded: 140, WordsDeleted: 60, WordsNet: 80},
		{Date: "2024-02-29", WordsDeleted: 50, WordsNet: -50},
		{Date: "2024-12-31", WordsAdded: 60, WordsNet: 60},
	}, daily)
	ranking, err := s.GetNovelRanking(context.Background(), "2024-01-01", "2025-01-01")
	require.NoError(t, err)
	require.Equal(t, []NovelWords{
		{NovelID: 1, Title: "现存作品", WordsAdded: 100},
		{NovelID: 2, Deleted: true, WordsAdded: 100},
	}, ranking)

	daily, err = s.GetDailyActivityRange(context.Background(), "2024-02-29", "2024-03-01")
	require.NoError(t, err)
	require.Len(t, daily, 1)
	require.Equal(t, "2024-02-29", daily[0].Date)
	ranking, err = s.GetNovelRanking(context.Background(), "2024-02-29", "2024-03-01")
	require.NoError(t, err)
	require.NotNil(t, ranking)
	require.Empty(t, ranking)
	daily, err = s.GetDailyActivityRange(context.Background(), "2022-01-01", "2023-01-01")
	require.NoError(t, err)
	require.NotNil(t, daily)
	require.Empty(t, daily)
}

func TestWritingRangeQueriesReturnErrors(t *testing.T) {
	db := openTestDB(t)
	s := NewStore(db, testLogger())
	_, err := s.GetNovelRanking(context.Background(), "2024-01-01", "2025-01-01")
	require.Error(t, err)
	require.NoError(t, db.Migrator().DropTable(&WritingLog{}))
	_, err = s.GetDailyActivityRange(context.Background(), "2024-01-01", "2025-01-01")
	require.Error(t, err)
}
