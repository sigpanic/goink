package engine

import (
	"errors"
	"log/slog"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type callbackStep struct {
	key string
	run func() error
}

func (s callbackStep) Key() string                          { return s.key }
func (s callbackStep) Run(_ *gorm.DB, _ *slog.Logger) error { return s.run() }

func TestRunStepsResumesAfterFailedStep(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&MigrateState{}); err != nil {
		t.Fatal(err)
	}

	calls := map[string]int{}
	steps := []Step{
		callbackStep{key: "first", run: func() error {
			calls["first"]++
			return nil
		}},
		callbackStep{key: "second", run: func() error {
			calls["second"]++
			if calls["second"] == 1 {
				return errors.New("simulated interruption after side effect")
			}
			return nil
		}},
		callbackStep{key: "third", run: func() error {
			calls["third"]++
			return nil
		}},
	}

	if err := RunSteps(db, slog.Default(), "test", steps); err == nil {
		t.Fatal("首次运行应在第二步中断")
	}
	if calls["first"] != 1 || calls["second"] != 1 || calls["third"] != 0 {
		t.Fatalf("首次运行次数错误: %#v", calls)
	}
	assertStepState(t, db, "test", "first", "done", "")
	assertStepState(t, db, "test", "second", "failed", "simulated interruption after side effect")

	if err := RunSteps(db, slog.Default(), "test", steps); err != nil {
		t.Fatalf("中断恢复运行: %v", err)
	}
	if calls["first"] != 1 || calls["second"] != 2 || calls["third"] != 1 {
		t.Fatalf("恢复运行次数错误: %#v", calls)
	}
	assertStepState(t, db, "test", "first", "done", "")
	assertStepState(t, db, "test", "second", "done", "")
	assertStepState(t, db, "test", "third", "done", "")
}

func assertStepState(t *testing.T, db *gorm.DB, migration, key, wantStatus, wantError string) {
	t.Helper()
	var state MigrateState
	if err := db.Where("migration = ? AND step = ?", migration, key).First(&state).Error; err != nil {
		t.Fatal(err)
	}
	if state.Status != wantStatus || state.Error != wantError {
		t.Fatalf("step %s state = status=%q error=%q, want status=%q error=%q", key, state.Status, state.Error, wantStatus, wantError)
	}
}
