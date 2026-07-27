package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// TestConnectionInput 是连通性测试的参数。
type TestConnectionInput struct {
	ProviderName string `json:"provider_name"`
	ChatURL      string `json:"chat_url"`
	APIKey       string `json:"api_key"`
	ModelID      string `json:"model_id"`
}

// TestConnection 发送最小化请求验证 provider 连通性。返回 error 表示失败。
func TestConnection(ctx context.Context, builtin map[string]Provider, input TestConnectionInput) error {
	chatURL := input.ChatURL
	buildHeaders := func(base map[string]string) map[string]string { return base }
	var buildRequest func(map[string]any) map[string]any

	if bp, ok := builtin[input.ProviderName]; ok {
		if chatURL == "" {
			chatURL = bp.ChatURL
		}
		if bp.BuildHeaders != nil {
			buildHeaders = bp.BuildHeaders
		}
		if bp.BuildRequest != nil {
			buildRequest = bp.BuildRequest
		}
	}

	chatURL = normalizeURL(chatURL)

	payload := map[string]any{
		"model": input.ModelID,
		"messages": []map[string]any{
			{"role": "user", "content": "hi"},
		},
		"max_tokens": 1,
		"stream":     true,
	}
	if buildRequest != nil {
		payload = buildRequest(payload)
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("序列化请求失败: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, chatURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("创建 HTTP 请求失败: %w", err)
	}

	headers := buildHeaders(map[string]string{
		"Content-Type":  "application/json",
		"Accept":        "text/event-stream",
		"Authorization": "Bearer " + input.APIKey,
	})
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		errBody := make([]byte, 1024)
		n, _ := resp.Body.Read(errBody)
		return fmt.Errorf("[%d] %s", resp.StatusCode, string(errBody[:n]))
	}

	// 检查 Content-Type 是 SSE 流。中转站 SPA 对不存在的路径可能返回 200 + HTML 首页，
	// 仅靠状态码会误判通过；这里强制要求 text/event-stream，HTML 错误页会被拦下。
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "text/event-stream") {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("响应不是 SSE 流 (Content-Type: %s), body: %s", ct, string(errBody))
	}

	// 扫描 SSE 流，验证至少有一个 data: 行且 JSON 含 choices 字段。
	// 防止中转站返回空 SSE 流或非标格式（如 GPT-5 Responses API 的 event: 类型）。
	// 读到第一个有效 chunk 就 break，不等流结束，负担小。
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	foundValid := false
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := line[len("data: "):]
		if data == "[DONE]" {
			break
		}
		var chunk map[string]any
		if err := json.Unmarshal([]byte(data), &chunk); err == nil {
			if _, ok := chunk["choices"]; ok {
				foundValid = true
				break
			}
		}
	}
	if !foundValid {
		return fmt.Errorf("SSE 流中未找到有效 chunk（可能模型不可用或返回非标准格式）")
	}

	return nil
}
