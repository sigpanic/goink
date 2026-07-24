// Package githubapi 封装 GitHub REST API 的轻量匿名客户端。
//
// 仅支持未认证调用（受 60 req/h 限制），调用方应自行缓存结果。
// 错误以语义类别（Kind）暴露，便于上层针对网络失败、rate limit、404 等
// 给出不同的用户提示。
package githubapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/sigpanic/goink/internal/apperr"
)

const (
	defaultTimeout = 5 * time.Second
	userAgent      = "Goink"
	acceptRaw      = "application/vnd.github.raw+json" // Contents API 直接返回原始内容，避免 base64 包装
	apiBase        = "https://api.github.com"
)

// Kind 表示 GitHub API 错误的语义类别。
type Kind int

const (
	KindNetwork     Kind = iota // 网络层错误（超时、连接拒绝、DNS 失败）
	KindRateLimited             // rate limit 触发：403 + X-RateLimit-Remaining:0，或 429
	KindNotFound                // 404
	KindForbidden               // 403 非 rate limit（仓库私有、被屏蔽等）
	KindOther                   // 其他 4xx/5xx
)

// String 返回 Kind 的可读名称，便于日志和调试。
func (k Kind) String() string {
	switch k {
	case KindNetwork:
		return "network"
	case KindRateLimited:
		return "rate_limited"
	case KindNotFound:
		return "not_found"
	case KindForbidden:
		return "forbidden"
	default:
		return "other"
	}
}

// Error 是 GitHub API 调用返回的语义化错误。
// 调用方可用 errors.As 提取 *Error 后根据 Kind 分支处理。
type Error struct {
	Kind    Kind
	Status  int       // HTTP 状态码，网络层错误为 0
	Message string    // 人类可读的错误描述
	ResetAt time.Time // rate limit 重置时间，仅 Kind == KindRateLimited 时有效
	Cause   error     // 底层错误（网络层错误时非 nil）
}

// Error 实现 error 接口。
func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("githubapi: %s: %v", e.Message, e.Cause)
	}
	return fmt.Sprintf("githubapi: %s", e.Message)
}

// Unwrap 暴露底层错误，支持 errors.Is/errors.As。
func (e *Error) Unwrap() error { return e.Cause }

// Code 实现 apperr.Coder 接口，按 Kind 映射到 apperr.Code。
// 映射逻辑从 apperr.codeFromGitHubAPIError 搬来（v1.3.0 集中映射改接口模式后，
// apperr 不再 import githubapi，改由各领域 error 自行实现 Code()）。
func (e *Error) Code() apperr.Code {
	switch e.Kind {
	case KindNetwork:
		return apperr.CodeGitHubAPINetwork
	case KindRateLimited:
		return apperr.CodeGitHubAPIRateLimited
	case KindNotFound:
		return apperr.CodeGitHubAPINotFound
	case KindForbidden:
		return apperr.CodeGitHubAPIForbidden
	default: // 含 KindOther
		return apperr.CodeGitHubAPIOther
	}
}

// RateLimit 携带本次请求后的 rate limit 配额信息。
// GitHub 在每个 API 响应的 header 中返回这些字段。
type RateLimit struct {
	Remaining int       // 剩余请求数
	ResetAt   time.Time // 配额重置时间（UTC）
}

// Client 是匿名调用的 GitHub API 客户端。
//
// 受未认证 60 req/h 限制，调用方应缓存结果避免频繁请求。
// 零值不可用，必须通过 NewClient 创建。
type Client struct {
	http      *http.Client
	userAgent string
	baseURL   string // 默认为 apiBase，测试时可覆盖指向 httptest.Server
}

// NewClient 创建一个使用默认配置（5s 超时、Goink User-Agent）的客户端。
func NewClient() *Client {
	return &Client{
		http:      &http.Client{Timeout: defaultTimeout},
		userAgent: userAgent,
		baseURL:   apiBase,
	}
}

// GetRawContent 通过 GitHub Contents API 拉取指定文件的原始内容。
//
// 使用 Accept: application/vnd.github.raw+json header 直接返回 raw 内容
// （而非默认的 base64 编码 JSON 包装），便于直接使用。
//
// 成功返回内容字节、rate limit 信息和 nil error。
// 失败返回 nil、响应中已知的 rate limit 信息（可能为 nil）和 *Error。
func (c *Client) GetRawContent(ctx context.Context, owner, repo, branch, path string) ([]byte, *RateLimit, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/contents/%s?ref=%s", c.baseURL, owner, repo, path, branch)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, nil, &Error{Kind: KindOther, Message: "build request", Cause: err}
	}
	req.Header.Set("Accept", acceptRaw)
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		kind := KindOther
		if isNetworkError(err) {
			kind = KindNetwork
		}
		return nil, nil, &Error{Kind: kind, Message: "request failed", Cause: err}
	}
	defer resp.Body.Close()

	rl := parseRateLimit(resp.Header)

	if resp.StatusCode == http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, rl, &Error{Kind: KindOther, Message: "read response body", Cause: err}
		}
		return body, rl, nil
	}

	kind := classifyStatus(resp.StatusCode, rl)
	e := &Error{
		Kind:    kind,
		Status:  resp.StatusCode,
		Message: fmt.Sprintf("HTTP %d for %s/%s@%s:%s", resp.StatusCode, owner, repo, branch, path),
	}
	if kind == KindRateLimited && rl != nil {
		e.ResetAt = rl.ResetAt
	}
	return nil, rl, e
}

// isNetworkError 判断是否为网络层错误（超时、连接拒绝、DNS 失败、context 取消等）。
// 这类错误通常是暂时的，调用方可以提示用户检查网络后重试。
func isNetworkError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}

// classifyStatus 根据 HTTP 状态码和已解析的 rate limit 信息分类错误。
//
// 注意：GitHub API 的 rate limit 实际返回 403（而非标准 429），
// 通过 response header 的 X-RateLimit-Remaining: 0 来区分 rate limit 与真 forbidden。
func classifyStatus(status int, rl *RateLimit) Kind {
	switch status {
	case http.StatusNotFound:
		return KindNotFound
	case http.StatusTooManyRequests:
		return KindRateLimited
	case http.StatusForbidden:
		// GitHub 特有行为：rate limit 时返回 403 + X-RateLimit-Remaining: 0
		if rl != nil && rl.Remaining == 0 {
			return KindRateLimited
		}
		return KindForbidden
	default:
		return KindOther
	}
}

// parseRateLimit 从 response header 解析 rate limit 信息。
//
// 解析 GitHub 返回的 X-RateLimit-Remaining 和 X-RateLimit-Reset 两个 header。
// 字段缺失时对应零值；两个 header 都缺失时返回 nil。
func parseRateLimit(h http.Header) *RateLimit {
	remainingStr := h.Get("X-RateLimit-Remaining")
	resetStr := h.Get("X-RateLimit-Reset")
	if remainingStr == "" && resetStr == "" {
		return nil
	}
	rl := &RateLimit{}
	if n, err := strconv.Atoi(remainingStr); err == nil {
		rl.Remaining = n
	}
	if unix, err := strconv.ParseInt(resetStr, 10, 64); err == nil {
		rl.ResetAt = time.Unix(unix, 0).UTC()
	}
	return rl
}
