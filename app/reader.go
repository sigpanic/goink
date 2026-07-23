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

// UpdateReaderPerspectiveInput 是更新读者认知条目的入参。
type UpdateReaderPerspectiveInput struct {
	Type            string `json:"type,omitempty"`
	Content         string `json:"content,omitempty"`
	PlantedChapter  int    `json:"planted_chapter,omitempty"`
	RelatedTruth    string `json:"related_truth,omitempty"`
	RevealedChapter int    `json:"revealed_chapter,omitempty"`
}

// GetReaderPerspectives 返回指定小说的全部读者认知条目，按 planted_chapter 升序排列。
func (a *App) GetReaderPerspectives(novelID int64) ([]reader.ReaderPerspective, error) {
	var all []reader.ReaderPerspective
	page := 1
	for {
		result, err := a.reader.ListByNovel(a.ctx, novelID, reader.ListByNovelOptions{
			PageParams: storage.PageParams{Page: page, Size: 100},
		})
		if err != nil {
			return nil, err
		}
		all = append(all, result.Items...)
		if page >= result.TotalPages {
			break
		}
		page++
	}
	return all, nil
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

// UpdateReaderPerspective 更新一条读者认知条目。
func (a *App) UpdateReaderPerspective(id int64, novelID int64, input UpdateReaderPerspectiveInput) error {
	var item reader.ReaderPerspective
	if err := storage.PatchAndSave(a.reader.DB.WithContext(a.ctx), id, novelID, &input, &item); err != nil {
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
