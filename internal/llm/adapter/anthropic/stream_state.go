package anthropic

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	sdk "github.com/anthropics/anthropic-sdk-go"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

type streamBlock struct {
	kind, id, name string
	closed         bool
	input          json.RawMessage
	arguments      strings.Builder
}

type streamState struct {
	message                 sdk.Message
	started, updated, ended bool
	blocks                  []*streamBlock
	discardNative           bool
}

func (s *streamState) consume(event sdk.MessageStreamEventUnion) (adapter.StreamEvent, bool, error) {
	var out adapter.StreamEvent
	publish := false
	switch event.Type {
	case "message_start", "content_block_start", "content_block_delta", "content_block_stop", "message_delta", "message_stop":
	default:
		return out, false, nil
	}
	if s.ended || (!s.started && event.Type != "message_start") {
		return out, false, invalidResponse("unexpected stream event " + event.Type)
	}
	switch event.Type {
	case "message_start":
		message := event.Message
		if s.started || len(message.Content) != 0 {
			return out, false, invalidResponse("invalid message_start")
		}
		s.started = true
	case "content_block_start":
		index, err := streamIndex(event)
		if err != nil || s.updated || index != int64(len(s.blocks)) {
			return out, false, invalidResponse("invalid content block start index or order")
		}
		block := event.ContentBlock
		tracked := &streamBlock{kind: block.Type, id: block.ID, name: block.Name}
		switch block.Type {
		case "text":
			if !stringField(block.JSON.Text) {
				return out, false, invalidResponse("text block requires text")
			}
			out = adapter.StreamEvent{Type: adapter.EventContent, Data: block.Text}
			publish = block.Text != ""
		case "thinking":
			if !stringField(block.JSON.Thinking) {
				s.discardNative = true
			}
			if raw := block.JSON.Signature.Raw(); raw != "" && raw != "null" && !stringField(block.JSON.Signature) {
				s.discardNative = true
			}
			out = adapter.StreamEvent{Type: adapter.EventThinking}
			if stringField(block.JSON.Thinking) {
				out.Data = block.Thinking
			}
			publish = true
		case "redacted_thinking":
			out, publish = adapter.StreamEvent{Type: adapter.EventThinking}, true
		case "tool_use":
			if !stringField(block.JSON.ID) || !stringField(block.JSON.Name) || block.ID == "" || block.Name == "" {
				return out, false, invalidResponse("tool block requires an ID and name")
			}
			caller := block.JSON.Caller.Raw()
			if block.ToolsetName != "" || (caller != "" && caller != "null" && (!block.JSON.Caller.Valid() || block.Caller.Type != "direct")) {
				return out, false, invalidResponse("provider-executed tool calls are not supported")
			}
			for _, previous := range s.blocks {
				if previous.kind == "tool_use" && previous.id == block.ID {
					return out, false, invalidResponse("duplicate streamed tool ID " + block.ID)
				}
			}
			tracked.input = json.RawMessage(block.JSON.Input.Raw())
			if !jsonObject(tracked.input) {
				return out, false, invalidToolInput(block.ID)
			}
			out = adapter.StreamEvent{Type: adapter.EventToolCallStart, Delta: &adapter.ToolCallDelta{ToolID: block.ID, ToolName: block.Name}}
			publish = true
		default:
		}
		s.blocks = append(s.blocks, tracked)
	case "content_block_delta", "content_block_stop":
		index, err := streamIndex(event)
		if err != nil || s.updated || index >= int64(len(s.blocks)) {
			return out, false, invalidResponse("invalid content block index or order")
		}
		block := s.blocks[index]
		if block.closed {
			return out, false, invalidResponse("content block already closed")
		}
		if event.Type == "content_block_stop" {
			if block.kind == "tool_use" {
				if block.arguments.Len() > 0 {
					block.input = json.RawMessage(block.arguments.String())
				}
				if !jsonObject(block.input) {
					return out, false, invalidToolInput(block.id)
				}
			}
			block.closed = true
			if block.kind == "thinking" || block.kind == "redacted_thinking" {
				out, publish = adapter.StreamEvent{Type: adapter.EventThinkingDone}, true
			}
			break
		}
		delta := event.Delta
		switch {
		case block.kind == "text" && delta.Type == "text_delta":
			if !stringField(delta.JSON.Text) {
				return out, false, invalidResponse("text delta requires text")
			}
			out, publish = adapter.StreamEvent{Type: adapter.EventContent, Data: delta.Text}, delta.Text != ""
		case block.kind == "text" && delta.Type == "citations_delta":
			if !jsonObject(json.RawMessage(delta.JSON.Citation.Raw())) {
				return out, false, nil
			}
		case block.kind == "thinking" && delta.Type == "thinking_delta":
			if !stringField(delta.JSON.Thinking) {
				s.discardNative = true
				return out, false, nil
			}
			out, publish = adapter.StreamEvent{Type: adapter.EventThinking, Data: delta.Thinking}, delta.Thinking != ""
		case block.kind == "thinking" && delta.Type == "signature_delta":
			if !stringField(delta.JSON.Signature) {
				s.discardNative = true
				return out, false, nil
			}
		case block.kind == "tool_use" && delta.Type == "input_json_delta":
			if !stringField(delta.JSON.PartialJSON) {
				return out, false, invalidToolInput(block.id)
			}
			if delta.PartialJSON != "" && block.arguments.Len() == 0 && !sameJSON(block.input, json.RawMessage(`{}`)) {
				return out, false, invalidToolInput(block.id)
			}
			block.arguments.WriteString(delta.PartialJSON)
			out = adapter.StreamEvent{Type: adapter.EventToolCallArguments, Delta: &adapter.ToolCallDelta{
				ToolID: block.id, ToolName: block.name, ArgumentsText: block.arguments.String(),
			}}
			publish = true
		default:
			return out, false, nil
		}
	case "message_delta":
		if !s.allClosed() {
			return out, false, invalidResponse("message_delta before block completion or without usage")
		}
		s.updated = true
	case "message_stop":
		if !s.allClosed() {
			return out, false, invalidResponse("message_stop before complete blocks and message_delta")
		}
		s.ended = true
	default:
	}
	if err := s.message.Accumulate(event); err != nil {
		return out, false, invalidResponse("SDK accumulation failed: " + err.Error())
	}
	return out, publish, nil
}

func (s *streamState) allClosed() bool {
	for _, block := range s.blocks {
		if !block.closed {
			return false
		}
	}
	return true
}

func streamIndex(event sdk.MessageStreamEventUnion) (int64, error) {
	index, err := strconv.ParseInt(strings.TrimSpace(event.JSON.Index.Raw()), 10, 64)
	if err != nil || index < 0 || index != event.Index {
		return 0, fmt.Errorf("invalid stream content index")
	}
	return index, nil
}

func invalidToolInput(id string) error {
	return &adapter.APIError{Kind: "tool_args_invalid", Retryable: true, Message: "invalid streamed object arguments for tool " + id}
}

func (s *streamState) finish(p Provider, params sdk.MessageNewParams) (adapter.Response, error) {
	if !s.started || !s.ended {
		return adapter.Response{}, &adapter.APIError{Kind: "incomplete_response", Retryable: true, Message: "Anthropic stream ended without message_stop"}
	}
	// SDK 更新 RawJSON 后不会刷新所有 JSON 字段的元数据，重解码最终原文再复用完整响应转换。
	var message sdk.Message
	if err := json.Unmarshal([]byte(s.message.RawJSON()), &message); err != nil {
		return adapter.Response{}, invalidResponse("invalid accumulated message: " + err.Error())
	}
	if len(message.Content) != len(s.blocks) {
		return adapter.Response{}, invalidResponse("SDK accumulation lost content blocks")
	}
	for i, block := range s.blocks {
		if block.kind == "tool_use" && !sameJSON(block.input, message.Content[i].Input) {
			return adapter.Response{}, invalidToolInput(block.id)
		}
	}
	response, err := p.convertResponse(params, &message)
	if err == nil && s.discardNative {
		response.State.Payload = nil
	}
	return response, err
}
