package agent

import (
	"github.com/sigpanic/goink/internal/llm"
	"github.com/sigpanic/goink/internal/mcp_tools"
)

// flushInterruptedTools 排干 stream 中待执行的 tool_call_end 事件，
// 不真正执行工具，直接标记为"操作被中断"并入 toolOutputs。
func (a *Agent) flushInterruptedTools(stream <-chan llm.StreamEvent, opts *RunOptions, toolOutputs *[]toolOutput) {
	for {
		done := false
		select {
		case event, ok := <-stream:
			if !ok {
				done = true
			} else if event.Type == llm.EventToolCallEnd {
				name := event.Delta.ToolName
				id := event.Delta.ToolID
				rawArgs := event.Delta.ArgumentsJSON
				args := parseArgs(rawArgs)
				display := a.buildDisplay(name, args, mcp_tools.PhaseFailed, opts.NovelID)
				*toolOutputs = append(*toolOutputs, toolOutput{
					name:         name,
					id:           id,
					rawArgs:      rawArgs,
					result:       &mcp_tools.ToolResult{Success: false, Error: "操作被中断"},
					displayText:  display.DisplayText,
					activityKind: display.ActivityKind,
				})
			}
		default:
			done = true
		}
		if done {
			return
		}
	}
}
