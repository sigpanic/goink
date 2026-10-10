package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// WebSearchResult 是一次 web 搜索的结果。
type WebSearchResult struct {
	Queries []string     `json:"queries"`
	Summary string       `json:"summary"`
	Sources []SourceItem `json:"sources"`
}

// SourceItem 是单条搜索结果的元信息。
type SourceItem struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

const (
	anthropicSearchURL     = "https://api.deepseek.com/anthropic/v1/messages"
	anthropicSearchVersion = "2023-06-01"
	webSearchTimeout       = 120 * time.Second
	webSearchMaxTokens     = 16384
	webSearchToolType      = "web_search_20260209"
	webSearchToolName      = "web_search"
)

// SearchWeb 通过 DeepSeek Anthropic 端点执行一次带搜索的对话。
// apiKey 是 DeepSeek 的 API key，model 如 "deepseek-flash"，query 是搜索词。
func SearchWeb(ctx context.Context, apiKey, model, query string) (*WebSearchResult, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("DeepSeek API key 未配置")
	}
	if model == "" {
		model = "deepseek-flash"
	}

	reqBody := map[string]any{
		"model":      model,
		"max_tokens": webSearchMaxTokens,
		"stream":     false,
		"messages": []map[string]any{
			{"role": "user", "content": query},
		},
		"tools": []map[string]any{
			{"type": webSearchToolType, "name": webSearchToolName},
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, anthropicSearchURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", anthropicSearchVersion)

	client := &http.Client{Timeout: webSearchTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, &APIError{
			StatusCode: resp.StatusCode,
			Message:    parseAnthropicError(respBody),
			Retryable:  statusRetryable(resp.StatusCode),
		}
	}

	var result struct {
		Content []json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return extractSearchResult(result.Content)
}

// extractSearchResult 从 Anthropic content blocks 中提取搜索结果。
func extractSearchResult(blocks []json.RawMessage) (*WebSearchResult, error) {
	out := &WebSearchResult{}

	for _, raw := range blocks {
		var meta struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &meta); err != nil {
			continue
		}

		switch meta.Type {
		case "server_tool_use":
			var block struct {
				Input struct {
					Query string `json:"query"`
				} `json:"input"`
			}
			if err := json.Unmarshal(raw, &block); err == nil && block.Input.Query != "" {
				out.Queries = append(out.Queries, block.Input.Query)
			}

		case "web_search_tool_result":
			var block struct {
				Content []struct {
					Title string `json:"title"`
					URL   string `json:"url"`
				} `json:"content"`
			}
			if err := json.Unmarshal(raw, &block); err == nil {
				for _, r := range block.Content {
					out.Sources = append(out.Sources, SourceItem{
						Title: r.Title,
						URL:   r.URL,
					})
				}
			}

		case "text":
			var block struct {
				Text string `json:"text"`
			}
			if err := json.Unmarshal(raw, &block); err == nil {
				out.Summary += block.Text
			}
		}
	}

	if len(out.Sources) == 0 && out.Summary == "" {
		return nil, fmt.Errorf("搜索未返回有效结果")
	}

	return out, nil
}

// parseAnthropicError 解析 Anthropic 格式的错误响应。
func parseAnthropicError(body []byte) string {
	var resp struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &resp); err == nil && resp.Error.Message != "" {
		return resp.Error.Message
	}
	return string(body)
}
