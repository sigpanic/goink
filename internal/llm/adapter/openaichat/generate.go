package openaichat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

// Generate 将非流式响应转换为完整的 Goink 结果，不执行响应中的工具。
func (c *Adapter) Generate(ctx context.Context, input adapter.Request) (adapter.Response, error) {
	p, err := c.provider(input.ProviderName)
	if err != nil {
		return adapter.Response{}, err
	}
	messages, err := c.buildMessages(input.Messages)
	if err != nil {
		return adapter.Response{}, err
	}
	tools, err := buildTools(input.Tools)
	if err != nil {
		return adapter.Response{}, err
	}
	payload := c.buildPayload(&p, messages, tools, input.Model, input.Options, false)
	body, err := json.Marshal(payload)
	if err != nil {
		return adapter.Response{}, fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.ChatURL, bytes.NewReader(body))
	if err != nil {
		return adapter.Response{}, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	headers := map[string]string{
		"Content-Type":  "application/json",
		"Authorization": "Bearer " + p.APIKey,
	}
	if p.BuildHeaders != nil {
		headers = p.BuildHeaders(input.Options, headers)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return adapter.Response{}, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return adapter.Response{}, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		msg := parseDefaultError(respBody).Error()
		if p.ParseError != nil {
			msg = p.ParseError(respBody).Error()
		}
		return adapter.Response{}, &APIError{StatusCode: resp.StatusCode, Message: msg, Retryable: adapter.StatusRetryable(resp.StatusCode), RetryAfter: adapter.ParseRetryAfter(resp.Header.Get("Retry-After"))}
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content         string `json:"content"`
				ThinkingContent string `json:"reasoning_content"`
				ToolCalls       []struct {
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage map[string]any `json:"usage"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return adapter.Response{}, fmt.Errorf("failed to parse response: %w", err)
	}
	if len(result.Choices) == 0 {
		return adapter.Response{}, fmt.Errorf("empty response from LLM")
	}
	choice := result.Choices[0]
	response := adapter.Response{
		Content:         choice.Message.Content,
		ThinkingContent: choice.Message.ThinkingContent,
		Usage:           result.Usage,
		StopReason:      choice.FinishReason,
	}
	seen := make(map[string]bool)
	for _, call := range choice.Message.ToolCalls {
		if call.ID != "" && seen[call.ID] {
			continue
		}
		seen[call.ID] = true
		if call.Function.Name == "" || !json.Valid([]byte(call.Function.Arguments)) {
			return adapter.Response{}, &APIError{
				Message:   "tool arguments 解析失败：" + call.Function.Name,
				Kind:      "tool_args_invalid",
				Retryable: true,
			}
		}
		response.ToolCalls = append(response.ToolCalls, ToolCallDelta{
			ToolID:        call.ID,
			ToolName:      call.Function.Name,
			ArgumentsText: call.Function.Arguments,
			ArgumentsJSON: json.RawMessage(call.Function.Arguments),
		})
	}
	return response, nil
}
