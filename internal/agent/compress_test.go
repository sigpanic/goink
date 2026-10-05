package agent

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/agentcfg"
	"github.com/sigpanic/goink/internal/session"
)

// newCompressTestAgent 建一个只迁移了 session/message 表的临时文件库，并写入一条 session 行。
// 用临时文件而非 ":memory:"：内存库在连接池下每个连接各持一份，容易在多次查询间"丢表"。
func newCompressTestAgent(t *testing.T, sessionID string) *Agent {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "compress.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	// 显式关闭连接：Windows 无法删除仍被占用的 db 文件，不关会让 t.TempDir() 清理失败。
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB(): %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&session.Session{}, &session.Message{}); err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	if err := db.Create(&session.Session{SessionID: sessionID, NovelID: 1, ActiveVersion: 1}).Error; err != nil {
		t.Fatalf("create session: %v", err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return &Agent{db: db, session: session.NewStore(db, logger), logger: logger}
}

// renderForAPI 复刻 loadAPIMessages 的渲染步骤：逐条 ToAPIFormat。
func renderForAPI(msgs []session.Message, logger *slog.Logger) []map[string]any {
	out := make([]map[string]any, 0, len(msgs))
	for i := range msgs {
		out = append(out, msgs[i].ToAPIFormat(logger))
	}
	return out
}

// assertUserContent 重载指定 version 的消息并渲染，断言含"你好"的用户消息恰好一条、
// 且内容等于 want（即只有单层时间前缀）。
func assertUserContent(t *testing.T, a *Agent, ctx context.Context, sessionID string, version int, want string) {
	t.Helper()
	msgs, err := a.session.GetMessagesForAPI(ctx, sessionID, version)
	if err != nil {
		t.Fatalf("reload v%d: %v", version, err)
	}
	var matched, allUser []string
	for i := range msgs {
		api := msgs[i].ToAPIFormat(a.logger)
		role, _ := api["role"].(string)
		content, _ := api["content"].(string)
		if role != "user" {
			continue
		}
		allUser = append(allUser, content)
		if strings.Contains(content, "你好") {
			matched = append(matched, content)
		}
	}
	if len(matched) != 1 {
		t.Fatalf("v%d: 期望恰好 1 条含\"你好\"的用户消息，实际 %d 条；全部 user content=%q",
			version, len(matched), allUser)
	}
	if matched[0] != want {
		t.Errorf("v%d: 用户消息 = %q, want %q（时间前缀被重复叠加）", version, matched[0], want)
	}
}

// TestPersistCompression_DoesNotDoublePrefixUserTimestamp 覆盖"压缩→持久化→重载→渲染"全链路：
// 用户消息的发送时间前缀只能有一层，反复压缩不得逐次叠加。
//
// 不接 LLM：Compress 中唯一用到 LLM 的是 generateSummary，而前缀叠加发生在它之后的
// persistCompression（纯 DB 操作），因此这里直接调用 persistCompression 并注入固定 summary，
// 用 retainMessages 复用生产保留逻辑，等价于真实压缩路径。
func TestPersistCompression_DoesNotDoublePrefixUserTimestamp(t *testing.T) {
	const sessionID = "sess_compress_prefix"
	a := newCompressTestAgent(t, sessionID)
	ctx := context.Background()

	// v1：显式递增 CreatedAt，保证渲染顺序与 DB 的 created_at ASC 排序一致。
	base := time.Date(2020, 1, 2, 3, 4, 0, 0, time.Local)
	v1 := []session.Message{
		{Role: "system", Content: "你是创作助手", CreatedAt: base, Version: 1, ToAPI: true, AgentType: "main"},
		{Role: "user", Content: "你好", CreatedAt: base.Add(time.Minute), Version: 1, ToAPI: true, ToFrontend: true, AgentType: "main"},
		{Role: "assistant", Content: "你好，有什么可以帮你", CreatedAt: base.Add(2 * time.Minute), Version: 1, ToAPI: true, AgentType: "main"},
	}
	for i := range v1 {
		if err := a.db.Create(&v1[i]).Error; err != nil {
			t.Fatalf("create v1 message[%d]: %v", i, err)
		}
	}

	// 用户消息的真实发送时间应原样保留在前缀里（分钟精度）。
	want := "[2020-01-02 03:05] 你好"

	opts := &RunOptions{SessionID: sessionID, TurnID: 2, ActiveVersion: 1, AgentType: "main"}

	// 第一次压缩
	version, err := a.persistCompression(ctx, opts, &agentcfg.SystemMessages{Identity: "sys-identity"},
		"第一次摘要", retainMessages(renderForAPI(v1, a.logger)))
	if err != nil {
		t.Fatalf("persistCompression #1: %v", err)
	}
	assertUserContent(t, a, ctx, sessionID, version, want)

	// 第二次压缩：模拟长会话反复压缩，验证前缀不叠加。
	opts.ActiveVersion = version
	reloaded, err := a.session.GetMessagesForAPI(ctx, sessionID, version)
	if err != nil {
		t.Fatalf("reload v%d: %v", version, err)
	}
	version2, err := a.persistCompression(ctx, opts, &agentcfg.SystemMessages{Identity: "sys-identity"},
		"第二次摘要", retainMessages(renderForAPI(reloaded, a.logger)))
	if err != nil {
		t.Fatalf("persistCompression #2: %v", err)
	}
	assertUserContent(t, a, ctx, sessionID, version2, want)
}
