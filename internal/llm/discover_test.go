package llm

import "testing"

// TestExtractBaseURL 验证从用户填的 chat URL 提取 base URL 的逻辑。
// extractBaseURL 用于 DiscoverModels 推导 /models 端点。
func TestExtractBaseURL(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"empty", "", ""},
		{"full endpoint", "https://x.com/v1/chat/completions", "https://x.com/v1"},
		{"base url with /v1", "https://x.com/v1", "https://x.com/v1"},
		{"bare domain with scheme", "https://x.com", "https://x.com/v1"},
		{"bare domain no scheme", "1024token.club", "https://1024token.club/v1"},
		{"trailing slash on /v1", "https://x.com/v1/", "https://x.com/v1"},
		{"trailing slash on full endpoint", "https://x.com/v1/chat/completions/", "https://x.com/v1"},
		{"custom path with /chat/completions", "https://x.com/api/openai/chat/completions", "https://x.com/api/openai"},
		{"custom path without /chat/completions", "https://x.com/api/openai", "https://x.com/api/openai"},
		{"with surrounding space", "  https://x.com/v1  ", "https://x.com/v1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractBaseURL(tt.raw); got != tt.want {
				t.Errorf("extractBaseURL(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}
