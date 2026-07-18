package githubapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetRawContent_OK 覆盖 200 成功路径，验证返回内容 + rate limit 解析。
func TestGetRawContent_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 验证请求 header
		assert.Equal(t, "application/vnd.github.raw+json", r.Header.Get("Accept"))
		assert.Equal(t, "Goink", r.Header.Get("User-Agent"))
		// 验证 ref 查询参数
		assert.Equal(t, "main", r.URL.Query().Get("ref"))

		w.Header().Set("X-RateLimit-Remaining", "59")
		w.Header().Set("X-RateLimit-Reset", "1700000000")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("# raw skill content"))
	}))
	defer srv.Close()

	c := NewClient()
	c.baseURL = srv.URL

	body, rl, err := c.GetRawContent(context.Background(), "sigpanic", "goink-skills", "main", "index.json")
	require.NoError(t, err)
	assert.Equal(t, "# raw skill content", string(body))
	require.NotNil(t, rl)
	assert.Equal(t, 59, rl.Remaining)
	assert.Equal(t, time.Unix(1700000000, 0).UTC(), rl.ResetAt)
}

// TestGetRawContent_NotFound 覆盖 404 → KindNotFound。
func TestGetRawContent_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "58")
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := NewClient()
	c.baseURL = srv.URL

	body, rl, err := c.GetRawContent(context.Background(), "sigpanic", "goink-skills", "main", "skills/missing.md")
	require.Error(t, err)
	assert.Nil(t, body)
	var apiErr *Error
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, KindNotFound, apiErr.Kind)
	assert.Equal(t, http.StatusNotFound, apiErr.Status)
	// 404 响应也应携带 rate limit 信息
	require.NotNil(t, rl)
	assert.Equal(t, 58, rl.Remaining)
}

// TestGetRawContent_RateLimited403 覆盖 GitHub 特有的 403 + X-RateLimit-Remaining:0 → KindRateLimited。
func TestGetRawContent_RateLimited403(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", "1700000123")
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	c := NewClient()
	c.baseURL = srv.URL

	_, _, err := c.GetRawContent(context.Background(), "sigpanic", "goink-skills", "main", "index.json")
	require.Error(t, err)
	var apiErr *Error
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, KindRateLimited, apiErr.Kind)
	assert.Equal(t, http.StatusForbidden, apiErr.Status)
	assert.Equal(t, time.Unix(1700000123, 0).UTC(), apiErr.ResetAt)
}

// TestGetRawContent_Forbidden403 覆盖 403 但无 rate limit header → KindForbidden。
func TestGetRawContent_Forbidden403(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 不设置 X-RateLimit header，模拟真 forbidden（仓库私有等）
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	c := NewClient()
	c.baseURL = srv.URL

	_, _, err := c.GetRawContent(context.Background(), "sigpanic", "goink-skills", "main", "index.json")
	require.Error(t, err)
	var apiErr *Error
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, KindForbidden, apiErr.Kind)
	assert.Equal(t, http.StatusForbidden, apiErr.Status)
	assert.True(t, apiErr.ResetAt.IsZero())
}

// TestGetRawContent_RateLimited429 覆盖标准 429 → KindRateLimited。
func TestGetRawContent_RateLimited429(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := NewClient()
	c.baseURL = srv.URL

	_, _, err := c.GetRawContent(context.Background(), "sigpanic", "goink-skills", "main", "index.json")
	require.Error(t, err)
	var apiErr *Error
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, KindRateLimited, apiErr.Kind)
	assert.Equal(t, http.StatusTooManyRequests, apiErr.Status)
}

// TestGetRawContent_ServerError 覆盖 5xx → KindOther。
func TestGetRawContent_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewClient()
	c.baseURL = srv.URL

	_, _, err := c.GetRawContent(context.Background(), "sigpanic", "goink-skills", "main", "index.json")
	require.Error(t, err)
	var apiErr *Error
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, KindOther, apiErr.Kind)
	assert.Equal(t, http.StatusInternalServerError, apiErr.Status)
}

// TestGetRawContent_Timeout 覆盖请求超时 → KindNetwork。
func TestGetRawContent_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 模拟服务器处理慢，触发客户端超时
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewClient()
	c.baseURL = srv.URL
	// 用极短超时的 ctx 触发 DeadlineExceeded
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, _, err := c.GetRawContent(ctx, "sigpanic", "goink-skills", "main", "index.json")
	require.Error(t, err)
	var apiErr *Error
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, KindNetwork, apiErr.Kind)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

// TestGetRawContent_ConnectionRefused 覆盖连接被拒 → KindNetwork。
func TestGetRawContent_ConnectionRefused(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close() // 立即关闭，使端口不可达

	c := NewClient()
	c.baseURL = srv.URL

	_, _, err := c.GetRawContent(context.Background(), "sigpanic", "goink-skills", "main", "index.json")
	require.Error(t, err)
	var apiErr *Error
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, KindNetwork, apiErr.Kind)
}

// TestGetRawContent_CanceledContext 覆盖 context 取消 → KindNetwork。
func TestGetRawContent_CanceledContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewClient()
	c.baseURL = srv.URL

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	_, _, err := c.GetRawContent(ctx, "sigpanic", "goink-skills", "main", "index.json")
	require.Error(t, err)
	var apiErr *Error
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, KindNetwork, apiErr.Kind)
	assert.ErrorIs(t, err, context.Canceled)
}

// TestParseRateLimit 覆盖 parseRateLimit 各种 header 组合。
func TestParseRateLimit(t *testing.T) {
	t.Run("both headers present", func(t *testing.T) {
		h := http.Header{}
		h.Set("X-RateLimit-Remaining", "42")
		h.Set("X-RateLimit-Reset", "1700000000")
		rl := parseRateLimit(h)
		require.NotNil(t, rl)
		assert.Equal(t, 42, rl.Remaining)
		assert.Equal(t, time.Unix(1700000000, 0).UTC(), rl.ResetAt)
	})

	t.Run("both headers missing", func(t *testing.T) {
		h := http.Header{}
		rl := parseRateLimit(h)
		assert.Nil(t, rl)
	})

	t.Run("only remaining", func(t *testing.T) {
		h := http.Header{}
		h.Set("X-RateLimit-Remaining", "10")
		rl := parseRateLimit(h)
		require.NotNil(t, rl)
		assert.Equal(t, 10, rl.Remaining)
		assert.True(t, rl.ResetAt.IsZero())
	})

	t.Run("invalid values", func(t *testing.T) {
		h := http.Header{}
		h.Set("X-RateLimit-Remaining", "not-a-number")
		h.Set("X-RateLimit-Reset", "also-not-number")
		rl := parseRateLimit(h)
		// 解析失败时返回零值，但不返回 nil（因为 header 存在）
		require.NotNil(t, rl)
		assert.Equal(t, 0, rl.Remaining)
		assert.True(t, rl.ResetAt.IsZero())
	})
}

// TestClassifyStatus 覆盖 classifyStatus 各状态码。
func TestClassifyStatus(t *testing.T) {
	tests := []struct {
		name   string
		status int
		rl     *RateLimit
		want   Kind
	}{
		{"404", http.StatusNotFound, nil, KindNotFound},
		{"429", http.StatusTooManyRequests, nil, KindRateLimited},
		{"403 with remaining 0", http.StatusForbidden, &RateLimit{Remaining: 0}, KindRateLimited},
		{"403 with remaining 5", http.StatusForbidden, &RateLimit{Remaining: 5}, KindForbidden},
		{"403 with nil rl", http.StatusForbidden, nil, KindForbidden},
		{"500", http.StatusInternalServerError, nil, KindOther},
		{"400", http.StatusBadRequest, nil, KindOther},
		{"401", http.StatusUnauthorized, nil, KindOther},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyStatus(tt.status, tt.rl)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestKindString 覆盖 Kind.String() 各分支。
func TestKindString(t *testing.T) {
	assert.Equal(t, "network", KindNetwork.String())
	assert.Equal(t, "rate_limited", KindRateLimited.String())
	assert.Equal(t, "not_found", KindNotFound.String())
	assert.Equal(t, "forbidden", KindForbidden.String())
	assert.Equal(t, "other", KindOther.String())
}

// TestError_Unwrap 验证 *Error 的 Unwrap 链路。
func TestError_Unwrap(t *testing.T) {
	inner := errors.New("dial tcp: connection refused")
	e := &Error{Kind: KindNetwork, Message: "request failed", Cause: inner}
	assert.ErrorIs(t, e, inner)
}

// TestError_ErrorMessage 验证 Error() 输出格式。
func TestError_ErrorMessage(t *testing.T) {
	t.Run("with cause", func(t *testing.T) {
		e := &Error{Kind: KindNetwork, Message: "request failed", Cause: errors.New("timeout")}
		assert.Contains(t, e.Error(), "request failed")
		assert.Contains(t, e.Error(), "timeout")
	})
	t.Run("without cause", func(t *testing.T) {
		e := &Error{Kind: KindNotFound, Message: "HTTP 404"}
		assert.Contains(t, e.Error(), "HTTP 404")
	})
}
