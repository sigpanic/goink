package openaichat

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

// buildMessages 构造本次请求独占的 OpenAI 消息，保留原始业务历史不变。
// 沿用历史加载的调用去重、孤立结果过滤策略，不在此新增缺失结果的修复规则。
// State 不作为出站字段；原生 payload 的兼容回放由后续协议状态单元接入。
func (c *Adapter) buildMessages(messages []adapter.Message) ([]map[string]any, error) {
	if messages == nil {
		return nil, nil
	}
	result := make([]map[string]any, 0, len(messages))
	validToolCallIDs := make(map[string]bool)
	seenToolMsgIDs := make(map[string]bool)
	var toolImages []adapter.ImageInput
	var toolImageSources []string
	flushToolImages := func() {
		if len(toolImages) > 0 {
			result = append(result, map[string]any{"role": "user", "content": imageContent(strings.Join(toolImageSources, "\n"), toolImages)})
			toolImages, toolImageSources = nil, nil
		}
	}
	for i, message := range messages {
		if message.Role != "tool" {
			flushToolImages()
		}
		if err := message.ValidateImages(); err != nil {
			return nil, fmt.Errorf("message %d: %w", i, err)
		}
		out := map[string]any{"role": message.Role, "content": message.Content}
		switch message.Role {
		case "user":
			if len(message.ImageInputs) > 0 {
				out["content"] = imageContent(message.Content, message.ImageInputs)
			}
		case "assistant":
			// 外部协议的显示用思考不冒充当前协议的推理输入。
			includeThinking := message.State == nil || message.State.Origin.AdapterName == adapter.ProtocolOpenAIChat
			if includeThinking && message.ThinkingContent != "" {
				out["reasoning_content"] = message.ThinkingContent
			}
			if len(message.ToolCalls) > 0 {
				calls := make([]map[string]any, 0, len(message.ToolCalls))
				seen := make(map[string]bool, len(message.ToolCalls))
				for _, call := range message.ToolCalls {
					if call.ID != "" {
						if seen[call.ID] {
							c.logger.Warn("duplicate tool_call_id in DB assistant tool_calls, skipping",
								"tool_call_id", call.ID)
							continue
						}
						seen[call.ID] = true
						validToolCallIDs[call.ID] = true
					}
					if !json.Valid(call.Arguments) {
						return nil, fmt.Errorf("message %d: invalid arguments JSON for tool %q", i, call.Name)
					}
					calls = append(calls, map[string]any{
						"id": call.ID, "type": "function",
						"function": map[string]any{"name": call.Name, "arguments": string(call.Arguments)},
					})
				}
				out["tool_calls"] = calls
				if includeThinking && message.ThinkingContent == "" {
					out["reasoning_content"] = ""
				}
			}
		case "tool":
			if message.ToolCallID != "" {
				if !validToolCallIDs[message.ToolCallID] {
					c.logger.Warn("orphan tool message without matching tool_call, skipping",
						"tool_call_id", message.ToolCallID)
					continue
				}
				if seenToolMsgIDs[message.ToolCallID] {
					c.logger.Warn("duplicate tool message in DB, skipping",
						"tool_call_id", message.ToolCallID)
					continue
				}
				seenToolMsgIDs[message.ToolCallID] = true
				out["tool_call_id"] = message.ToolCallID
			}
			if message.ToolName != "" {
				out["name"] = message.ToolName
			}
			if len(message.ImageInputs) > 0 {
				toolImages = append(toolImages, message.ImageInputs...)
				for _, ref := range message.Images {
					toolImageSources = append(toolImageSources, fmt.Sprintf("工具 %s 的图片结果，调用 ID：%s，路径：attachments/%s", message.ToolName, message.ToolCallID, ref.AttachmentID))
				}
			}
		}
		result = append(result, out)
	}
	flushToolImages()
	// 出站归一化：合并多条 system 为单条置于头部（参见 mergeSystemMessages）。
	return mergeSystemMessages(result), nil
}

func imageContent(text string, images []adapter.ImageInput) []map[string]any {
	var content []map[string]any
	if text != "" {
		content = append(content, map[string]any{"type": "text", "text": text})
	}
	for _, image := range images {
		content = append(content, map[string]any{"type": "image_url", "image_url": map[string]any{"url": image.DataURL()}})
	}
	return content
}

// mergeSystemMessages 将 messages 数组中所有 role=system 的消息合并成单条置于头部。
//
// 背景：OpenAI 协议允许多条 system 消息，但大量本地推理后端（llama.cpp 系列、
// 部分 ollama 后端等）只支持头部最多一条 system，多余会被静默丢弃或直接 400。
// OpenAI 官方社区确认：多条 system ≈ 合并成一条（推理效果几乎等价），所以
// 客户端主动合并是无损的，且能最大化跨后端兼容性。
//
// 行为：
//   - 收集所有 role=system 且 content 为非空 string 的消息内容
//   - 用空行分隔拼成单条 system，置于 messages 头部
//   - 其余消息（含 content 非 string 的 system，如多模态）保持原有顺序
//   - 0 或 1 条 string system 时直接返回原 messages，避免不必要的拷贝
func mergeSystemMessages(messages []map[string]any) []map[string]any {
	var sysParts []string
	var others []map[string]any
	for _, m := range messages {
		if role, _ := m["role"].(string); role == "system" {
			if content, ok := m["content"].(string); ok && content != "" {
				sysParts = append(sysParts, content)
				continue
			}
		}
		others = append(others, m)
	}
	if len(sysParts) <= 1 {
		return messages
	}
	merged := map[string]any{"role": "system", "content": strings.Join(sysParts, "\n\n")}
	out := make([]map[string]any, 0, len(others)+1)
	out = append(out, merged)
	out = append(out, others...)
	return out
}
