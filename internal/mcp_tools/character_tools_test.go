package mcp_tools_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/sigpanic/goink/internal/mcp_tools"
)

// newTestRegistry 构造一个注册了全部工具的 Registry，logger 丢弃输出。
func newTestRegistry(t *testing.T) *mcp_tools.Registry {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	reg := mcp_tools.NewRegistry(logger)
	mcp_tools.RegisterAllTools(reg)
	return reg
}

// execTool 调用 Registry.Execute。ToolContext 零值——本文件所有用例都在
// Unmarshal 阶段（DisallowUnknownFields）或入口 switch 阶段返回，不碰 DB。
func execTool(t *testing.T, reg *mcp_tools.Registry, name string, rawArgs any) *mcp_tools.ToolResult {
	t.Helper()
	b, err := json.Marshal(rawArgs)
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}
	return reg.Execute(context.Background(), name, b, mcp_tools.ToolContext{}, nil)
}

// ── 框架层 DisallowUnknownFields ──────────────────────────

// 验证 base.go 的 DisallowUnknownFields 拒绝 schema 外字段（id/novel_id/is_current
// 等 entity 不可变字段），防止 LLM hallucination 传额外字段经第二次 Unmarshal 覆盖 entity。
func TestDisallowUnknownFields_RejectsEntityImmutableFields(t *testing.T) {
	reg := newTestRegistry(t)
	// UpdateCharacterRelationshipArgs 不含 id/novel_id/is_current
	args := map[string]any{
		"relation_id":       1,
		"id":                999,
		"novel_id":          7,
		"is_current":        true,
		"relation_describe": "师徒",
	}
	res := execTool(t, reg, "update_character_relationship", args)
	if res.Success {
		t.Error("expected failure on unknown fields id/novel_id/is_current")
	}
	if !strings.Contains(res.Error, "参数格式不正确") {
		t.Errorf("expected 参数格式不正确, got: %s", res.Error)
	}
}

// 验证 DisallowUnknownFields 是框架层生效，对 update_character 同样有效。
func TestDisallowUnknownFields_FrameworkLevelAcrossTools(t *testing.T) {
	reg := newTestRegistry(t)
	// UpdateCharacterArgs 不含 id（entity 主键），LLM 试图传 id 覆盖
	args := map[string]any{
		"character_id": 1,
		"id":           999,
		"name":         "新名",
	}
	res := execTool(t, reg, "update_character", args)
	if res.Success {
		t.Error("expected failure on unknown field 'id'")
	}
	if !strings.Contains(res.Error, "参数格式不正确") {
		t.Errorf("expected 参数格式不正确, got: %s", res.Error)
	}
}

// ── 入口字段存在性校验（替换 ==0 分支）──────────────────────

// 验证 schema 内字段通过 DisallowUnknownFields，但互斥组合在入口校验被拒。
// 此用例不碰 DB（非法组合在 switch default 返回）。
func TestUpdateCharacterRelationship_AcceptsSchemaFieldsButRejectsMutualExclusion(t *testing.T) {
	reg := newTestRegistry(t)
	// relation_id + source_character_id 都是 schema 内字段，通过 DisallowUnknownFields
	// 但两者互斥，入口校验返回"参数组合非法"
	args := map[string]any{
		"relation_id":         1,
		"source_character_id": 2,
	}
	res := execTool(t, reg, "update_character_relationship", args)
	if res.Success {
		t.Error("expected failure on mutually exclusive fields")
	}
	if strings.Contains(res.Error, "参数格式不正确") {
		t.Errorf("schema fields should pass DisallowUnknownFields, got: %s", res.Error)
	}
	if !strings.Contains(res.Error, "参数组合非法") {
		t.Errorf("expected 参数组合非法, got: %s", res.Error)
	}
}

// 验证各种非法组合都被入口校验拦截（不碰 DB）。
func TestUpdateCharacterRelationship_IllegalCombinations(t *testing.T) {
	reg := newTestRegistry(t)
	cases := []struct {
		name string
		args map[string]any
	}{
		{"all_three", map[string]any{"relation_id": 1, "source_character_id": 2, "target_character_id": 3}},
		{"relation_id_plus_source", map[string]any{"relation_id": 1, "source_character_id": 2}},
		{"relation_id_plus_target", map[string]any{"relation_id": 1, "target_character_id": 3}},
		{"source_only", map[string]any{"source_character_id": 2}},
		{"target_only", map[string]any{"target_character_id": 3}},
		{"none", map[string]any{"relation_describe": "师徒"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res := execTool(t, reg, "update_character_relationship", c.args)
			if res.Success {
				t.Error("expected failure on illegal combination")
			}
			if !strings.Contains(res.Error, "参数组合非法") {
				t.Errorf("expected 参数组合非法, got: %s", res.Error)
			}
		})
	}
}

// ── 零值陷阱：4.2.2 核心修复点 ────────────────────────────

// 验证"从原始参数校验字段存在性"堵死了 ==0 陷阱。
// 旧逻辑（a.SourceCharacterID == 0）：LLM 显式传 source_character_id:0 被判为"未传"，
//
//	进入 editRelation → json.Unmarshal(tc.RawArgs, &rel) 把 rel.SourceCharacterID 覆盖为 0。
//
// 新逻辑（map 字段存在性）：hasSource=true → hasRelationID && hasSource → 非法组合，
//
//	RawArgs 进不了 editRelation 的 Unmarshal，source/target 不会被覆盖。
func TestUpdateCharacterRelationship_ZeroValueTrapBlocked(t *testing.T) {
	reg := newTestRegistry(t)
	// LLM 显式传 source_character_id:0 + relation_id:1
	args := map[string]any{
		"relation_id":         1,
		"source_character_id": 0, // 显式 0，hasSource=true
	}
	res := execTool(t, reg, "update_character_relationship", args)
	if res.Success {
		t.Error("expected failure: relation_id + explicit source_character_id:0 is illegal")
	}
	if !strings.Contains(res.Error, "参数组合非法") {
		t.Errorf("explicit zero should be detected as field present, got: %s", res.Error)
	}
	// 关键：不能进 editRelation（否则 Unmarshal 会把 source 覆盖为 0）
	if strings.Contains(res.Error, "不存在") {
		t.Errorf("should not reach editRelation DB query, got: %s", res.Error)
	}
}

// 验证 target_character_id:0 同样被识别为字段存在。
func TestUpdateCharacterRelationship_ZeroValueTrapBlockedTarget(t *testing.T) {
	reg := newTestRegistry(t)
	args := map[string]any{
		"relation_id":         1,
		"target_character_id": 0, // 显式 0，hasTarget=true
	}
	res := execTool(t, reg, "update_character_relationship", args)
	if res.Success {
		t.Error("expected failure: relation_id + explicit target_character_id:0 is illegal")
	}
	if !strings.Contains(res.Error, "参数组合非法") {
		t.Errorf("explicit zero target should be detected as field present, got: %s", res.Error)
	}
}
