package llm

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
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

	if err := TestConnection(context.Background(), Builtin, input); err != nil {
		t.Errorf("expected pass, got error: %v", err)
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

	err := TestConnection(context.Background(), Builtin, input)
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

	err := TestConnection(context.Background(), Builtin, input)
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

	err := TestConnection(context.Background(), Builtin, input)
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

	err := TestConnection(context.Background(), Builtin, input)
	if err == nil {
		t.Fatal("expected error for 401, got nil")
	}
}
