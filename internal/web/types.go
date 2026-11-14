package web

import (
	"context"
	"net/http"
)

type SourceType string

const (
	SourceDeepSeek SourceType = "deepseek"
	SourceLocal    SourceType = "local"
)

// BusinessError 表示已确认的业务失败；Message 是来源实现生成的可直接展示的中文结论。
// HTTPCode 和 APICode 在没有对应响应时为零值；底层依赖异常使用普通 error 保留错误链。
type BusinessError struct {
	HTTPCode int
	APICode  string
	Message  string
	Source   SourceType
}

func (e *BusinessError) Error() string { return e.Message }

// isBusinessStatus 将确定性 HTTP 拒绝与超时、限流和服务异常区分。
func isBusinessStatus(status int) bool {
	return status >= 400 && status < 500 && status != http.StatusRequestTimeout && status != http.StatusTooManyRequests
}

type FetchMode string

const (
	LocalFetch  FetchMode = "local"
	RemoteFetch FetchMode = "remote"
)

type SearchResult struct {
	Query   string       `json:"query"`
	Results []SearchItem `json:"results"`
}

type SearchItem struct {
	Title       string   `json:"title"`
	URL         string   `json:"url"`
	Summary     string   `json:"summary"`
	Date        string   `json:"date,omitempty"`
	Truncated   bool     `json:"truncated"`
	Authority   *int     `json:"authority,omitempty"`
	RerankScore *float64 `json:"rerank_score,omitempty"`
}

// FetchResult 是网页抓取结果。
type FetchResult struct {
	URL       string    `json:"url"`
	Title     string    `json:"title"`
	Text      string    `json:"text"`
	Mode      FetchMode `json:"mode"`
	Truncated bool      `json:"truncated"`
}

type Searcher interface {
	Search(ctx context.Context, query string) (*SearchResult, error)
}

type Fetcher interface {
	Fetch(ctx context.Context, url string) (*FetchResult, error)
}
