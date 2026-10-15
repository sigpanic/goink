package engine

import (
	"errors"
	"log/slog"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type testStep string

func (s testStep) Key() string { return string(s) }
func (s testStep) Run(_ *gorm.DB, _ *slog.Logger) error {
	return nil
}

func TestBuildPlansUsesStateBeforeSchemaDetection(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&MigrateState{}); err != nil {
		t.Fatal(err)
	}

	detected := 0
	migration := Migration{
		Name: "test",
		NeedsMigration: func(_ *gorm.DB) (bool, error) {
			detected++
			return true, nil
		},
		PreSchemaSteps:  []Step{testStep("pre")},
		PostSchemaSteps: []Step{testStep("post")},
	}

	plans, err := BuildPlans(db, slog.Default(), true, []Migration{migration})
	if err != nil {
		t.Fatal(err)
	}
	if plans[0].Action != PlanMarkDone || detected != 0 {
		t.Fatalf("新库应直接登记 done: action=%v detected=%d", plans[0].Action, detected)
	}

	if err := db.Create(&MigrateState{Migration: "test", Step: "pre", Status: "done"}).Error; err != nil {
		t.Fatal(err)
	}
	plans, err = BuildPlans(db, slog.Default(), false, []Migration{migration})
	if err != nil {
		t.Fatal(err)
	}
	if plans[0].Action != PlanRun || detected != 0 {
		t.Fatalf("不完整状态应直接续跑: action=%v detected=%d", plans[0].Action, detected)
	}

	if err := db.Create(&MigrateState{Migration: "test", Step: "post", Status: "done"}).Error; err != nil {
		t.Fatal(err)
	}
	plans, err = BuildPlans(db, slog.Default(), false, []Migration{migration})
	if err != nil {
		t.Fatal(err)
	}
	if plans[0].Action != PlanSkip || detected != 0 {
		t.Fatalf("全部 done 应跳过: action=%v detected=%d", plans[0].Action, detected)
	}
}

func TestBuildPlansIgnoresUnknownStepState(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&MigrateState{}); err != nil {
		t.Fatal(err)
	}

	migration := Migration{
		Name:           "test",
		NeedsMigration: func(_ *gorm.DB) (bool, error) { return true, nil },
		PreSchemaSteps: []Step{testStep("pre")},
	}
	for _, state := range []MigrateState{
		{Migration: "test", Step: "pre", Status: "done"},
		{Migration: "test", Step: "obsolete-step", Status: "done"},
	} {
		if err := db.Create(&state).Error; err != nil {
			t.Fatal(err)
		}
	}

	plans, err := BuildPlans(db, slog.Default(), false, []Migration{migration})
	if err != nil {
		t.Fatalf("未知历史步骤不应阻断启动: %v", err)
	}
	if plans[0].Action != PlanSkip {
		t.Fatalf("已知步骤均完成时应跳过: action=%v", plans[0].Action)
	}

	migration.PostSchemaSteps = []Step{testStep("post")}
	plans, err = BuildPlans(db, slog.Default(), false, []Migration{migration})
	if err != nil {
		t.Fatalf("未知历史步骤不应阻断续跑: %v", err)
	}
	if plans[0].Action != PlanRun {
		t.Fatalf("当前步骤缺失时应续跑: action=%v", plans[0].Action)
	}
}

func TestPrepareSequentialAdoptsCompletedLegacyGroups(t *testing.T) {
	db := openEngineTestDB(t)
	autoCalls := 0
	registry := []Migration{
		{Name: "with-post", PreSchemaSteps: []Step{testStep("pre")}, PostSchemaSteps: []Step{testStep("post")}},
		{Name: "pre-only", PreSchemaSteps: []Step{testStep("pre")}},
	}
	for i := range registry {
		migration := &registry[i]
		if err := MarkStepsDone(db, slog.Default(), migration.Name, migration.AllSteps()); err != nil {
			t.Fatal(err)
		}
		migration.SchemaStep = callbackStep{key: "auto", run: func() error { autoCalls++; return nil }}
	}
	if err := PrepareSequential(db, slog.Default(), registry); err != nil {
		t.Fatal(err)
	}
	plans, err := BuildPlans(db, slog.Default(), false, registry)
	if err != nil {
		t.Fatal(err)
	}
	for _, plan := range plans {
		if plan.Action != PlanSkip {
			t.Fatalf("旧完成组不应重跑: %+v", plan)
		}
		assertStepState(t, db, plan.Migration.Name, "auto", "done", "")
	}
	if autoCalls != 0 {
		t.Fatal("接纳旧记录不应执行历史 schema")
	}
	assertStepState(t, db, schedulerMigration, sequentialStep, "done", "")
}

func TestPrepareSequentialRejectsIncompleteLegacyGroupsAtomically(t *testing.T) {
	for _, phase := range []string{"pre-running", "pre-done", "post-failed"} {
		t.Run(phase, func(t *testing.T) {
			db := openEngineTestDB(t)
			registry := []Migration{
				{Name: "finished", PreSchemaSteps: []Step{testStep("pre")}, SchemaStep: testStep("auto")},
				{Name: "partial", PreSchemaSteps: []Step{testStep("pre")}, SchemaStep: testStep("auto"), PostSchemaSteps: []Step{testStep("post")}},
			}
			states := []MigrateState{
				{Migration: "finished", Step: "pre", Status: "done"},
				{Migration: "partial", Step: "pre", Status: "done"},
			}
			if phase == "pre-running" {
				states[1].Status = "running"
			}
			if phase == "post-failed" {
				states = append(states, MigrateState{Migration: "partial", Step: "post", Status: "failed"})
			}
			if err := db.Create(&states).Error; err != nil {
				t.Fatal(err)
			}
			if err := PrepareSequential(db, slog.Default(), registry); !errors.Is(err, ErrLegacyMigrationIncomplete) {
				t.Fatalf("旧中断组必须拒绝接管: %v", err)
			}
			var count int64
			if err := db.Model(&MigrateState{}).Count(&count).Error; err != nil {
				t.Fatal(err)
			}
			if count != int64(len(states)) {
				t.Fatalf("拒绝升级后不应留下 auto 或调度标记: rows=%d", count)
			}
		})
	}
}

func TestPrepareSequentialPreservesNewInterruptionBeforeAuto(t *testing.T) {
	db := openEngineTestDB(t)
	preCalls, autoCalls := 0, 0
	migration := Migration{
		Name:           "pre-only",
		PreSchemaSteps: []Step{callbackStep{key: "pre", run: func() error { preCalls++; return nil }}},
		SchemaStep:     callbackStep{key: "auto", run: func() error { autoCalls++; return nil }},
	}
	registry := []Migration{migration}
	if err := PrepareSequential(db, slog.Default(), registry); err != nil {
		t.Fatal(err)
	}
	// 模拟 pre 已完成、auto 尚未写 running 时进程中断；没有 post 可辅助辨别。
	if err := RunSteps(db, slog.Default(), migration.Name, migration.PreSchemaSteps); err != nil {
		t.Fatal(err)
	}
	if err := PrepareSequential(db, slog.Default(), registry); err != nil {
		t.Fatalf("新版中断应允许恢复: %v", err)
	}
	status, err := stepStatus(db, migration.Name, "auto")
	if err != nil || status != "" {
		t.Fatalf("不能把未执行的 auto 接纳为旧完成记录: status=%q err=%v", status, err)
	}
	if err := RunSteps(db, slog.Default(), migration.Name, migration.AllSteps()); err != nil {
		t.Fatal(err)
	}
	if preCalls != 1 || autoCalls != 1 {
		t.Fatalf("恢复调用次数: pre=%d auto=%d", preCalls, autoCalls)
	}
}

func openEngineTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&MigrateState{}); err != nil {
		t.Fatal(err)
	}
	return db
}
