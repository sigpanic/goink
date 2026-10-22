package anthropic

import (
	"net/http"
	"testing"
	"time"
)

func TestRetryAfter(t *testing.T) {
	for _, tt := range []struct {
		raw  string
		want time.Duration
	}{
		{"", 0}, {"garbage", 0}, {"-1", 0}, {"9223372036854775807", 0},
		{"0", 0}, {" 12 ", 12 * time.Second},
		{time.Now().Add(-time.Hour).UTC().Format(http.TimeFormat), 0},
	} {
		if got := parseRetryAfter(tt.raw); got != tt.want {
			t.Fatalf("parseRetryAfter(%q) = %v, want %v", tt.raw, got, tt.want)
		}
	}
	got := parseRetryAfter(time.Now().Add(time.Minute).UTC().Format(http.TimeFormat))
	if got < 58*time.Second || got > time.Minute {
		t.Fatalf("future HTTP-date parsed as %v", got)
	}
}
