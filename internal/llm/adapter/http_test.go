package adapter

import (
	"net/http"
	"testing"
	"time"
)

// --- parseRetryAfter ---

func TestParseRetryAfter_Empty(t *testing.T) {
	if got := ParseRetryAfter(""); got != 0 {
		t.Errorf("empty string: got %v, want 0", got)
	}
}

func TestParseRetryAfter_WhitespaceOnly(t *testing.T) {
	if got := ParseRetryAfter("   "); got != 0 {
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
		{"12", 12 * time.Second},
		{"120", 120 * time.Second},
		{"9223372036", 9223372036 * time.Second},
		{"9223372037", 0},
		{"18446744074", 0},
		{"9223372036854775807", 0},
		{"9223372036854775808", 0},
	}
	for _, c := range cases {
		if got := ParseRetryAfter(c.input); got != c.want {
			t.Errorf("ParseRetryAfter(%q): got %v, want %v", c.input, got, c.want)
		}
	}
}

func TestParseRetryAfter_SecondsWithSurroundingSpaces(t *testing.T) {
	// TrimSpace 后等价于 "120"
	if got := ParseRetryAfter("  120  "); got != 120*time.Second {
		t.Errorf("got %v, want 120s", got)
	}
}

func TestParseRetryAfter_NegativeSeconds(t *testing.T) {
	// secs >= 0 检查拒绝负数；HTTP-date 解析也失败 → 0
	if got := ParseRetryAfter("-5"); got != 0 {
		t.Errorf("negative seconds: got %v, want 0", got)
	}
}

func TestParseRetryAfter_InvalidString(t *testing.T) {
	for _, value := range []string{"abc", "garbage", "invalid", "not a date", "1.5"} {
		if got := ParseRetryAfter(value); got != 0 {
			t.Errorf("invalid string %q: got %v, want 0", value, got)
		}
	}
}

func TestParseRetryAfter_FutureHTTPDate(t *testing.T) {
	// 未来日期 → 正 duration
	future := time.Now().Add(2 * time.Hour).UTC().Format(http.TimeFormat)
	got := ParseRetryAfter(future)
	if got <= 0 {
		t.Errorf("future date %q: got %v, want > 0", future, got)
	}
	// 应接近 2 小时（容忍漂移）
	if got < 2*time.Hour-2*time.Second || got > 2*time.Hour {
		t.Errorf("future date %q: got %v, want ~2h", future, got)
	}
}

func TestParseRetryAfter_PastHTTPDate(t *testing.T) {
	// 过去日期 → time.Until 负数 → 不返回 → 0
	past := time.Now().Add(-2 * time.Hour).UTC().Format(http.TimeFormat)
	if got := ParseRetryAfter(past); got != 0 {
		t.Errorf("past date %q: got %v, want 0", past, got)
	}
}
