package web

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

type webRoundTripFunc func(*http.Request) (*http.Response, error)

func (f webRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func webJSONResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

func TestDeepSeekSearch(t *testing.T) {
	summary := strings.Repeat("网页相关摘要", 500)
	calls := 0
	client := NewDeepSeekClient("test-key", &http.Client{Transport: webRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.Method != http.MethodPost || req.URL.String() != deepSeekBaseURL+"/v1/tools/search" {
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL)
		}
		if req.Header.Get("Authorization") != "Bearer test-key" || req.Header.Get("Content-Type") != "application/json" {
			t.Fatal("missing API headers")
		}
		var payload map[string]any
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if len(payload) != 3 || payload["query"] != "如何查询资料" || payload["count"] != float64(10) || payload["need_content"] != false {
			t.Fatalf("unexpected search payload: %#v", payload)
		}
		data, _ := json.Marshal(map[string]any{"results": []map[string]any{
			{"title": "来源一", "url": "https://example.com/one", "summary": summary, "snippet": "短预览", "date": "2026-11-01T00:00:00Z", "authority": 3, "unknown": true},
			{"title": "来源二", "url": "https://example.com/two", "summary": "  ", "snippet": "回退摘要", "date": nil},
		}})
		return webJSONResponse(http.StatusOK, string(data)), nil
	})})
	result, err := client.Search(context.Background(), " 如何查询资料 ")
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || result.Query != "如何查询资料" || len(result.Results) != 2 {
		t.Fatalf("unexpected search result: calls=%d, %+v", calls, result)
	}
	if result.Results[0].Summary != summary || result.Results[0].Date != "2026-11-01T00:00:00Z" || result.Results[1].Summary != "回退摘要" || result.Results[1].Date != "" {
		t.Fatalf("search fields were not preserved: %+v", result)
	}
	encoded, _ := json.Marshal(result)
	for _, field := range []string{`"snippet"`, `"authority"`, `"unknown"`, `"content"`} {
		if strings.Contains(string(encoded), field) {
			t.Fatalf("result leaked unused field %s", field)
		}
	}
}

func TestDeepSeekSearchEmptyResults(t *testing.T) {
	client := NewDeepSeekClient("test-key", &http.Client{Transport: webRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return webJSONResponse(http.StatusOK, `{"results":[]}`), nil
	})})
	result, err := client.Search(context.Background(), "没有匹配结果的问题")
	if err != nil || result == nil || result.Results == nil || len(result.Results) != 0 {
		t.Fatalf("empty search should succeed with []: result=%+v err=%v", result, err)
	}
}

func TestDeepSeekSearchSummaryBudget(t *testing.T) {
	for _, tc := range []struct {
		name          string
		lengths       []int
		want          []int
		snippetAtLast bool
	}{
		{"under budget with long summary", []int{1000, 12000}, []int{1000, 12000}, false},
		{"exact budget", []int{7000, 8000}, []int{7000, 8000}, false},
		{"shared ceiling", []int{500, 500, 10000, 12000}, []int{500, 500, 7000, 7000}, false},
		{"short summaries retain space", []int{500, 500, 500, 500, 500, 500, 500, 500, 500, 20000}, []int{500, 500, 500, 500, 500, 500, 500, 500, 500, 10500}, false},
		{"integer remainder", []int{6000, 6000, 6000, 6000, 6000, 6000, 6000}, []int{2142, 2142, 2142, 2142, 2142, 2142, 2142}, false},
		{"snippet fallback counts toward budget", []int{14000, 2000}, []int{13000, 2000}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			items := make([]map[string]any, 0, len(tc.lengths))
			for i, length := range tc.lengths {
				item := map[string]any{
					"title": "来源" + strconv.Itoa(i), "url": "https://example.com/" + strconv.Itoa(i),
					"summary": strings.Repeat("界", length), "date": "2026-11-01T00:00:00Z",
				}
				if tc.snippetAtLast && i == len(tc.lengths)-1 {
					item["snippet"] = item["summary"]
					item["summary"] = ""
				}
				items = append(items, item)
			}
			data, err := json.Marshal(map[string]any{"results": items})
			if err != nil {
				t.Fatal(err)
			}
			client := NewDeepSeekClient("test-key", &http.Client{Transport: webRoundTripFunc(func(*http.Request) (*http.Response, error) {
				return webJSONResponse(http.StatusOK, string(data)), nil
			})})
			result, err := client.Search(context.Background(), "查询资料")
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Results) != len(tc.want) {
				t.Fatalf("result count changed: got %d want %d", len(result.Results), len(tc.want))
			}
			total := 0
			for i, item := range result.Results {
				total += utf8.RuneCountInString(item.Summary)
				if item.Summary != strings.Repeat("界", tc.want[i]) || item.Truncated != (tc.lengths[i] > tc.want[i]) || !utf8.ValidString(item.Summary) {
					t.Fatalf("result %d: chars=%d truncated=%v, want chars=%d truncated=%v", i, utf8.RuneCountInString(item.Summary), item.Truncated, tc.want[i], tc.lengths[i] > tc.want[i])
				}
				if item.Title != items[i]["title"] || item.URL != items[i]["url"] || item.Date != items[i]["date"] {
					t.Fatalf("result %d lost metadata or changed order", i)
				}
			}
			if total > 15000 {
				t.Fatalf("summary budget exceeded: %d", total)
			}
		})
	}
}

func TestDeepSeekInputValidation(t *testing.T) {
	calls := 0
	client := NewDeepSeekClient("test-key", &http.Client{Transport: webRoundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return webJSONResponse(http.StatusOK, `{"results":[]}`), nil
	})})
	for _, query := range []string{"", " \t\n", strings.Repeat("中", 129)} {
		if _, err := client.Search(context.Background(), query); err == nil {
			t.Errorf("expected invalid query: %q", query)
		}
	}
	for _, rawURL := range []string{"", "file:///etc/passwd", "https:///article", "https://user:pass@example.com/", "https://example.com/" + strings.Repeat("a", 8192)} {
		if _, err := client.Fetch(context.Background(), rawURL); err == nil {
			t.Errorf("expected invalid URL: %q", rawURL)
		}
	}
	if calls != 0 {
		t.Fatalf("invalid input reached transport %d times", calls)
	}
	if _, err := client.Search(context.Background(), strings.Repeat("中", 128)); err != nil || calls != 1 {
		t.Fatalf("128 Unicode characters should be accepted: calls=%d err=%v", calls, err)
	}
	client.apiKey = ""
	for _, call := range []func() error{
		func() error { _, err := client.Search(context.Background(), "有效问题"); return err },
		func() error { _, err := client.Fetch(context.Background(), "https://example.com/article"); return err },
	} {
		if err := call(); err == nil || !strings.Contains(err.Error(), "未配置") {
			t.Errorf("expected missing-key error, got %v", err)
		}
	}
	if calls != 1 {
		t.Fatal("missing key reached transport")
	}
}

func TestDeepSeekFetch(t *testing.T) {
	for _, size := range []int{100, fetchMaxChars, fetchMaxChars + 1} {
		t.Run(strconv.Itoa(size), func(t *testing.T) {
			content := strings.Repeat("文", size)
			client := NewDeepSeekClient("test-key", &http.Client{Transport: webRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				var payload map[string]any
				if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
					t.Fatal(err)
				}
				if req.URL.String() != deepSeekBaseURL+"/v1/tools/fetch" || len(payload) != 1 || payload["url"] != "https://example.com/article" {
					t.Fatalf("unexpected fetch request: %s %#v", req.URL, payload)
				}
				data, _ := json.Marshal(map[string]any{"title": "文章", "content": "\n" + content + "\n"})
				return webJSONResponse(http.StatusOK, string(data)), nil
			})})
			result, err := client.Fetch(context.Background(), "https://example.com/article")
			if err != nil {
				t.Fatal(err)
			}
			wantText := content
			if size > fetchMaxChars {
				wantText = strings.Repeat("文", fetchMaxChars) + "\n\n...[内容已截断]"
			}
			if result.URL != "https://example.com/article" || result.Title != "文章" || result.Mode != RemoteFetch || result.Truncated != (size > fetchMaxChars) || result.Text != wantText || !utf8.ValidString(result.Text) {
				t.Fatalf("unexpected fetch mapping: mode=%s truncated=%v chars=%d", result.Mode, result.Truncated, utf8.RuneCountInString(result.Text))
			}
		})
	}
}

func TestDeepSeekAPIErrorsDoNotRetry(t *testing.T) {
	for _, status := range []int{401, 402, 403, 404, 408, 429, 500, 504} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			for _, fetch := range []bool{false, true} {
				calls := 0
				client := NewDeepSeekClient("test-key", &http.Client{Transport: webRoundTripFunc(func(*http.Request) (*http.Response, error) {
					calls++
					return webJSONResponse(status, `{"error":{"message":"已知错误原因","code":"api_error","param":null}}`), nil
				})})
				var err error
				if fetch {
					_, err = client.Fetch(context.Background(), "https://example.com/article")
				} else {
					_, err = client.Search(context.Background(), "查询")
				}
				var apiErr *APIError
				if !errors.As(err, &apiErr) || apiErr.StatusCode != status || apiErr.Code != "api_error" || apiErr.Message != "已知错误原因" || calls != 1 {
					t.Fatalf("error mapping or retry mismatch: calls=%d err=%v", calls, err)
				}
			}
		})
	}
}

func TestDeepSeekInvalidResponses(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		fetch      bool
	}{
		{"invalid JSON", `{`, false},
		{"missing results", `{}`, false},
		{"invalid results", `{"results":"broken"}`, false},
		{"missing content", `{"title":"没有正文"}`, true},
		{"blank content", `{"content":" \n\t"}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := NewDeepSeekClient("test-key", &http.Client{Transport: webRoundTripFunc(func(*http.Request) (*http.Response, error) {
				return webJSONResponse(http.StatusOK, tc.body), nil
			})})
			var err error
			if tc.fetch {
				_, err = client.Fetch(context.Background(), "https://example.com/article")
			} else {
				_, err = client.Search(context.Background(), "查询")
			}
			if err == nil {
				t.Fatal("invalid response was accepted as complete success")
			}
		})
	}
	client := NewDeepSeekClient("test-key", &http.Client{Transport: webRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return webJSONResponse(http.StatusBadGateway, "<html>Bad gateway</html>"), nil
	})})
	_, err := client.Search(context.Background(), "查询")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Message != "Bad Gateway" {
		t.Fatalf("non-JSON errors lost HTTP status: %v", err)
	}
}

type brokenWebBody struct{}

func (brokenWebBody) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestDeepSeekReadAndContextErrors(t *testing.T) {
	client := NewDeepSeekClient("test-key", &http.Client{Transport: webRoundTripFunc(func(*http.Request) (*http.Response, error) {
		resp := webJSONResponse(http.StatusOK, "")
		resp.Body = io.NopCloser(io.MultiReader(strings.NewReader(`{"results":[]}`), brokenWebBody{}))
		return resp, nil
	})})
	if result, err := client.Search(context.Background(), "查询"); result != nil || !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("truncated transport response was accepted: result=%+v err=%v", result, err)
	}
	calls := 0
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client.http = &http.Client{Transport: webRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		<-req.Context().Done()
		return nil, req.Context().Err()
	})}
	if _, err := client.Search(ctx, "查询"); !errors.Is(err, context.Canceled) || calls != 0 {
		t.Fatalf("pre-cancelled request reached transport: calls=%d err=%v", calls, err)
	}
	ctx, cancel = context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := client.Fetch(ctx, "https://example.com/article"); !errors.Is(err, context.DeadlineExceeded) || calls != 1 {
		t.Fatalf("request did not honor deadline: calls=%d err=%v", calls, err)
	}
}
