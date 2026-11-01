package openairesponses

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

func (p Provider) buildMessages(params *responses.ResponseNewParams, messages []adapter.Message) error {
	for i, message := range messages {
		if err := message.ValidateImages(); err != nil {
			return fmt.Errorf("message %d: %w", i, err)
		}
	}
	used := make(map[string]bool)
	for i := 0; i < len(messages); i++ {
		message := messages[i]
		if message.Role != "assistant" && len(message.ToolCalls) > 0 {
			return fmt.Errorf("message %d: tool calls require assistant role", i)
		}
		switch message.Role {
		case "system", "developer", "user":
			if len(message.ImageInputs) > 0 {
				var content responses.ResponseInputMessageContentListParam
				if message.Content != "" {
					content = append(content, responses.ResponseInputContentParamOfInputText(message.Content))
				}
				for _, image := range message.ImageInputs {
					part := responses.ResponseInputContentParamOfInputImage(responses.ResponseInputImageDetailAuto)
					part.OfInputImage.ImageURL = param.NewOpt(image.DataURL())
					content = append(content, part)
				}
				params.Input.OfInputItemList = append(params.Input.OfInputItemList,
					responses.ResponseInputItemParamOfMessage(content, responses.EasyInputMessageRole(message.Role)))
				continue
			}
			// Responses 可直接表达这些角色，保留中途 system 的原始位置。
			if message.Content != "" {
				params.Input.OfInputItemList = append(params.Input.OfInputItemList,
					responses.ResponseInputItemParamOfMessage(message.Content, responses.EasyInputMessageRole(message.Role)))
			}
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
			native, replay, err := p.replayOutput(*params, message)
			if err != nil {
				return fmt.Errorf("message %d: %w", i, err)
			}
			if replay {
				params.Input.OfInputItemList = append(params.Input.OfInputItemList, native...)
			} else {
				if message.Content != "" {
					params.Input.OfInputItemList = append(params.Input.OfInputItemList,
						responses.ResponseInputItemParamOfMessage(message.Content, responses.EasyInputMessageRoleAssistant))
				}
				for _, call := range calls {
					params.Input.OfInputItemList = append(params.Input.OfInputItemList,
						responses.ResponseInputItemParamOfFunctionCall(string(call.Arguments), call.ID, call.Name))
				}
			}
			for _, result := range results {
				item := responses.ResponseInputItemParamOfFunctionCallOutput(result.Content)
				if len(result.ImageInputs) > 0 {
					var output responses.ResponseFunctionCallOutputItemListParam
					if result.Content != "" {
						output = append(output, responses.ResponseFunctionCallOutputItemParamOfInputText(result.Content))
					}
					for _, image := range result.ImageInputs {
						output = append(output, responses.ResponseFunctionCallOutputItemUnionParam{
							OfInputImage: &responses.ResponseInputImageContentParam{ImageURL: param.NewOpt(image.DataURL()), Detail: "auto"},
						})
					}
					item = responses.ResponseInputItemParamOfFunctionCallOutput(output)
				}
				item.OfFunctionCallOutput.CallID = param.NewOpt(result.ToolCallID)
				params.Input.OfInputItemList = append(params.Input.OfInputItemList, item)
			}
			i = end
		case "tool":
			// 已配对结果由 assistant 消费；其余是孤立记录。
			continue
		default:
			return fmt.Errorf("message %d: unsupported role %q", i, message.Role)
		}
	}
	return nil
}

// pairedTools 只配对紧邻的 tool 记录，按调用顺序输出结果，不跨越其他消息。
// 完全重复保留首项，冲突报错；孤立结果和缺失结果的调用不出站。
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
	// UseNumber 避免浮点数舍入，比较时忽略嵌套对象的键顺序。
	var left, right any
	x, y := json.NewDecoder(bytes.NewReader(a)), json.NewDecoder(bytes.NewReader(b))
	x.UseNumber()
	y.UseNumber()
	return x.Decode(&left) == nil && y.Decode(&right) == nil && reflect.DeepEqual(left, right)
}
