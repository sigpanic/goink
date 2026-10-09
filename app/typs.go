package app

import "github.com/sigpanic/goink/internal/storage"

// ListInput 是按小说分页查询的公共入参，排序由各接口在后端指定。
type ListInput struct {
	NovelID int64 `json:"novel_id"`
	storage.PageParams
	Search string `json:"search"`
}
