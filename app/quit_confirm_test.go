package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDecideClose(t *testing.T) {
	tests := []struct {
		name          string
		phase         StartupPhase
		frontendReady bool
		pending       bool
		confirmed     bool
		want          quitDecision
	}{
		{"非初始化阶段直接放行", PhaseReady, true, false, false, quitAllow},
		{"未配置阶段直接放行", PhaseUnconfigured, true, false, false, quitAllow},
		{"初始化失败阶段直接放行", PhaseFailed, true, false, false, quitAllow},
		{"前端未就绪时放行，避免关不掉窗口", PhaseInitializing, false, false, false, quitAllow},
		{"已确认时放行，避免重复弹窗", PhaseInitializing, true, false, true, quitAllow},
		{"首次关窗请求弹确认", PhaseInitializing, true, false, false, quitConfirm},
		{"确认期间再次关窗视为强制退出", PhaseInitializing, true, true, false, quitForce},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, decideClose(tt.phase, tt.frontendReady, tt.pending, tt.confirmed))
		})
	}
}

func TestQuitConfirmMarkers(t *testing.T) {
	// 用户点「继续等待」：标记清除后，下一次关窗仍会弹确认。
	a := newStartupStateTestApp()
	a.setStartupState(PhaseInitializing, "")
	a.frontendReady = true
	a.quitConfirmPending = true

	a.CancelQuit()
	assert.Equal(t, quitConfirm, decideClose(a.startupState.Phase, a.frontendReady, a.quitConfirmPending, a.quitConfirmed))

	// 用户点「退出」：登记已确认后，Quit 触发的 OnBeforeClose 直接放行。
	// 此处 ctx 为 nil，ConfirmQuit 不会真的退出进程。
	a.ConfirmQuit()
	assert.Equal(t, quitAllow, decideClose(a.startupState.Phase, a.frontendReady, a.quitConfirmPending, a.quitConfirmed))
}
