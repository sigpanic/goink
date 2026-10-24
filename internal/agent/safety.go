package agent

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/sigpanic/goink/internal/llm"
	"github.com/sigpanic/goink/internal/mcp_tools"
)

// 只读工具集合，死循环检测用。
var readOnlyTools = map[string]bool{
	"search_story_memory":     true,
	"get_timeline":            true,
	"get_chapter_list":        true,
	"get_characters":          true,
	"get_locations":           true,
	"get_story_arcs":          true,
	"get_reader_perspective":  true,
	"get_character_relations": true,
	"read":                    true,
}

type toolOutput struct {
	name         string
	id           string
	rawArgs      json.RawMessage
	result       *mcp_tools.ToolResult
	displayText  string // buildDisplay 生成的展示文本
	activityKind string // buildDisplay 生成的活动类别
}

func (to toolOutput) resultJSON() string {
	payload := map[string]any{}
	if to.result != nil {
		payload["success"] = to.result.Success
		if to.result.Error != "" {
			payload["error"] = to.result.Error
		}
		if to.result.Data != nil {
			payload["data"] = to.result.Data
		}
	}
	b, _ := json.Marshal(payload)
	return string(b)
}

// buildToolCalls 将本轮 toolOutputs 转换为公共工具调用，参数保留原始 JSON。
func buildToolCalls(outputs []toolOutput) []llm.ToolCall {
	var calls []llm.ToolCall
	for _, o := range outputs {
		calls = append(calls, llm.ToolCall{
			ID:        o.id,
			Name:      o.name,
			Arguments: o.rawArgs,
		})
	}
	return calls
}

// isStuckLoop 检测是否陷入死循环：最近 4 轮 ≤2 种调用模式 + 全是只读工具 + turn≥4。
func isStuckLoop(patterns []string, outputs []toolOutput, loopCount int) bool {
	if len(patterns) < 4 || loopCount < 4 {
		return false
	}
	recent := patterns[len(patterns)-4:]
	uniq := make(map[string]bool, len(recent))
	for _, p := range recent {
		uniq[p] = true
	}
	if len(uniq) > 2 {
		return false
	}
	for _, o := range outputs {
		if !readOnlyTools[o.name] {
			return false
		}
	}
	return true
}

// toolPattern 为当前轮的工具调用生成模式串，供死循环检测用。
// 按工具名排序后拼接，确保多工具调用轮次可与 Python 行为一致地比较。
func toolPattern(outputs []toolOutput) string {
	parts := make([]string, len(outputs))
	for i, o := range outputs {
		parts[i] = o.name + ":" + string(o.rawArgs)
		if len(parts[i]) > 100 {
			parts[i] = parts[i][:100]
		}
	}
	sort.Strings(parts)
	return strings.Join(parts, "|")
}
