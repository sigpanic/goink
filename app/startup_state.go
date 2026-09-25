package app

import (
	"errors"
	"fmt"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// StartupPhase 描述应用启动阶段，供前端决定渲染哪个界面。
//
// 单靠 IsInitialized() 的布尔量无法区分“未配置”与“初始化中 / 初始化失败”：
// 它返回的是 cfg != nil，而 cfg 只有全部初始化成功才赋值，于是后两种状态都
// 被折叠成 false，前端只能显示首次初始化界面——既误导用户，又让“开始使用”
// 按钮有机会触发第二次并发初始化。
type StartupPhase string

const (
	// PhaseUnconfigured 指针文件不存在，等待用户在前端完成首次初始化。
	PhaseUnconfigured StartupPhase = "unconfigured"
	// PhaseInitializing 包括读取启动配置及 initWithConfig 进行中的全部准备工作。
	PhaseInitializing StartupPhase = "initializing"
	// PhaseReady 运行时模块全部就绪，可以进入主界面。
	PhaseReady StartupPhase = "ready"
	// PhaseFailed 初始化失败，Error 带原因；前端应显示错误页而非首次初始化界面。
	PhaseFailed StartupPhase = "failed"
)

// StartupState 是启动状态的快照。Version 在每次转换时递增，前端据此丢弃
// 乱序到达的旧事件；值语义可安全跨 goroutine 传递。
type StartupState struct {
	Phase   StartupPhase `json:"phase"`
	Error   string       `json:"error,omitempty"`
	Version uint64       `json:"version"`
}

// startupEventName 是启动状态变化事件名，前端用 EventsOn 订阅。
const startupEventName = "startup:state"

// setStartupState 写入一个新状态，并在前端已就绪时推送版本化快照。
func (a *App) setStartupState(phase StartupPhase, errMsg string) {
	a.startupMu.Lock()
	snapshot := a.setStartupStateLocked(phase, errMsg)
	ready := a.frontendReady
	a.startupMu.Unlock()

	a.emitStartupState(ready, snapshot)
}

// setStartupStateLocked 需要调用方持有 startupMu。
func (a *App) setStartupStateLocked(phase StartupPhase, errMsg string) StartupState {
	a.startupState = StartupState{
		Phase:   phase,
		Error:   errMsg,
		Version: a.startupState.Version + 1,
	}
	return a.startupState
}

// emitStartupState 在锁外发送事件，避免事件派发阻塞状态转换。
func (a *App) emitStartupState(ready bool, snapshot StartupState) {
	if ready && a.ctx != nil {
		runtime.EventsEmit(a.ctx, startupEventName, snapshot)
	}
}

// beginInitialization 原子地校验来源状态并进入 initializing，防止多个前端调用
// 在“检查状态”与“实际开始初始化”之间同时穿透。
func (a *App) beginInitialization(from StartupPhase) error {
	// ready 是终态，不允许从它再进入初始化。本函数只负责切换状态，真正执行初始化的是
	// 调用方（Initialize / RetryStartup）——这里放行就等于让它们在一个已就绪的应用上
	// 重跑一遍 initWithConfig（重开数据库、重跑迁移）。检查放在锁外：from 是调用方
	// 传入的参数，不读共享状态，无需持锁。
	if from == PhaseReady {
		return errors.New("应用已就绪，无需再次初始化")
	}

	a.startupMu.Lock()
	current := a.startupState.Phase
	if current != from {
		a.startupMu.Unlock()
		if current == PhaseInitializing {
			return errors.New("应用正在初始化，请稍候")
		}
		return fmt.Errorf("当前启动状态为 %q，不能开始初始化", current)
	}
	snapshot := a.setStartupStateLocked(PhaseInitializing, "")
	ready := a.frontendReady
	a.startupMu.Unlock()

	a.emitStartupState(ready, snapshot)
	return nil
}

// GetStartupState 返回当前启动状态。
//
// 前端可在任意时刻调用：Wails 的绑定在 CreateApp 阶段就注册完毕，早于
// OnStartup 的执行，因此初始化期间调用是合法的；而本方法只读内存字段，
// 不触碰 db 与各领域 store，所以也不会 nil panic。
func (a *App) GetStartupState() StartupState {
	a.startupMu.Lock()
	defer a.startupMu.Unlock()
	return a.startupState
}

// isInitializing 报告是否正处于初始化中，用于迁移期间的关窗确认。
func (a *App) isInitializing() bool {
	return a.GetStartupState().Phase == PhaseInitializing
}

// FrontendReady 由前端在挂载完成、且已注册 startup:state 监听之后调用。
//
// 它在同一临界区内标记前端就绪并返回当前快照：此前状态由返回值补齐，此后
// 状态转换都会发事件。前端按 Version 应用快照，因此无需依赖事件投递顺序。
// 在 React StrictMode 下重复调用也是安全的。
func (a *App) FrontendReady() StartupState {
	a.startupMu.Lock()
	a.frontendReady = true
	snapshot := a.startupState
	a.startupMu.Unlock()
	return snapshot
}
