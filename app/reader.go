package app

import (
	"fmt"

	"github.com/sigpanic/goink/internal/reader"
	"github.com/sigpanic/goink/internal/storage"
)

// CreateReaderPerspectiveInput 是创建读者认知条目的入参。
type CreateReaderPerspectiveInput struct {
	Type            string `json:"type"`                       // 必填："known" | "suspense" | "misconception"
	Content         string `json:"content"`                    // 必填
	PlantedChapter  int    `json:"planted_chapter"`            // 必填
	RelatedTruth    string `json:"related_truth,omitempty"`    // 可选
	RevealedChapter int    `json:"revealed_chapter,omitempty"` // 可选，默认 0
}

// UpdateReaderPerspectiveInput 采用 PUT 语义：前端全量传，后端全量覆盖。
type UpdateReaderPerspectiveInput struct {
	Type            string `json:"type"`
	Content         string `json:"content"`
	PlantedChapter  int    `json:"planted_chapter"`
	RelatedTruth    string `json:"related_truth"`
	RevealedChapter int    `json:"revealed_chapter"`
}

// GetReaderPerspectives 返回指定小说的全部读者认知条目，按 type, planted_chapter ASC 排列。
// 4b: 改调 ListByNovel(Size=-1) 一次拉全（废弃循环翻页拉全）。显式传 Order 保持原排序。
func (a *App) GetReaderPerspectives(novelID int64) ([]reader.ReaderPerspective, error) {
	result, err := a.reader.ListByNovel(a.ctx, novelID, reader.ListByNovelOptions{
		PageParams: storage.PageParams{Size: -1},
		Order:      "type, planted_chapter ASC",
	})
	if err != nil {
		return nil, err
	}
	if result.Items == nil {
		return []reader.ReaderPerspective{}, nil
	}
	return result.Items, nil
}

// CreateReaderPerspective 创建一条读者认知条目。
func (a *App) CreateReaderPerspective(novelID int64, input CreateReaderPerspectiveInput) (*reader.ReaderPerspective, error) {
	if input.Type == "" || input.Content == "" {
		return nil, fmt.Errorf("类型和内容不能为空")
	}
	item := reader.ReaderPerspective{
		NovelID:         novelID,
		Type:            input.Type,
		Content:         input.Content,
		PlantedChapter:  input.PlantedChapter,
		RelatedTruth:    input.RelatedTruth,
		RevealedChapter: input.RevealedChapter,
	}
	if err := a.reader.DB.WithContext(a.ctx).Create(&item).Error; err != nil {
		return nil, fmt.Errorf("create reader perspective: %w", err)
	}
	return &item, nil
}

// UpdateReaderPerspective 更新一条读者认知条目。PUT 全量覆盖用户可编辑字段。
func (a *App) UpdateReaderPerspective(id int64, novelID int64, input UpdateReaderPerspectiveInput) error {
	var item reader.ReaderPerspective
	if err := a.reader.DB.WithContext(a.ctx).
		Where("id = ? AND novel_id = ?", id, novelID).
		First(&item).Error; err != nil {
		return fmt.Errorf("update reader perspective: %w", err)
	}
	item.Type = input.Type
	item.Content = input.Content
	item.PlantedChapter = input.PlantedChapter
	item.RelatedTruth = input.RelatedTruth
	item.RevealedChapter = input.RevealedChapter
	if err := a.reader.DB.WithContext(a.ctx).Save(&item).Error; err != nil {
		return fmt.Errorf("update reader perspective: %w", err)
	}
	return nil
}

// DeleteReaderPerspective 删除一条读者认知条目。
func (a *App) DeleteReaderPerspective(id int64, novelID int64) error {
	if err := a.reader.DB.WithContext(a.ctx).
		Where("id = ? AND novel_id = ?", id, novelID).
		Delete(&reader.ReaderPerspective{}).Error; err != nil {
		return fmt.Errorf("delete reader perspective: %w", err)
	}
	return nil
}
