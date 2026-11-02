package writing

import "context"

// NovelWords 按期间累计新增字数排名；作品被删除后仍保留 ID 和累计字数。
// Title 是现存作品的当前标题，Deleted 为真时由展示层提供本地化占位文案。
type NovelWords struct {
	NovelID    int64
	Title      string
	Deleted    bool
	WordsAdded int
}

// GetDailyActivityRange 返回本地日期 [start, end) 内的每日增删量，不改变个人页滚动查询。
func (s *Store) GetDailyActivityRange(ctx context.Context, start, end string) ([]DailyActivity, error) {
	rows := make([]DailyActivity, 0)
	err := s.DB.WithContext(ctx).Model(&WritingLog{}).
		Select("date, SUM(word_delta) AS words_net, SUM(words_added) AS words_added, SUM(words_deleted) AS words_deleted").
		Where("date >= ? AND date < ? AND "+editedWordsExpr+" > 0", start, end).
		Group("date").Order("date ASC").Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// GetNovelRanking 汇总本地日期 [start, end) 内有新增文字的作品，字数相同时按作品 ID 排序。
func (s *Store) GetNovelRanking(ctx context.Context, start, end string) ([]NovelWords, error) {
	rows := make([]NovelWords, 0)
	err := s.DB.WithContext(ctx).Model(&WritingLog{}).
		Select("writing_log.novel_id, COALESCE(novels.title, '') AS title, novels.id IS NULL AS deleted, SUM(writing_log.words_added) AS words_added").
		Joins("LEFT JOIN novels ON novels.id = writing_log.novel_id").
		Where("writing_log.date >= ? AND writing_log.date < ?", start, end).
		Group("writing_log.novel_id, novels.id, novels.title").
		Having("SUM(writing_log.words_added) > 0").
		Order("words_added DESC, writing_log.novel_id ASC").Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}
