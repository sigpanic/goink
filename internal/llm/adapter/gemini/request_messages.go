package gemini

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	types "google.golang.org/genai/interactions/models/interactions"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

func (p Provider) buildMessages(params *types.CreateModelInteraction, messages []adapter.Message) error {
	start := 0
	var system []string
	for start < len(messages) && messages[start].Role == "system" {
		if len(messages[start].ToolCalls) > 0 {
			return fmt.Errorf("message %d: tool calls require assistant role", start)
		}
		if messages[start].Content != "" {
			system = append(system, messages[start].Content)
		}
		start++
	}
	if len(system) > 0 {
		instruction := strings.Join(system, "\n\n")
		params.SystemInstruction = &instruction
	}
	input := types.NewInteractionsInput([]types.Step{})
	params.Input = &input
	used := make(map[string]bool)
	for i := start; i < len(messages); i++ {
		message := messages[i]
		if message.Role != "assistant" && len(message.ToolCalls) > 0 {
			return fmt.Errorf("message %d: tool calls require assistant role", i)
		}
		switch message.Role {
		case "system", "user":
			if message.Content == "" {
				continue
			}
			content := message.Content
			if message.Role == "system" {
				content = "<system-reminder>\n" + content + "\n</system-reminder>"
			}
			input.ArrayOfStep = append(input.ArrayOfStep, types.NewStep(types.UserInputStep{Content: textContent(content)}))
		case "assistant":
			calls, results, end, err := pairedTools(messages, i)
			if err != nil {
				return fmt.Errorf("message %d: %w", i, err)
			}
			for _, call := range calls {
				if used[call.ID] {
					return fmt.Errorf("message %d: reused tool call id %q", i, call.ID)
				}
				used[call.ID] = true
				if strings.TrimSpace(call.Name) == "" || !jsonObject(call.Arguments) {
					return fmt.Errorf("message %d: tool %q requires a name and object arguments", i, call.ID)
				}
			}
			message.ToolCalls = calls
			native, replay, err := p.replaySteps(*params, message)
			if err != nil {
				return fmt.Errorf("message %d: %w", i, err)
			}
			if replay {
				input.ArrayOfStep = append(input.ArrayOfStep, native...)
			} else {
				if message.Content != "" {
					input.ArrayOfStep = append(input.ArrayOfStep, types.NewStep(types.ModelOutputStep{Content: textContent(message.Content)}))
				}
				for _, call := range calls {
					// SDK FunctionCallStep.Arguments 是 map[string]any；原始 JSON 避免大整数失真。
					raw, err := json.Marshal(functionCall{Type: "function_call", ID: call.ID, Name: call.Name, Arguments: call.Arguments})
					if err != nil {
						return fmt.Errorf("encode Gemini tool call %q: %w", call.ID, err)
					}
					input.ArrayOfStep = append(input.ArrayOfStep, types.NewStepUnknown(raw))
				}
			}
			for j, result := range results {
				name := calls[j].Name
				// 公共消息没有执行错误标志，不从正文猜测 is_error；空结果也保留。
				input.ArrayOfStep = append(input.ArrayOfStep, types.NewStep(types.FunctionResultStep{
					CallID: result.ToolCallID, Name: &name,
					Result: types.NewFunctionResultStepResultUnion([]types.FunctionResultSubcontent{
						types.NewFunctionResultSubcontent(types.TextContent{Text: result.Content}),
					}),
				}))
			}
			i = end
		case "tool":
			// 配对结果已由 assistant 消费；孤立记录不能单独发往目标协议。
			continue
		default:
			return fmt.Errorf("message %d: unsupported role %q", i, message.Role)
		}
	}
	return nil
}

func textContent(text string) []types.Content {
	return []types.Content{types.NewContent(types.TextContent{Text: text})}
}

type functionCall struct {
	Type      string          `json:"type"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// pairedTools 只在紧邻的 tool 记录中配对，按调用顺序发回结果。
// 完全重复保留首项，冲突报错；孤立结果和缺失结果的调用丢弃，不伪造完成。
func pairedTools(messages []adapter.Message, index int) ([]adapter.ToolCall, []adapter.Message, int, error) {
	end := index
	byID := make(map[string]adapter.Message)
	for end+1 < len(messages) && messages[end+1].Role == "tool" {
		end++
		result := messages[end]
		if len(result.ToolCalls) > 0 {
			return nil, nil, end, fmt.Errorf("tool calls require assistant role")
		}
		if previous, exists := byID[result.ToolCallID]; exists {
			if previous.Content != result.Content || previous.ToolName != result.ToolName {
				return nil, nil, end, fmt.Errorf("conflicting tool result id %q", result.ToolCallID)
			}
			continue
		}
		byID[result.ToolCallID] = result
	}
	var calls []adapter.ToolCall
	var results []adapter.Message
	seen := make(map[string]adapter.ToolCall)
	for _, call := range messages[index].ToolCalls {
		if strings.TrimSpace(call.ID) == "" {
			return nil, nil, end, fmt.Errorf("tool call id must not be empty")
		}
		if previous, exists := seen[call.ID]; exists {
			if previous.Name != call.Name || !sameJSON(previous.Arguments, call.Arguments) {
				return nil, nil, end, fmt.Errorf("conflicting tool call id %q", call.ID)
			}
			continue
		}
		seen[call.ID] = call
		result, paired := byID[call.ID]
		if !paired {
			continue
		}
		if result.ToolName != "" && result.ToolName != call.Name {
			return nil, nil, end, fmt.Errorf("tool result name does not match call %q", call.ID)
		}
		calls = append(calls, call)
		results = append(results, result)
	}
	return calls, results, end, nil
}

func jsonObject(data json.RawMessage) bool {
	return json.Valid(data) && bytes.HasPrefix(bytes.TrimSpace(data), []byte("{"))
}

func sameJSON(a, b json.RawMessage) bool {
	if !jsonObject(a) || !jsonObject(b) {
		return false
	}
	// UseNumber 保留数值精度，也允许对象字段顺序不同，包括嵌套对象。
	var left, right any
	x, y := json.NewDecoder(bytes.NewReader(a)), json.NewDecoder(bytes.NewReader(b))
	x.UseNumber()
	y.UseNumber()
	if x.Decode(&left) != nil || y.Decode(&right) != nil {
		return false
	}
	return reflect.DeepEqual(left, right)
}
