package app

import (
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newStartupStateTestApp() *App {
	logger := slog.New(slog.NewTextHandler(nil, &slog.HandlerOptions{Level: slog.LevelError}))
	return New(logger)
}

func TestStartupState_InitialSnapshot(t *testing.T) {
	a := newStartupStateTestApp()

	assert.Equal(t, StartupState{
		Phase:   PhaseInitializing,
		Version: 1,
	}, a.GetStartupState())
}

func TestStartupState_FrontendReadyReturnsSnapshotWithoutTransition(t *testing.T) {
	a := newStartupStateTestApp()
	a.setStartupState(PhaseUnconfigured, "")

	snapshot := a.FrontendReady()
	assert.Equal(t, a.GetStartupState(), snapshot)
	assert.Equal(t, uint64(2), snapshot.Version)

	a.setStartupState(PhaseInitializing, "")
	assert.Equal(t, StartupState{
		Phase:   PhaseInitializing,
		Version: 3,
	}, a.FrontendReady())
}

func TestStartupState_BeginInitializationIsAtomic(t *testing.T) {
	a := newStartupStateTestApp()
	a.setStartupState(PhaseUnconfigured, "")

	var wg sync.WaitGroup
	var successes atomic.Int32
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if a.beginInitialization(PhaseUnconfigured) == nil {
				successes.Add(1)
			}
		}()
	}
	wg.Wait()

	require.Equal(t, int32(1), successes.Load())
	assert.Equal(t, StartupState{
		Phase:   PhaseInitializing,
		Version: 3,
	}, a.GetStartupState())
}

func TestStartupState_ConcurrentFrontendReadyAndTransitions(t *testing.T) {
	a := newStartupStateTestApp()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for range 100 {
			a.setStartupState(PhaseFailed, "temporary failure")
			a.setStartupState(PhaseInitializing, "")
		}
	}()
	go func() {
		defer wg.Done()
		for range 100 {
			_ = a.FrontendReady()
		}
	}()
	wg.Wait()

	assert.Equal(t, uint64(201), a.GetStartupState().Version)
}
