package agent

import (
	"fmt"

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

// interruptTracker 统计工具连续失败次数，决定是否中断对话。
//
// 三级计数：
//   - systemFailCnt: per-tool system 错误（DB/网络/panic），5 次中断
//   - argsFailCnt:   per-tool args 错误（参数格式/校验），5 次中断
//   - globalStreak:  全局连续失败（任意工具/任意原因，含业务错误），10 次兜底中断
//
// 成功重置对应计数；system/args 错误交叉重置（不同失败模式互不累加）。
type interruptTracker struct {
	systemFailCnt map[string]int
	argsFailCnt   map[string]int
	globalStreak  int
}

func newInterruptTracker() *interruptTracker {
	return &interruptTracker{
		systemFailCnt: make(map[string]int),
		argsFailCnt:   make(map[string]int),
	}
}

// recordFailure 记录一次工具失败，返回是否应中断对话及中断原因。
// 优先级：per-tool（system/args）先于 global，单工具先触发更精确。
func (t *interruptTracker) recordFailure(name string, kind mcp_tools.ErrKind) (bool, error) {
	switch kind {
	case mcp_tools.ErrKindSystem:
		t.systemFailCnt[name]++
		t.argsFailCnt[name] = 0
	case mcp_tools.ErrKindArgs:
		t.argsFailCnt[name]++
		t.systemFailCnt[name] = 0
	default:
		// 业务错误：不计入 per-tool 计数，但计入全局连续失败
	}
	t.globalStreak++

	if t.systemFailCnt[name] >= 5 {
		return true, fmt.Errorf("工具 %s 连续系统错误 %d 次，已中断对话", name, t.systemFailCnt[name])
	}
	if t.argsFailCnt[name] >= 5 {
		return true, fmt.Errorf("工具 %s 连续参数错误 %d 次，已中断对话", name, t.argsFailCnt[name])
	}
	if t.globalStreak >= 10 {
		return true, fmt.Errorf("连续失败 %d 次，已中断对话", t.globalStreak)
	}
	return false, nil
}

// recordSuccess 记录一次工具成功，重置该工具的 per-tool 计数与全局连续失败。
func (t *interruptTracker) recordSuccess(name string) {
	t.systemFailCnt[name] = 0
	t.argsFailCnt[name] = 0
	t.globalStreak = 0
}
