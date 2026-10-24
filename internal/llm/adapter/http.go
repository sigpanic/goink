package adapter

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ParseRetryAfter 解析 Retry-After header。
// 支持两种格式：
//   - 秒数：如 "120" 表示 120 秒后重试
//   - HTTP-date：如 "Wed, 21 Oct 2026 07:28:00 GMT"
//
// 解析失败、为空或秒数超出 time.Duration 范围时返回 0。
func ParseRetryAfter(value string) time.Duration {
	value = strings.TrimSpace(value)
	// 优先尝试秒数
	if secs, err := strconv.ParseInt(value, 10, 64); err == nil {
		if secs >= 0 && secs <= int64((1<<63-1)/time.Second) {
			return time.Duration(secs) * time.Second
		}
		return 0
	}
	// 尝试 HTTP-date
	if date, err := http.ParseTime(value); err == nil {
		if delay := time.Until(date); delay > 0 {
			return delay
		}
	}
	return 0
}

// ParseChatURL 只校验完整地址，不补路径、移除后缀或猜测版本；根地址也可作为后续连接测试的候选。
// 保留路径、转义和查询参数，不执行协议专属的端点转换。
func ParseChatURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("chat_url requires an absolute HTTP(S) URL")
	}
	if u.User != nil || u.Fragment != "" {
		return nil, fmt.Errorf("chat_url must not contain user info or a fragment")
	}
	return u, nil
}
