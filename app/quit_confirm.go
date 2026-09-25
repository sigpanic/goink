package app

import (
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// quitConfirmEventName 是请求前端弹出关窗确认的事件名，前端用 EventsOn 订阅。
const quitConfirmEventName = "app:quit-confirm"

// quitDecision 是一次关窗请求的处理结论。
type quitDecision int

const (
	// quitAllow 直接放行，不弹确认。
	quitAllow quitDecision = iota
	// quitConfirm 已拦截本次关闭，需要前端弹出确认。
	quitConfirm
	// quitForce 用户已在确认中再次请求关闭，视为放弃等待，放行。
	quitForce
)

// decideClose 根据启动阶段与关窗标记决定如何处理本次关闭请求。
//
// 只有「初始化中 + 前端能弹窗 + 尚未确认过」才值得拦一次，其余一律放行：前端不可用
// 时根本弹不出确认，用户已在确认中时再拦一次等于把他关在窗口里——宁可放行，不可阻塞
// 退出。做成纯函数是为了让这几条兜底规则可以直接表驱动测试，不必构造 App。
func decideClose(phase StartupPhase, frontendReady, confirmPending, confirmed bool) quitDecision {
	switch {
	case phase != PhaseInitializing:
		return quitAllow
	case !frontendReady:
		return quitAllow
	case confirmed:
		return quitAllow
	case confirmPending:
		return quitForce
	default:
		return quitConfirm
	}
}

// ConfirmQuit 由前端在用户确认退出后调用。
//
// 先登记「已确认」再退出：runtime.Quit 会再次触发 OnBeforeClose，这个标记让那一次
// 直接放行，否则又会弹出一轮确认。
func (a *App) ConfirmQuit() {
	a.startupMu.Lock()
	a.quitConfirmed = true
	a.quitConfirmPending = false
	a.startupMu.Unlock()

	if a.ctx != nil {
		runtime.Quit(a.ctx)
	}
}

// CancelQuit 由前端在用户选择继续等待后调用。
//
// 必须清除待确认标记：否则用户下次关窗会被 decideClose 当成「确认期间再次关闭」
// 而直接放行，等于第二次关闭不再劝阻。
func (a *App) CancelQuit() {
	a.startupMu.Lock()
	a.quitConfirmPending = false
	a.startupMu.Unlock()
}
