package agent

import (
	"testing"
	"time"
)

func TestComputeBackoff_RetryAfterTakesPrecedence(t *testing.T) {
	// Retry-After header 优先遵循
	got := computeBackoff(1, 30*time.Second)
	if got != 30*time.Second {
		t.Errorf("retry-after 30s: got %v, want 30s", got)
	}
}

func TestComputeBackoff_RetryAfterCappedAtMax(t *testing.T) {
	// Retry-After 超过 120s 上限
	got := computeBackoff(1, 200*time.Second)
	if got != maxRetryAfter {
		t.Errorf("retry-after 200s: got %v, want %v (capped)", got, maxRetryAfter)
	}
}

func TestComputeBackoff_FixedBackoffBase(t *testing.T) {
	// 无 Retry-After header 时使用固定基数
	expected := []time.Duration{
		2347 * time.Millisecond,
		4618 * time.Millisecond,
		8283 * time.Millisecond,
		16742 * time.Millisecond,
		31189 * time.Millisecond,
		48631 * time.Millisecond,
		60473 * time.Millisecond,
		60218 * time.Millisecond,
	}
	for i, want := range expected {
		attempt := i + 1
		got := computeBackoff(attempt, 0)
		if got != want {
			t.Errorf("attempt %d: got %v, want %v", attempt, got, want)
		}
	}
}

func TestComputeBackoff_InvalidAttemptFallback(t *testing.T) {
	// attempt 越界时回退到 1s
	got := computeBackoff(0, 0)
	if got != 1*time.Second {
		t.Errorf("attempt 0: got %v, want 1s", got)
	}
	got = computeBackoff(len(backoffBase)+1, 0)
	if got != 1*time.Second {
		t.Errorf("attempt %d: got %v, want 1s", len(backoffBase)+1, got)
	}
}

func TestComputeBackoff_ZeroRetryAfterUsesBackoff(t *testing.T) {
	// Retry-After=0 等价于未设置，使用 backoffBase
	got := computeBackoff(1, 0)
	if got != backoffBase[0] {
		t.Errorf("retry-after 0: got %v, want %v (use backoffBase)", got, backoffBase[0])
	}
}
