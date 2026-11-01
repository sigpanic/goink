package anthropic

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	sdk "github.com/anthropics/anthropic-sdk-go"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

func (p Provider) buildMessages(params *sdk.MessageNewParams, messages []adapter.Message) error {
	for i, message := range messages {
		if err := message.ValidateImages(); err != nil {
			return fmt.Errorf("message %d: %w", i, err)
		}
	}
	start := 0
	for start < len(messages) && messages[start].Role == "system" {
		if len(messages[start].ToolCalls) > 0 {
			return fmt.Errorf("message %d: tool calls require assistant role", start)
		}
		if messages[start].Content != "" {
			params.System = append(params.System, sdk.TextBlockParam{Text: messages[start].Content})
		}
		start++
	}
	used := make(map[string]bool)
	for i := start; i < len(messages); i++ {
		message := messages[i]
		switch message.Role {
		case "system", "user":
			if len(message.ToolCalls) > 0 {
				return fmt.Errorf("message %d: tool calls require assistant role", i)
			}
			if message.Content == "" && len(message.ImageInputs) == 0 {
				continue
			}
			content := message.Content
			if message.Role == "system" {
				content = "<system-reminder>\n" + content + "\n</system-reminder>"
			}
			var blocks []sdk.ContentBlockParamUnion
			if content != "" {
				blocks = append(blocks, sdk.NewTextBlock(content))
			}
			for _, image := range message.ImageInputs {
				blocks = append(blocks, sdk.NewImageBlockBase64(image.MIMEType, image.Base64))
			}
			appendUserMessage(params, blocks...)
		case "assistant":
			calls, results, end, err := pairedTools(messages, i)
			if err != nil {
				return fmt.Errorf("message %d: %w", i, err)
			}
			var blocks []sdk.ContentBlockParamUnion
			if message.Content != "" {
				blocks = append(blocks, sdk.NewTextBlock(message.Content))
			}
			for _, call := range calls {
				if used[call.ID] {
					return fmt.Errorf("message %d: reused tool call id %q", i, call.ID)
				}
				used[call.ID] = true
				if call.Name == "" || !jsonObject(call.Arguments) {
					return fmt.Errorf("message %d: tool %q requires a name and object arguments", i, call.ID)
				}
				blocks = append(blocks, sdk.NewToolUseBlock(call.ID, slices.Clone(call.Arguments), call.Name))
			}
			message.ToolCalls = calls
			native, replay, err := p.replayContent(*params, message)
			if err != nil {
				return fmt.Errorf("message %d: %w", i, err)
			}
			if replay {
				blocks = native
			}
			if len(blocks) == 0 {
				i = end
				continue
			}
			params.Messages = append(params.Messages, sdk.NewAssistantMessage(blocks...))
			for _, result := range results {
				// 公共消息尚无执行错误标志；不从结果正文猜测 is_error。
				block := sdk.NewToolResultBlock(result.ToolCallID, result.Content, false)
				if result.Content == "" {
					block.OfToolResult.Content = nil
				}
				for _, image := range result.ImageInputs {
					imageBlock := sdk.NewImageBlockBase64(image.MIMEType, image.Base64)
					block.OfToolResult.Content = append(block.OfToolResult.Content, sdk.ToolResultBlockParamContentUnion{OfImage: imageBlock.OfImage})
				}
				appendUserMessage(params, block)
			}
			i = end
		case "tool":
			// 已配对结果在 assistant 分支中消费，此处剩余结果均为孤立记录。
			continue
		default:
			return fmt.Errorf("message %d: unsupported role %q", i, message.Role)
		}
	}
	return nil
}

func appendUserMessage(params *sdk.MessageNewParams, blocks ...sdk.ContentBlockParamUnion) {
	if n := len(params.Messages); n > 0 && params.Messages[n-1].Role == sdk.MessageParamRoleUser {
		params.Messages[n-1].Content = append(params.Messages[n-1].Content, blocks...)
		return
	}
	params.Messages = append(params.Messages, sdk.NewUserMessage(blocks...))
}

// pairedTools 只保留有紧邻结果的调用，不跨 user/system/assistant 边界寻找结果。
// 同组重复声明及结果保留首项；冲突声明报错，缺失结果不伪造。
func pairedTools(messages []adapter.Message, index int) ([]adapter.ToolCall, []adapter.Message, int, error) {
	end := index
	byID := make(map[string]adapter.Message)
	for end+1 < len(messages) && messages[end+1].Role == "tool" {
		end++
		result := messages[end]
		if _, exists := byID[result.ToolCallID]; !exists {
			byID[result.ToolCallID] = result
		}
	}
	var calls []adapter.ToolCall
	var results []adapter.Message
	seen := make(map[string]adapter.ToolCall)
	for _, call := range messages[index].ToolCalls {
		if call.ID == "" {
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
	return json.Valid(data) && strings.HasPrefix(strings.TrimSpace(string(data)), "{")
}

func sameJSON(a, b json.RawMessage) bool {
	// RawMessage 保留数值精度；比较规范化的对象键，不经 float64 解码。
	var left, right map[string]json.RawMessage
	if !jsonObject(a) || !jsonObject(b) || json.Unmarshal(a, &left) != nil || json.Unmarshal(b, &right) != nil {
		return false
	}
	x, _ := json.Marshal(left)
	y, _ := json.Marshal(right)
	return string(x) == string(y)
}
