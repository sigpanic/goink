package web

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
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
	result, err := Fetch(context.Background(), "https://203.0.113.1/article")
	var businessErr *BusinessError
	if result != nil || !errors.As(err, &businessErr) || businessErr.Source != SourceLocal || businessErr.HTTPCode != http.StatusOK || businessErr.Message != "网页未返回有效正文" {
		t.Fatalf("empty body should be a local business failure: result=%+v err=%v", result, err)
	}
}

func TestLocalFetchUnavailableContent(t *testing.T) {
	for _, tc := range []struct {
		name, body, message string
		header              http.Header
	}{
		{"anti-crawl header", "", "反爬/CDN", http.Header{"Cf-Mitigated": {"challenge"}}},
		{"oversized body", strings.Repeat("a", fetchMaxBytes+1), "网页过大", nil},
		{"garbled body", "<html><body><p>" + strings.Repeat("文�", 100) + "</p></body></html>", "网页编码异常", nil},
		{"verification page", "<html><head><title>Just a moment</title></head><body><p>Please verify your browser.</p></body></html>", "反爬验证页面", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setLocalFetchTransport(t, webRoundTripFunc(func(*http.Request) (*http.Response, error) {
				resp := webJSONResponse(http.StatusOK, tc.body)
				if tc.header != nil {
					resp.Header = tc.header
				}
				return resp, nil
			}))
			result, err := Fetch(context.Background(), "https://203.0.113.1/article")
			var businessErr *BusinessError
			if result != nil || !errors.As(err, &businessErr) || businessErr.Source != SourceLocal || businessErr.HTTPCode != http.StatusOK || businessErr.APICode != "" || !strings.Contains(businessErr.Message, tc.message) {
				t.Fatalf("unavailable content lost its local business conclusion: result=%+v err=%v", result, err)
			}
		})
	}
}

func TestLocalFetchInvalidTargets(t *testing.T) {
	for _, rawURL := range []string{"https://example.com/%zz", "file:///etc/passwd", "https:///article", "https://user:pass@example.com/", "https://169.254.169.254/", "https://127.0.0.1/"} {
		result, err := Fetch(context.Background(), rawURL)
		var businessErr *BusinessError
		if result != nil || !errors.As(err, &businessErr) || businessErr.Source != SourceLocal || businessErr.HTTPCode != 0 || businessErr.APICode != "" {
			t.Fatalf("invalid target should be a local business failure: url=%q result=%+v err=%v", rawURL, result, err)
		}
	}
}

func TestLocalFetchRedirectErrorsKeepTheirChain(t *testing.T) {
	for _, tc := range []struct {
		name, target, message string
		calls                 int
	}{
		{"private target", "https://127.0.0.1/private", "禁止访问内网地址", 1},
		{"redirect limit", "https://203.0.113.1/loop", "重定向次数过多", 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			setLocalFetchTransport(t, webRoundTripFunc(func(*http.Request) (*http.Response, error) {
				calls++
				resp := webJSONResponse(http.StatusFound, "")
				resp.Header.Set("Location", tc.target)
				return resp, nil
			}))
			result, err := Fetch(context.Background(), "https://203.0.113.1/article")
			var businessErr *BusinessError
			var urlErr *url.Error
			if result != nil || calls != tc.calls || !errors.As(err, &businessErr) || !errors.As(err, &urlErr) || businessErr.Source != SourceLocal || !strings.Contains(businessErr.Message, tc.message) || !strings.Contains(err.Error(), "请求失败") {
				t.Fatalf("redirect lost its error chain or business kind: calls=%d result=%+v err=%v", calls, result, err)
			}
			if tc.name == "private target" && !strings.Contains(err.Error(), "校验重定向目标失败") {
				t.Fatalf("redirect validation context was lost: %v", err)
			}
		})
	}
}

func TestLocalFetchRedirectDNSFailure(t *testing.T) {
	previous := net.DefaultResolver
	t.Cleanup(func() { net.DefaultResolver = previous })
	net.DefaultResolver = &net.Resolver{
		PreferGo: true,
		Dial: func(context.Context, string, string) (net.Conn, error) {
			return nil, errors.New("resolver unavailable")
		},
	}
	setLocalFetchTransport(t, webRoundTripFunc(func(*http.Request) (*http.Response, error) {
		resp := webJSONResponse(http.StatusFound, "")
		resp.Header.Set("Location", "https://redirect.example.invalid/article")
		return resp, nil
	}))
	result, err := Fetch(context.Background(), "https://203.0.113.1/article")
	var businessErr *BusinessError
	var dnsErr *net.DNSError
	if result != nil || errors.As(err, &businessErr) || !errors.As(err, &dnsErr) || !strings.Contains(err.Error(), "校验重定向目标失败") || !strings.Contains(err.Error(), "resolver unavailable") {
		t.Fatalf("redirect DNS failure lost its system error chain: result=%+v err=%v", result, err)
	}
}

func TestLocalFetchDependencyErrors(t *testing.T) {
	for _, readBody := range []bool{false, true} {
		setLocalFetchTransport(t, webRoundTripFunc(func(*http.Request) (*http.Response, error) {
			if !readBody {
				return nil, io.ErrUnexpectedEOF
			}
			resp := webJSONResponse(http.StatusOK, "")
			resp.Body = io.NopCloser(brokenWebBody{})
			return resp, nil
		}))
		result, err := Fetch(context.Background(), "https://203.0.113.1/article")
		var businessErr *BusinessError
		if result != nil || errors.As(err, &businessErr) || !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("dependency failure lost its cause or system kind: readBody=%v result=%+v err=%v", readBody, result, err)
		}
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
