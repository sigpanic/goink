package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	deepSeekBaseURL = "https://api.deepseek.com"
	searchCount     = 10
	searchMaxChars  = 15000
)

// DeepSeekClient 使用独立的 Search/Fetch API，不发起模型对话或自动重试。
type DeepSeekClient struct {
	apiKey string
	http   *http.Client
}

var (
	_ Searcher = (*DeepSeekClient)(nil)
	_ Fetcher  = (*DeepSeekClient)(nil)
)

// NewDeepSeekClient 的 httpClient 可用于注入现有 HTTP 传输；nil 使用一分钟超时。
func NewDeepSeekClient(apiKey string, httpClient *http.Client) *DeepSeekClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: time.Minute}
	}
	return &DeepSeekClient{apiKey: strings.TrimSpace(apiKey), http: httpClient}
}

func (c *DeepSeekClient) Search(ctx context.Context, query string) (*SearchResult, error) {
	query = strings.TrimSpace(query)
	if n := utf8.RuneCountInString(query); n < 1 || n > 128 {
		return nil, &BusinessError{Source: SourceDeepSeek, Message: "搜索内容必须为 1～128 个字符，且不能全为空白"}
	}
	request := struct {
		Query       string `json:"query"`
		Count       int    `json:"count"`
		NeedContent bool   `json:"need_content"`
	}{Query: query, Count: searchCount}
	var response struct {
		Results []struct {
			SearchItem
			Snippet string `json:"snippet"`
		} `json:"results"`
	}
	if err := c.do(ctx, "/v1/tools/search", request, &response); err != nil {
		return nil, err
	}
	if response.Results == nil {
		return nil, fmt.Errorf("DeepSeek 搜索响应缺少 results 数组")
	}
	result := &SearchResult{Query: query, Results: make([]SearchItem, 0, len(response.Results))}
	for _, item := range response.Results {
		if strings.TrimSpace(item.Summary) == "" {
			item.Summary = item.Snippet
		}
		if item.Authority != nil && *item.Authority == -1 {
			item.Authority = nil
		}
		result.Results = append(result.Results, item.SearchItem)
	}
	truncateSearchSummaries(result.Results)
	return result, nil
}

// truncateSearchSummaries 仅限制摘要字符总量，标题、链接和日期不计入预算。
func truncateSearchSummaries(items []SearchItem) {
	lengths := make([]int, len(items))
	total, longest := 0, 0
	for i, item := range items {
		lengths[i] = utf8.RuneCountInString(item.Summary)
		total += lengths[i]
		longest = max(longest, lengths[i])
	}
	if total <= searchMaxChars {
		return
	}

	low, high := 0, min(longest, searchMaxChars)
	for low < high {
		mid := low + (high-low+1)/2
		used := 0
		for _, length := range lengths {
			used += min(length, mid)
		}
		if used <= searchMaxChars {
			low = mid
		} else {
			high = mid - 1
		}
	}
	for i := range items {
		if lengths[i] > low {
			items[i].Summary = string([]rune(items[i].Summary)[:low])
			items[i].Truncated = true
		}
	}
}

func (c *DeepSeekClient) Fetch(ctx context.Context, rawURL string) (*FetchResult, error) {
	if utf8.RuneCountInString(rawURL) > 8192 {
		return nil, &BusinessError{Source: SourceDeepSeek, Message: "网页 URL 不能超过 8192 个字符"}
	}
	if _, err := parseFetchURL(rawURL, SourceDeepSeek); err != nil {
		return nil, err
	}
	request := struct {
		URL string `json:"url"`
	}{URL: rawURL}
	var response struct {
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	if err := c.do(ctx, "/v1/tools/fetch", request, &response); err != nil {
		return nil, err
	}
	text := strings.TrimSpace(response.Content)
	if text == "" {
		return nil, fmt.Errorf("DeepSeek 抓取未返回有效正文")
	}
	text, truncated := truncateFetchText(text)
	return &FetchResult{
		URL: rawURL, Title: response.Title, Text: text,
		Mode: RemoteFetch, Truncated: truncated,
	}, nil
}

func (c *DeepSeekClient) do(ctx context.Context, path string, payload, result any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if c.apiKey == "" {
		return &BusinessError{Source: SourceDeepSeek, Message: "DeepSeek API key 未配置"}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("编码 DeepSeek 请求: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, deepSeekBaseURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("创建 DeepSeek 请求: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("DeepSeek 请求失败: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, fetchMaxBytes+1))
	if err != nil {
		return fmt.Errorf("读取 DeepSeek 响应: %w", err)
	}
	if len(data) > fetchMaxBytes {
		return fmt.Errorf("DeepSeek 响应过大，超过 %d MB", fetchMaxBytes>>20)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var response struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(data, &response)
		if response.Error.Message == "" {
			response.Error.Message = http.StatusText(resp.StatusCode)
		}
		if isBusinessStatus(resp.StatusCode) {
			return &BusinessError{
				HTTPCode: resp.StatusCode,
				APICode:  response.Error.Code,
				Message:  deepSeekBusinessMessage(resp.StatusCode),
				Source:   SourceDeepSeek,
			}
		}
		return fmt.Errorf("DeepSeek API HTTP %d (code=%q): %s", resp.StatusCode, response.Error.Code, response.Error.Message)
	}
	if err := json.Unmarshal(data, result); err != nil {
		return fmt.Errorf("解析 DeepSeek 响应: %w", err)
	}
	return ctx.Err()
}

func deepSeekBusinessMessage(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "DeepSeek 服务拒绝当前请求，请调整参数"
	case http.StatusUnauthorized:
		return "DeepSeek API Key 无效，请检查配置"
	case http.StatusPaymentRequired:
		return "DeepSeek 余额不足，请充值后重试"
	case http.StatusForbidden:
		return "DeepSeek 服务的安全策略拒绝了当前请求"
	case http.StatusNotFound:
		return "DeepSeek 服务未获取到可用内容，请更换来源"
	case http.StatusRequestEntityTooLarge:
		return "请求内容超过 DeepSeek 服务的大小限制"
	default:
		return fmt.Sprintf("DeepSeek 服务拒绝当前请求（HTTP %d），请调整参数、来源或配置", status)
	}
}
