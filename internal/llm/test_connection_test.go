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

	url, err := TestConnection(context.Background(), Builtin, input)
	if err != nil {
		t.Fatalf("expected pass, got error: %v", err)
	}
	// 第一个候选是原样（完整端点），应原样返回
	if url != input.ChatURL {
		t.Errorf("expected url %s, got %s", input.ChatURL, url)
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

	url, err := TestConnection(context.Background(), Builtin, input)
	if err != nil {
		t.Fatalf("expected pass via fallback, got error: %v", err)
	}
	expected := server.URL + "/v1/chat/completions"
	if url != expected {
		t.Errorf("expected fallback url %s, got %s", expected, url)
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

	url, err := TestConnection(context.Background(), Builtin, input)
	if err != nil {
		t.Fatalf("expected pass via fallback, got error: %v", err)
	}
	expected := server.URL + "/v1/chat/completions"
	if url != expected {
		t.Errorf("expected fallback url %s, got %s", expected, url)
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

// TestExpandChatURLCandidates 验证候选 URL 生成逻辑。
func TestExpandChatURLCandidates(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{
			name: "bare domain",
			raw:  "https://1024token.club",
			want: []string{
				"https://1024token.club",
				"https://1024token.club/chat/completions",
				"https://1024token.club/v1/chat/completions",
			},
		},
		{
			name: "base url with /v1",
			raw:  "https://api.deepseek.com/v1",
			want: []string{
				"https://api.deepseek.com/v1",
				"https://api.deepseek.com/v1/chat/completions",
			},
		},
		{
			name: "full endpoint",
			raw:  "https://x.com/v1/chat/completions",
			want: []string{
				"https://x.com/v1/chat/completions",
				"https://x.com/chat/completions",
			},
		},
		{
			name: "trailing slash trimmed",
			raw:  "https://x.com/v1/",
			want: []string{
				"https://x.com/v1",
				"https://x.com/v1/chat/completions",
			},
		},
		{
			name: "no scheme gets https",
			raw:  "1024token.club",
			want: []string{
				"https://1024token.club",
				"https://1024token.club/chat/completions",
				"https://1024token.club/v1/chat/completions",
			},
		},
		{
			// 用户填错路径（少 s）→ 第 6 档兜底生成 host + /v1/chat/completions
			name: "wrong path typo fallback to host+/v1",
			raw:  "https://x.com/v4/chat/completion",
			want: []string{
				"https://x.com/v4/chat/completion",
				"https://x.com/v4/chat/completion/chat/completions",
				"https://x.com/v1/chat/completions",
			},
		},
		{
			// 带端口：第 6 档用 net/url 提取 host，端口正确保留
			name: "port preserved in fallback",
			raw:  "https://x.com:8080/v4/chat/completion",
			want: []string{
				"https://x.com:8080/v4/chat/completion",
				"https://x.com:8080/v4/chat/completion/chat/completions",
				"https://x.com:8080/v1/chat/completions",
			},
		},
		{
			name: "empty",
			raw:  "",
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := expandChatURLCandidates(tt.raw)
			if len(got) != len(tt.want) {
				t.Errorf("expandChatURLCandidates(%q) = %v, want %v", tt.raw, got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("expandChatURLCandidates(%q)[%d] = %q, want %q", tt.raw, i, got[i], tt.want[i])
				}
			}
		})
	}
}

// TestSummarizeErrorBody 验证 HTTP 错误响应体的格式化逻辑。
// 覆盖三种路径：OpenAI 兼容 JSON、HTML 错误页、其他格式。
func TestSummarizeErrorBody(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		want       string
	}{
		{
			name:       "openai compatible error with type",
			statusCode: 401,
			body:       `{"error":{"message":"Invalid API Key","type":"invalid_key"}}`,
			want:       "[401] Invalid API Key (invalid_key)",
		},
		{
			name:       "openai compatible error without type",
			statusCode: 402,
			body:       `{"error":{"message":"余额不足"}}`,
			want:       "[402] 余额不足",
		},
		{
			// 扁平格式：message 在顶层，error 是字符串而非对象（非标准中转站通用格式）
			name:       "flat error format with message and error string",
			statusCode: 404,
			body:       `{"code":5,"error":"url.not_found","message":"没找到对象","method":"GET","scode":"0x5","status":false,"ua":"Go-http-client/2.0","url":"/v1/chat/completi/models"}`,
			want:       "[404] 没找到对象 (url.not_found)",
		},
		{
			// 扁平格式：只有 message，无 error 字符串
			name:       "flat error format with message only",
			statusCode: 400,
			body:       `{"message":"参数错误"}`,
			want:       "[400] 参数错误",
		},
		{
			// Spring Boot 默认错误格式：error 是 HTTP 描述字符串 + path，无 message
			// path 让用户看出 URL 拼接错误（/v4/chat/completion ← chat URL 填错）
			name:       "spring boot error format with path",
			statusCode: 404,
			body:       `{"timestamp":"2026-07-29T01:47:18.779+00:00","status":404,"error":"Not Found","path":"/v4/chat/completion/models"}`,
			want:       "[404] Not Found (path: /v4/chat/completion/models)",
		},
		{
			// 扁平 error 字符串无 path（最简 Spring Boot 格式缺 path）
			name:       "flat error string without path",
			statusCode: 500,
			body:       `{"error":"Internal Server Error"}`,
			want:       "[500] Internal Server Error",
		},
		{
			name:       "html error page",
			statusCode: 404,
			body:       `<html><head><title>404 Not Found</title></head><body>openresty</body></html>`,
			want:       "[404] [HTML 错误页，可能 URL 错误或被防火墙拦]",
		},
		{
			name:       "plain text error",
			statusCode: 500,
			body:       `Internal Server Error`,
			want:       "[500] Internal Server Error",
		},
		{
			name:       "long plain text truncated",
			statusCode: 500,
			body:       strings.Repeat("a", 300),
			want:       "[500] " + strings.Repeat("a", 200) + "...",
		},
		{
			name:       "json without error message falls through",
			statusCode: 400,
			body:       `{"foo":"bar"}`,
			want:       `[400] {"foo":"bar"}`,
		},
		{
			name:       "empty body",
			statusCode: 404,
			body:       ``,
			want:       "[404] [空响应，可能 URL 错误或端点不存在]",
		},
		{
			name:       "whitespace only body",
			statusCode: 500,
			body:       `   `,
			want:       "[500] [空响应，可能 URL 错误或端点不存在]",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := summarizeErrorBody(tt.statusCode, []byte(tt.body))
			if got != tt.want {
				t.Errorf("summarizeErrorBody(%d, %q) = %q, want %q", tt.statusCode, tt.body, got, tt.want)
			}
		})
	}
}
