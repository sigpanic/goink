package agent

import (
	"time"
)

// P2 可恢复错误重试配置
const (
	maxRetries = 8 // 最大重试次数

	// maxRetryAfter 限制服务商通过 Retry-After header 指定的等待时间上限。
	// 避免服务商异常或恶意指定过长等待导致对话长时间挂起。
	maxRetryAfter = 120 * time.Second
)

// backoffBase 退避基数（已含抖动，不再额外加 jitter）。
// 8 次重试对应 2.347s / 4.618s / 8.283s / 16.742s / 31.189s / 48.631s / 60.473s / 60.218s。
// 总等待时间约 232.5s。
// 实测效果好于随机延迟退避
var backoffBase = [...]time.Duration{
	2347 * time.Millisecond,
	4618 * time.Millisecond,
	8283 * time.Millisecond,
	16742 * time.Millisecond,
	31189 * time.Millisecond,
	48631 * time.Millisecond,
	60473 * time.Millisecond,
	60218 * time.Millisecond,
}

// computeBackoff 计算重试退避时间。
//
// attempt: 第几次重试（1-indexed，1..maxRetries）
// retryAfter: 服务商通过 Retry-After header 指定的重试等待时间，>0 时优先遵循（上限 maxRetryAfter）
//
// 返回退避时间。
func computeBackoff(attempt int, retryAfter time.Duration) time.Duration {
	// 服务商指定优先遵循
	if retryAfter > 0 {
		if retryAfter > maxRetryAfter {
			return maxRetryAfter
		}
		return retryAfter
	}

	// 越界保护
	if attempt < 1 || attempt > len(backoffBase) {
		return 1 * time.Second
	}

	return backoffBase[attempt-1]
}
