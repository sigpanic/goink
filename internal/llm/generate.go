package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// GenerateText 发起非流式对话，返回模型生成的纯文本。
// messages 由调用方自行组装，格式与 ChatStream 一致。
func (c *Client) GenerateText(
	ctx context.Context,
	providerName string,
	messages []map[string]any,
	model string,
) (string, error) {
	c.mu.RLock()
	p, ok := c.providers[providerName]
	c.mu.RUnlock()
	if !ok {
		return "", fmt.Errorf("unknown provider: %s", providerName)
	}

	payload := map[string]any{
		"model":    model,
		"messages": messages,
		"stream":   false,
	}

	if p.BuildRequest != nil {
		payload = p.BuildRequest(payload)
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.ChatURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create HTTP request: %w", err)
	}

	headers := map[string]string{
		"Content-Type":  "application/json",
		"Authorization": "Bearer " + p.APIKey,
	}
	if p.BuildHeaders != nil {
		headers = p.BuildHeaders(nil, headers)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		msg := parseDefaultError(respBody).Error()
		if p.ParseError != nil {
			msg = p.ParseError(respBody).Error()
		}
		return "", &APIError{StatusCode: resp.StatusCode, Message: msg, Retryable: statusRetryable(resp.StatusCode)}
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("empty response from LLM")
	}
	return result.Choices[0].Message.Content, nil
}
