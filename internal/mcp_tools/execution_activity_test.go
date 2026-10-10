package mcp_tools

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/activity"
)

type activityToolArgs struct {
	Name string `json:"name" validate:"required"`
}

type activityTestTool struct {
	run func() (*ToolResult, error)
}

func (activityTestTool) Name() string                { return "activity_test" }
func (activityTestTool) Description() string         { return "test only" }
func (activityTestTool) Category() ToolCategory      { return CategoryWritingAssistant }
func (activityTestTool) JSONSchema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (activityTestTool) ExposeToLLM() bool           { return false }
func (activityTestTool) NewArgs() any                { return &activityToolArgs{} }
func (t activityTestTool) Execute(context.Context, any, ToolContext) (*ToolResult, error) {
	return t.run()
}

func setupExecutionActivityTest(t *testing.T) (*Registry, ToolContext) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "activity.db")), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&activity.DailyActivity{}))
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewRegistry(logger), ToolContext{DB: db, Logger: logger}
}

func requireToolActivity(t *testing.T, db *gorm.DB, want int64) {
	t.Helper()
	var rows []activity.DailyActivity
	require.NoError(t, db.Find(&rows).Error)
	var calls, turns int64
	for _, row := range rows {
		calls += row.ToolCalls
		turns += row.ConversationTurns
	}
	require.Equal(t, want, calls)
	require.Zero(t, turns, "工具执行不增加用户对话轮数")
}

func TestRegistryActivityCountsExecutionAttempts(t *testing.T) {
	reg, tc := setupExecutionActivityTest(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for i, outcome := range []string{"success", "business", "system", "panic", "cancel"} {
		reg.Register(activityTestTool{run: func() (*ToolResult, error) {
			requireToolActivity(t, tc.DB, int64(i+1))
			switch outcome {
			case "business":
				return &ToolResult{Success: false, Error: "拒绝操作"}, nil
			case "system":
				return nil, errors.New("execution failed")
			case "panic":
				panic("execution panicked")
			case "cancel":
				cancel()
				return nil, ctx.Err()
			default:
				return &ToolResult{Success: true}, nil
			}
		}})
		result := reg.Execute(ctx, "activity_test", json.RawMessage(`{"name":"test"}`), tc, nil)
		require.Equal(t, outcome == "success", result.Success)
	}
	requireToolActivity(t, tc.DB, 5)
}

func TestRegistryActivitySkipsRejectedCalls(t *testing.T) {
	reg, tc := setupExecutionActivityTest(t)
	reg.Register(activityTestTool{run: func() (*ToolResult, error) {
		t.Fatal("被拦截的工具不应执行")
		return nil, nil
	}})
	for _, tt := range []struct {
		name, args string
		allowed    map[string]bool
	}{
		{"missing", `{}`, nil},
		{"activity_test", `{"name":"test"}`, map[string]bool{}},
		{"activity_test", `{`, nil},
		{"activity_test", `{}`, nil},
	} {
		result := reg.Execute(context.Background(), tt.name, json.RawMessage(tt.args), tc, tt.allowed)
		require.False(t, result.Success)
	}
	requireToolActivity(t, tc.DB, 0)
}

func TestRegistryActivityFailureDoesNotBlockExecution(t *testing.T) {
	reg, tc := setupExecutionActivityTest(t)
	require.NoError(t, tc.DB.Exec(`CREATE TRIGGER reject_activity BEFORE INSERT ON activity_daily
		BEGIN SELECT RAISE(ABORT, 'statistics unavailable'); END`).Error)
	called := false
	reg.Register(activityTestTool{run: func() (*ToolResult, error) {
		called = true
		return &ToolResult{Success: true}, nil
	}})
	result := reg.Execute(context.Background(), "activity_test", json.RawMessage(`{"name":"test"}`), tc, nil)
	require.True(t, called)
	require.True(t, result.Success)
	requireToolActivity(t, tc.DB, 0)
}
