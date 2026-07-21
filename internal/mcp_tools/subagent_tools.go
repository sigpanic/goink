package mcp_tools

import (
	"context"
	"encoding/json"
)

// ── run_subagent ─────────────────────────────────────────

// RunSubagentArgs 是 run_subagent 的参数。
type RunSubagentArgs struct {
	AgentType   string `json:"agent_type" jsonschema:"required,enum=memory,enum=review,description=子 Agent 类型。memory：记忆检索分析员，探索故事数据并整理报告；review：章节审稿人，全面质量审查" validate:"required,oneof=memory review"`
	Instruction string `json:"instruction" jsonschema:"required,description=给子 Agent 的任务指令，描述需要完成的具体工作" validate:"required"`
}

// RunSubagentTool 启动子 Agent 执行专项任务，返回最终报告。
type RunSubagentTool struct{}

func (t *RunSubagentTool) Name() string           { return "run_subagent" }
func (t *RunSubagentTool) Description() string    { return runSubagentDescription }
func (t *RunSubagentTool) Category() ToolCategory { return CategoryWritingAssistant }

func (t *RunSubagentTool) JSONSchema() json.RawMessage { return SchemaOf(RunSubagentArgs{}) }
func (t *RunSubagentTool) ExposeToLLM() bool           { return true }
func (t *RunSubagentTool) NewArgs() any                { return &RunSubagentArgs{} }

func (t *RunSubagentTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*RunSubagentArgs)

	if tc.RunSubAgent == nil {
		return &ToolResult{
			Success: false,
			Error:   "子 Agent 运行器未配置",
		}, nil
	}

	report, err := tc.RunSubAgent(ctx, SubAgentRequest{
		AgentType:   a.AgentType,
		NovelID:     tc.NovelID,
		Instruction: a.Instruction,
		ToolID:      tc.ToolID,
	})
	if err != nil {
		return &ToolResult{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	return &ToolResult{
		Success: true,
		Data: map[string]any{
			"agent_type": a.AgentType,
			"report":     report,
		},
	}, nil
}

const runSubagentDescription = `启动专项子 Agent 执行任务，子 Agent 独立运行多轮思考后返回报告。

Agent 类型：
- memory：记忆检索分析员。可以搜索故事记忆、查询角色/时间线/弧线/地点/读者认知等所有数据，将分散的信息整合为连贯报告。用于需要多维度检索和深度分析的场景。
- review：章节审稿人。可以读取章节内容、查询角色设定/伏笔/弧线等参考数据，逐项检查角色一致性、情节逻辑、伏笔管理、读者认知和弧线推进，输出审稿意见。

instruction 应清晰描述任务目标和期望输出。`

// ── 注册 ──────────────────────────────────────────────────

// RegisterSubagentTools 注册子 Agent 工具。
func RegisterSubagentTools(r *Registry) {
	r.Register(&RunSubagentTool{})
}
