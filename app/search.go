package app

import "github.com/sigpanic/goink/internal/search"

// SearchAll 执行全局搜索：实体 LIKE + 正文精确 + RAG 语义。
func (a *App) SearchAll(novelID int64, query string) ([]search.Result, error) {
	svc := a.searchService.Load()
	if svc == nil {
		return nil, nil
	}
	return svc.SearchAll(a.ctx, novelID, query)
}
