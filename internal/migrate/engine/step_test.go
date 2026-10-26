package engine

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

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

func TestMarkStepPreservesStateHistory(t *testing.T) {
	for _, status := range []string{"running", "done", "failed"} {
		t.Run(status, func(t *testing.T) {
			db := openEngineTestDB(t)
			created := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
			started := created.Add(time.Minute)
			finished := started.Add(time.Minute)
			original := MigrateState{
				Migration: "history", Step: "step", Status: "failed", Error: "old failure",
				StartedAt: &started, FinishedAt: &finished, CreatedAt: created, UpdatedAt: finished,
			}
			if err := db.Create(&original).Error; err != nil {
				t.Fatal(err)
			}
			runErr := errors.New("new failure")
			before := time.Now()
			if err := markStep(db, slog.Default(), "history", "step", status, runErr); err != nil {
				t.Fatal(err)
			}
			var state MigrateState
			if err := db.Where("migration = ? AND step = ?", "history", "step").First(&state).Error; err != nil {
				t.Fatal(err)
			}
			if state.ID != original.ID || !state.CreatedAt.Equal(created) || state.UpdatedAt.Before(before) || state.Status != status {
				t.Fatalf("更新状态应保留记录身份和创建时间，并刷新更新时间: %+v", state)
			}
			if status == "running" {
				if state.StartedAt == nil || state.StartedAt.Before(before) || state.FinishedAt != nil || state.Error != "" {
					t.Fatalf("重试应更新开始时间并清除上次结束时间和错误: %+v", state)
				}
				return
			}
			if state.StartedAt == nil || !state.StartedAt.Equal(started) || state.FinishedAt == nil || state.FinishedAt.Before(before) {
				t.Fatalf("结束状态应保留本次开始时间并刷新结束时间: %+v", state)
			}
			wantError := ""
			if status == "failed" {
				wantError = runErr.Error()
			}
			if state.Error != wantError {
				t.Fatalf("错误信息 = %q，期望 %q", state.Error, wantError)
			}
		})
	}
}

func TestMarkStepsDoneRollsBackWholeGroup(t *testing.T) {
	db := openEngineTestDB(t)
	if err := markStep(db, slog.Default(), "atomic", "existing", "running", nil); err != nil {
		t.Fatal(err)
	}
	if err := markStep(db, slog.Default(), "atomic", "existing", "failed", errors.New("keep failure")); err != nil {
		t.Fatal(err)
	}
	var original MigrateState
	if err := db.Where("migration = ? AND step = ?", "atomic", "existing").First(&original).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TRIGGER reject_last_step BEFORE INSERT ON migrate_state
		WHEN NEW.migration = 'atomic' AND NEW.step = 'last'
		BEGIN SELECT RAISE(ABORT, 'last step blocked'); END`).Error; err != nil {
		t.Fatal(err)
	}
	steps := []Step{testStep("existing"), testStep("new"), testStep("last")}
	if err := MarkStepsDone(db, slog.Default(), "atomic", steps); err == nil || !strings.Contains(err.Error(), "last step blocked") {
		t.Fatalf("整组登记必须返回最后一步的写入错误: %v", err)
	}
	assertStepState(t, db, "atomic", "existing", "failed", "keep failure")
	var count int64
	if err := db.Model(&MigrateState{}).Where("migration = ?", "atomic").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("失败后新增记录必须回滚: rows=%d", count)
	}
	if err := db.Exec("DROP TRIGGER reject_last_step").Error; err != nil {
		t.Fatal(err)
	}
	if err := MarkStepsDone(db, slog.Default(), "atomic", steps); err != nil {
		t.Fatalf("解除故障后应允许重新登记整组: %v", err)
	}
	for _, step := range steps {
		assertStepState(t, db, "atomic", step.Key(), "done", "")
		var state MigrateState
		if err := db.Where("migration = ? AND step = ?", "atomic", step.Key()).First(&state).Error; err != nil {
			t.Fatal(err)
		}
		if state.FinishedAt == nil {
			t.Fatal("登记完成必须写入结束时间")
		}
		if step.Key() == "existing" {
			if state.ID != original.ID || !state.CreatedAt.Equal(original.CreatedAt) || state.StartedAt == nil || !state.StartedAt.Equal(*original.StartedAt) {
				t.Fatalf("整组登记应保留既有步骤的身份和开始时间: %+v", state)
			}
		} else if state.StartedAt != nil {
			t.Fatalf("未实际执行的步骤不应有开始时间: %+v", state)
		}
	}
}

func TestRunStepsStopsOnStateWriteFailure(t *testing.T) {
	for _, status := range []string{"running", "done", "failed"} {
		t.Run(status, func(t *testing.T) {
			db := openEngineTestDB(t)
			for _, operation := range []string{"INSERT", "UPDATE"} {
				query := fmt.Sprintf(`CREATE TRIGGER reject_%s BEFORE %s ON migrate_state
					WHEN NEW.status = '%s' BEGIN SELECT RAISE(ABORT, 'state write blocked'); END`, operation, operation, status)
				if err := db.Exec(query).Error; err != nil {
					t.Fatal(err)
				}
			}
			firstCalls, nextCalls := 0, 0
			businessErr := errors.New("business failure")
			steps := []Step{
				callbackStep{key: "first", run: func() error {
					firstCalls++
					if status == "failed" && firstCalls == 1 {
						return businessErr
					}
					return nil
				}},
				callbackStep{key: "next", run: func() error { nextCalls++; return nil }},
			}
			err := RunSteps(db, slog.Default(), "strict", steps)
			if err == nil || !strings.Contains(err.Error(), "state write blocked") {
				t.Fatalf("状态写入失败必须返回: %v", err)
			}
			if status == "failed" && !errors.Is(err, businessErr) {
				t.Fatalf("状态写入失败不能遮蔽业务错误: %v", err)
			}
			if nextCalls != 0 || (status == "running" && firstCalls != 0) {
				t.Fatalf("状态未落盘却执行了后续步骤: first=%d next=%d", firstCalls, nextCalls)
			}
			for _, operation := range []string{"INSERT", "UPDATE"} {
				if err := db.Exec("DROP TRIGGER reject_" + operation).Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := RunSteps(db, slog.Default(), "strict", steps); err != nil {
				t.Fatalf("解除写入故障后应恢复: %v", err)
			}
			assertStepState(t, db, "strict", "first", "done", "")
			assertStepState(t, db, "strict", "next", "done", "")
		})
	}
}

func TestRunStepsStopsOnStateReadFailure(t *testing.T) {
	db := openEngineTestDB(t)
	if err := db.Migrator().DropTable(&MigrateState{}); err != nil {
		t.Fatal(err)
	}
	called := false
	err := RunSteps(db, slog.Default(), "read-failure", []Step{callbackStep{key: "first", run: func() error {
		called = true
		return nil
	}}})
	if err == nil || called {
		t.Fatalf("读取状态失败应停止: called=%v err=%v", called, err)
	}
}

type schemaRecoveryModel struct {
	ID     int64 `gorm:"primaryKey"`
	Legacy string
}

func (schemaRecoveryModel) TableName() string { return "schema_recovery" }

type schemaRecoverySecond struct {
	ID int64 `gorm:"primaryKey"`
}

func (schemaRecoverySecond) TableName() string { return "schema_recovery_second" }

func TestAutoMigrateStepResumesPartialSchema(t *testing.T) {
	db := openEngineTestDB(t)
	blocked := errors.New("second table blocked")
	if err := db.Callback().Raw().Before("gorm:raw").Register("test:block-schema", func(tx *gorm.DB) {
		if strings.Contains(tx.Statement.SQL.String(), "CREATE TABLE `schema_recovery_second`") {
			tx.AddError(blocked)
		}
	}); err != nil {
		t.Fatal(err)
	}
	postCalls := 0
	steps := []Step{
		AutoMigrateStep{Models: func() []any { return []any{&schemaRecoveryModel{}, &schemaRecoverySecond{}} }},
		callbackStep{key: "post", run: func() error { postCalls++; return nil }},
	}
	if err := RunSteps(db, slog.Default(), "schema", steps); !errors.Is(err, blocked) {
		t.Fatalf("应在第二张表中断: %v", err)
	}
	if !db.Migrator().HasTable(&schemaRecoveryModel{}) || db.Migrator().HasTable(&schemaRecoverySecond{}) || postCalls != 0 {
		t.Fatal("中断时只应建完第一张表，尚未进入 post")
	}
	if err := db.Callback().Raw().Remove("test:block-schema"); err != nil {
		t.Fatal(err)
	}
	if err := RunSteps(db, slog.Default(), "schema", steps); err != nil {
		t.Fatal(err)
	}
	if !db.Migrator().HasTable(&schemaRecoverySecond{}) || postCalls != 1 {
		t.Fatal("恢复后应建完第二张表再执行 post")
	}
	assertStepState(t, db, "schema", "auto", "done", "")
}

func TestPostInterruptionDoesNotReplayAutoMigrate(t *testing.T) {
	db := openEngineTestDB(t)
	postCalls := 0
	steps := []Step{
		AutoMigrateStep{Models: func() []any { return []any{&schemaRecoveryModel{}} }},
		callbackStep{key: "post", run: func() error {
			postCalls++
			if postCalls == 1 {
				if err := db.Exec("ALTER TABLE schema_recovery DROP COLUMN legacy").Error; err != nil {
					return err
				}
				return errors.New("post interrupted after dropping column")
			}
			if db.Migrator().HasColumn(&schemaRecoveryModel{}, "legacy") {
				return errors.New("auto recreated a column already removed by post")
			}
			return nil
		}},
	}
	if err := RunSteps(db, slog.Default(), "schema", steps); err == nil {
		t.Fatal("首次应在 post 中断")
	}
	assertStepState(t, db, "schema", "auto", "done", "")
	if err := RunSteps(db, slog.Default(), "schema", steps); err != nil {
		t.Fatal(err)
	}
	if postCalls != 2 || db.Migrator().HasColumn(&schemaRecoveryModel{}, "legacy") {
		t.Fatal("恢复 post 时不得重放 auto")
	}
}
