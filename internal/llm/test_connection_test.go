package llm

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestTestConnection_SSEValid 验证标准 OpenAI SSE 流能通过连通性测试。
func TestTestConnection_SSEValid(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "data: {\"id\":\"x\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\"}}]}\n\n")
		fmt.Fprint(w, "data: {\"id\":\"x\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hi\"}}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	input := TestConnectionInput{
		ProviderName: "custom",
		ChatURL:      server.URL + "/v1/chat/completions",
		APIKey:       "sk-test",
		ModelID:      "test-model",
	}

	result, err := TestConnection(context.Background(), Builtin, input)
	if err != nil {
		t.Fatalf("expected pass, got error: %v", err)
	}
	// 第一个候选是原样（完整端点），应原样返回
	if result.URL != input.ChatURL {
		t.Errorf("expected url %s, got %s", input.ChatURL, result.URL)
	}
}

// TestTestConnection_HTMLPage 验证中转站 SPA fallback 返回 200 + HTML 时被拦下。
// 这是 zhangjianqin case 的根因：URL 拼错导致中转站返回 HTML 首页，旧版只检查状态码会误判通过。
func TestTestConnection_HTMLPage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "<!doctype html><html><body>SPA fallback</body></html>")
	}))
	defer server.Close()

	input := TestConnectionInput{
		ProviderName: "custom",
		ChatURL:      server.URL + "/v1/chat/completions",
		APIKey:       "sk-test",
		ModelID:      "test-model",
	}

	_, err := TestConnection(context.Background(), Builtin, input)
	if err == nil {
		t.Fatal("expected error for HTML response, got nil")
	}
	if err.Error() == "" {
		t.Error("error message should not be empty")
	}
}

// TestTestConnection_EmptySSE 验证空 SSE 流（无有效 chunk）被拦下。
// 模拟中转站返回空流或非标格式导致 parseSSE hasContent=false 的场景。
func TestTestConnection_EmptySSE(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		// 只有 [DONE]，无有效 chunk
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	input := TestConnectionInput{
		ProviderName: "custom",
		ChatURL:      server.URL + "/v1/chat/completions",
		APIKey:       "sk-test",
		ModelID:      "test-model",
	}

	_, err := TestConnection(context.Background(), Builtin, input)
	if err == nil {
		t.Fatal("expected error for empty SSE stream, got nil")
	}
}

// TestTestConnection_NonChoicesChunk 验证 SSE 流里 JSON 不含 choices 字段时被拦下。
// 模拟 GPT-5 Responses API 的 event: 格式（无 choices 字段，Goink parseSSE 会判空）。
func TestTestConnection_NonChoicesChunk(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		// GPT-5 Responses API 格式：type 字段而非 choices
		fmt.Fprint(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"hi\"}\n\n")
	}))
	defer server.Close()

	input := TestConnectionInput{
		ProviderName: "custom",
		ChatURL:      server.URL + "/v1/chat/completions",
		APIKey:       "sk-test",
		ModelID:      "test-model",
	}

	_, err := TestConnection(context.Background(), Builtin, input)
	if err == nil {
		t.Fatal("expected error for non-choices SSE chunk, got nil")
	}
}

// TestTestConnection_HTTPError 验证 4xx/5xx 状态码被拦下（保留旧逻辑）。
func TestTestConnection_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":{"message":"API_KEY_REQUIRED","type":"one_api_error"}}`)
	}))
	defer server.Close()

	input := TestConnectionInput{
		ProviderName: "custom",
		ChatURL:      server.URL + "/v1/chat/completions",
		APIKey:       "sk-test",
		ModelID:      "test-model",
	}

	_, err := TestConnection(context.Background(), Builtin, input)
	if err == nil {
		t.Fatal("expected error for 401, got nil")
	}
}

// TestTestConnection_RateLimited429 验证 429 限流视为通过（配置有效，只是被限速）。
// 429 不应该阻止保存：URL/Key/Model 都对，只是当前请求被限流，重试就行。
// 返回 (result, nil)，result.URL 是端点，result.Warning 非空提示限流。
func TestTestConnection_RateLimited429(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `{"error":{"message":"Rate limit exceeded","type":"rate_limit_exceeded"}}`)
	}))
	defer server.Close()

	input := TestConnectionInput{
		ProviderName: "custom",
		ChatURL:      server.URL + "/v1/chat/completions",
		APIKey:       "sk-test",
		ModelID:      "test-model",
	}

	result, err := TestConnection(context.Background(), Builtin, input)
	if err != nil {
		t.Fatalf("expected pass for 429 (rate limited but valid), got error: %v", err)
	}
	if result.URL != input.ChatURL {
		t.Errorf("expected url %s, got %s", input.ChatURL, result.URL)
	}
	if result.Warning == "" {
		t.Error("expected non-empty warning for 429, got empty")
	}
	if !strings.Contains(result.Warning, "429") {
		t.Errorf("warning should mention 429, got: %s", result.Warning)
	}
}

// TestTestConnection_FallbackBareDomain 验证裸域名多层 fallback 到 /v1/chat/completions。
// mock server 只对 /v1/chat/completions 路径返回有效 SSE，其他路径 404。
// 用户填裸域名（无路径），应 fallback 探测到 /v1/chat/completions。
func TestTestConnection_FallbackBareDomain(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"error":"not found"}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "data: {\"id\":\"x\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hi\"}}]}\n\n")
	}))
	defer server.Close()

	input := TestConnectionInput{
		ProviderName: "custom",
		ChatURL:      server.URL, // 裸域名，无路径
		APIKey:       "sk-test",
		ModelID:      "test-model",
	}

	result, err := TestConnection(context.Background(), Builtin, input)
	if err != nil {
		t.Fatalf("expected pass via fallback, got error: %v", err)
	}
	expected := server.URL + "/v1/chat/completions"
	if result.URL != expected {
		t.Errorf("expected fallback url %s, got %s", expected, result.URL)
	}
}

// TestTestConnection_FallbackBaseURL 验证 base URL（/v1）fallback 到 /v1/chat/completions。
// 用户填 https://x.com/v1，应探测到 /v1/chat/completions。
func TestTestConnection_FallbackBaseURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "data: {\"id\":\"x\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hi\"}}]}\n\n")
	}))
	defer server.Close()

	input := TestConnectionInput{
		ProviderName: "custom",
		ChatURL:      server.URL + "/v1", // base URL
		APIKey:       "sk-test",
		ModelID:      "test-model",
	}

	result, err := TestConnection(context.Background(), Builtin, input)
	if err != nil {
		t.Fatalf("expected pass via fallback, got error: %v", err)
	}
	expected := server.URL + "/v1/chat/completions"
	if result.URL != expected {
		t.Errorf("expected fallback url %s, got %s", expected, result.URL)
	}
}

// TestTestConnection_AllCandidatesFail 验证所有候选 URL 均失败时返回 error。
func TestTestConnection_AllCandidatesFail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"error":"not found"}`)
	}))
	defer server.Close()

	input := TestConnectionInput{
		ProviderName: "custom",
		ChatURL:      server.URL,
		APIKey:       "sk-test",
		ModelID:      "test-model",
	}

	_, err := TestConnection(context.Background(), Builtin, input)
	if err == nil {
		t.Fatal("expected error when all candidates fail, got nil")
	}
	if !strings.Contains(err.Error(), "均验证失败") {
		t.Errorf("error should mention all candidates failed, got: %v", err)
	}
}
