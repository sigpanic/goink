package web

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func setLocalFetchTransport(t *testing.T, transport http.RoundTripper) {
	t.Helper()
	previous := http.DefaultTransport
	http.DefaultTransport = transport
	t.Cleanup(func() { http.DefaultTransport = previous })
}

func TestLocalFetchResult(t *testing.T) {
	for _, tc := range []struct {
		name      string
		content   string
		truncated bool
	}{
		{"article", strings.Repeat("这是一段正常的文章正文，用于阅读和提取。", 50), false},
		{"long article", strings.Repeat("这是一段正常的文章正文，用于阅读和提取。", 1000), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setLocalFetchTransport(t, webRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.Method != http.MethodGet {
					t.Fatalf("unexpected method: %s", req.Method)
				}
				return webJSONResponse(http.StatusOK, "<html><head><title>测试文章</title></head><body><article><p>"+tc.content+"</p></article></body></html>"), nil
			}))
			result, err := Fetch(context.Background(), "https://203.0.113.1/article")
			if err != nil {
				t.Fatal(err)
			}
			if result.Mode != LocalFetch || result.Truncated != tc.truncated || result.URL != "https://203.0.113.1/article" || result.Title != "测试文章" || !utf8.ValidString(result.Text) {
				t.Fatalf("unexpected local result: %+v", result)
			}
			if tc.truncated {
				if result.Text != string([]rune(tc.content)[:fetchMaxChars])+"\n\n...[内容已截断]" {
					t.Fatal("local fetch did not retain the expected Unicode prefix")
				}
			} else if result.Text != tc.content {
				t.Fatal("local fetch altered the article body")
			}
		})
	}
}

func TestLocalFetchEmptyBody(t *testing.T) {
	setLocalFetchTransport(t, webRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return webJSONResponse(http.StatusOK, "<html><head><title>空页面</title></head><body></body></html>"), nil
	}))
	if result, err := Fetch(context.Background(), "https://203.0.113.1/article"); err == nil || result != nil {
		t.Fatalf("empty body was accepted: result=%+v err=%v", result, err)
	}
}

func TestLocalFetchCancellation(t *testing.T) {
	calls := 0
	setLocalFetchTransport(t, webRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		<-req.Context().Done()
		return nil, req.Context().Err()
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Fetch(ctx, "https://203.0.113.1/article"); !errors.Is(err, context.Canceled) || calls != 0 {
		t.Fatalf("pre-cancelled fetch reached transport: calls=%d err=%v", calls, err)
	}
	ctx, cancel = context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := Fetch(ctx, "https://203.0.113.1/article"); !errors.Is(err, context.DeadlineExceeded) || calls != 0 {
		t.Fatalf("fetch delay ignored cancellation: calls=%d err=%v", calls, err)
	}
}

func TestLocalFetchCancelsHTTPRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	setLocalFetchTransport(t, webRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		cancel()
		<-req.Context().Done()
		return nil, req.Context().Err()
	}))
	if result, err := Fetch(ctx, "https://203.0.113.1/article"); !errors.Is(err, context.Canceled) || result != nil {
		t.Fatalf("HTTP request ignored cancellation: result=%+v err=%v", result, err)
	}
}
