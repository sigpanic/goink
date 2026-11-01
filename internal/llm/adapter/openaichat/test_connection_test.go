package openaichat

import (
	"strings"
	"testing"
)

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
