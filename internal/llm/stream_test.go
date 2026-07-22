package llm

import (
	"net/http"
	"testing"
	"time"
)

// --- parseRetryAfter ---

func TestParseRetryAfter_Empty(t *testing.T) {
	if got := parseRetryAfter(""); got != 0 {
		t.Errorf("empty string: got %v, want 0", got)
	}
}

func TestParseRetryAfter_WhitespaceOnly(t *testing.T) {
	if got := parseRetryAfter("   "); got != 0 {
		t.Errorf("whitespace only: got %v, want 0", got)
	}
}

func TestParseRetryAfter_Seconds(t *testing.T) {
	cases := []struct {
		input string
		want  time.Duration
	}{
		{"0", 0},
		{"1", time.Second},
		{"120", 120 * time.Second},
	}
	for _, c := range cases {
		if got := parseRetryAfter(c.input); got != c.want {
			t.Errorf("parseRetryAfter(%q): got %v, want %v", c.input, got, c.want)
		}
	}
}

func TestParseRetryAfter_SecondsWithSurroundingSpaces(t *testing.T) {
	// TrimSpace 后等价于 "120"
	if got := parseRetryAfter("  120  "); got != 120*time.Second {
		t.Errorf("got %v, want 120s", got)
	}
}

func TestParseRetryAfter_NegativeSeconds(t *testing.T) {
	// secs >= 0 检查拒绝负数；HTTP-date 解析也失败 → 0
	if got := parseRetryAfter("-5"); got != 0 {
		t.Errorf("negative seconds: got %v, want 0", got)
	}
}

func TestParseRetryAfter_InvalidString(t *testing.T) {
	if got := parseRetryAfter("abc"); got != 0 {
		t.Errorf("invalid string: got %v, want 0", got)
	}
}

func TestParseRetryAfter_FutureHTTPDate(t *testing.T) {
	// 未来日期 → 正 duration
	future := time.Now().Add(2 * time.Hour).UTC().Format(http.TimeFormat)
	got := parseRetryAfter(future)
	if got <= 0 {
		t.Errorf("future date %q: got %v, want > 0", future, got)
	}
	// 应接近 2 小时（容忍漂移）
	if got > 2*time.Hour+10*time.Second {
		t.Errorf("future date %q: got %v, want <= ~2h", future, got)
	}
}

func TestParseRetryAfter_PastHTTPDate(t *testing.T) {
	// 过去日期 → time.Until 负数 → 不返回 → 0
	past := time.Now().Add(-2 * time.Hour).UTC().Format(http.TimeFormat)
	if got := parseRetryAfter(past); got != 0 {
		t.Errorf("past date %q: got %v, want 0", past, got)
	}
}

// --- statusRetryable ---

func TestStatusRetryable(t *testing.T) {
	cases := []struct {
		code int
		want bool
	}{
		{429, true},  // 限流
		{408, true},  // 超时
		{500, true},  // 服务端错误
		{502, true},
		{503, true},
		{599, true},  // 5xx 上界
		{400, false}, // 客户端错误
		{401, false},
		{404, false},
		{422, false},
		{600, false}, // 超出范围
		{200, false}, // 成功
		{0, false},   // 网络错误（无状态码）
	}
	for _, c := range cases {
		if got := statusRetryable(c.code); got != c.want {
			t.Errorf("statusRetryable(%d): got %v, want %v", c.code, got, c.want)
		}
	}
}

// --- parseDefaultError ---

func TestParseDefaultError_StandardFormat(t *testing.T) {
	body := []byte(`{"error":{"message":"rate limit exceeded"}}`)
	err := parseDefaultError(body)
	if err.Error() != "rate limit exceeded" {
		t.Errorf("got %q, want %q", err.Error(), "rate limit exceeded")
	}
}

func TestParseDefaultError_EmptyMessage(t *testing.T) {
	// JSON 合法但 message 为空 → 回退到 raw body
	body := []byte(`{"error":{"message":""}}`)
	err := parseDefaultError(body)
	want := `request failed: {"error":{"message":""}}`
	if err.Error() != want {
		t.Errorf("got %q, want %q", err.Error(), want)
	}
}

func TestParseDefaultError_NoMessageField(t *testing.T) {
	body := []byte(`{"foo":"bar"}`)
	err := parseDefaultError(body)
	want := `request failed: {"foo":"bar"}`
	if err.Error() != want {
		t.Errorf("got %q, want %q", err.Error(), want)
	}
}

func TestParseDefaultError_InvalidJSON(t *testing.T) {
	body := []byte("not json at all")
	err := parseDefaultError(body)
	want := "request failed: not json at all"
	if err.Error() != want {
		t.Errorf("got %q, want %q", err.Error(), want)
	}
}
