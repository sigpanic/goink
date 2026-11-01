package adapter

import "context"

// SendEvent 避免调用方取消后停止消费，导致生产方阻塞在 channel 发送上。
func SendEvent(ctx context.Context, ch chan<- StreamEvent, event StreamEvent) bool {
	if ctx.Err() != nil {
		return false
	}
	select {
	case ch <- event:
		return true
	case <-ctx.Done():
		return false
	}
}
