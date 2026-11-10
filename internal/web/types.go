package web

import "context"

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
	Title     string `json:"title"`
	URL       string `json:"url"`
	Summary   string `json:"summary"`
	Date      string `json:"date,omitempty"`
	Truncated bool   `json:"truncated"`
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
