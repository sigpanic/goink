package engine

import (
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
