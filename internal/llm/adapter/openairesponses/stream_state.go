package openairesponses

import (
	"encoding/json"
	"slices"
	"strconv"
	"strings"

	"github.com/openai/openai-go/v3/packages/respjson"
	"github.com/openai/openai-go/v3/responses"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

type streamItem struct {
	id, kind, callID, name string
	arguments              string
	done                   json.RawMessage
}

type streamState struct {
	accumulator       responses.ResponseAccumulator
	accumulatorFailed bool
	started           bool
	items             []*streamItem
	terminal          *responses.Response
	text, thinking    strings.Builder
}

func (s *streamState) consume(event responses.ResponseStreamEventUnion, emit func(adapter.StreamEvent) bool) error {
	if event.Type == "error" {
		return streamError(json.RawMessage(event.RawJSON()))
	}
	switch event.Type {
	case "response.content_part.done", "response.output_text.done", "response.refusal.done",
		"response.reasoning_summary_part.done", "response.reasoning_summary_text.done",
		"response.reasoning_text.delta", "response.reasoning_text.done", "response.output_text.annotation.added":
		return nil
	}
	switch event.Type {
	case "response.created", "response.in_progress", "response.completed", "response.failed", "response.incomplete",
		"response.output_item.added", "response.output_item.done",
		"response.function_call_arguments.delta", "response.function_call_arguments.done",
		"response.output_text.delta", "response.refusal.delta", "response.reasoning_summary_text.delta",
		"response.content_part.added", "response.reasoning_summary_part.added",
		"response.content_part.done", "response.output_text.done", "response.refusal.done",
		"response.reasoning_summary_part.done", "response.reasoning_summary_text.done",
		"response.reasoning_text.delta", "response.reasoning_text.done", "response.output_text.annotation.added":
	default:
		return nil
	}
	if s.terminal != nil || (!s.started && event.Type != "response.created") {
		return invalidResponse("unexpected stream event " + event.Type)
	}
	// 固定 SDK 累积器接收 WebSocket 类型；相同 SSE 事件的 RawJSON 可无损解码，原事件仍用于实时显示。
	var observed responses.ResponsesServerEventUnion
	if err := json.Unmarshal([]byte(event.RawJSON()), &observed); err != nil {
		return invalidResponse("invalid SDK stream event")
	}
	if !s.accumulatorFailed {
		s.accumulatorFailed = s.accumulator.AddEvent(observed) != nil
	}
	accumulated := !s.accumulatorFailed
	publish := func(kind adapter.StreamEventType, text string) {
		if kind == adapter.EventThinking {
			s.thinking.WriteString(text)
		} else {
			s.text.WriteString(text)
		}
		if text != "" {
			emit(adapter.StreamEvent{Type: kind, Data: text})
		}
	}
	switch event.Type {
	case "response.created":
		if s.started || len(event.Response.Output) != 0 {
			return invalidResponse("invalid response.created")
		}
		s.started = true
		return nil
	case "response.in_progress":
		return nil
	case "response.completed", "response.failed", "response.incomplete":
		if event.Response.Status != "" && "response."+string(event.Response.Status) != event.Type {
			return invalidResponse("terminal response identity or status changed")
		}
		s.terminal = &event.Response
		return nil
	case "response.output_item.added":
		item := event.Item
		if !wireIndex(event.JSON.OutputIndex) || event.OutputIndex != int64(len(s.items)) {
			return invalidResponse("invalid output item index or ID")
		}
		for _, previous := range s.items {
			if item.Type == "function_call" && previous.kind == "function_call" && previous.callID == item.CallID {
				return invalidResponse("duplicate streamed item or function call ID")
			}
		}
		tracked := &streamItem{id: item.ID, kind: item.Type, callID: item.CallID, name: item.Name, arguments: item.Arguments.OfString}
		switch item.Type {
		case "message":
			if len(item.Content) != 0 {
				return invalidResponse("streamed message must start with empty assistant content")
			}
		case "reasoning":
			text, err := reasoningSummary([]responses.ResponseOutputItemUnion{item})
			if err != nil {
				return err
			}
			publish(adapter.EventThinking, text)
		case "function_call":
			if strings.TrimSpace(item.CallID) == "" || strings.TrimSpace(item.Name) == "" {
				return invalidResponse("streamed function requires call_id and name")
			}
			emit(adapter.StreamEvent{Type: adapter.EventToolCallStart, Delta: &adapter.ToolCallDelta{ToolID: item.CallID, ToolName: item.Name}})
			if tracked.arguments != "" {
				emit(adapter.StreamEvent{Type: adapter.EventToolCallArguments, Delta: tracked.delta()})
			}
		default:
		}
		s.items = append(s.items, tracked)
		return nil
	}
	item, err := s.activeItem(event)
	if err != nil {
		return err
	}
	if item.kind != "message" && item.kind != "reasoning" && item.kind != "function_call" && event.Type != "response.output_item.done" {
		return nil
	}
	switch event.Type {
	case "response.output_item.done":
		if event.Item.Type != item.kind ||
			(item.kind == "function_call" && (event.Item.CallID != item.callID || event.Item.Name != item.name)) {
			return invalidResponse("completed output item changed streamed identity")
		}
		if item.kind == "function_call" && item.arguments != "" && !sameJSON(json.RawMessage(item.arguments), json.RawMessage(event.Item.Arguments.OfString)) {
			return invalidStreamArguments(item.callID)
		}
		item.done = json.RawMessage(event.Item.RawJSON())
	case "response.function_call_arguments.delta", "response.function_call_arguments.done":
		if item.kind != "function_call" {
			return invalidResponse("function arguments on a non-function item")
		}
		if event.Type == "response.function_call_arguments.done" && item.arguments != "" &&
			!sameJSON(json.RawMessage(item.arguments), json.RawMessage(event.Arguments)) {
			return invalidStreamArguments(item.callID)
		}
		if accumulated {
			for _, current := range s.accumulator.Snapshot().Output {
				if current.OutputIndex == event.OutputIndex {
					item.arguments = current.Arguments
					emit(adapter.StreamEvent{Type: adapter.EventToolCallArguments, Delta: item.delta()})
					return nil
				}
			}
		}
		if event.Type == "response.function_call_arguments.delta" {
			if !wireString(event.JSON.Delta) {
				return invalidStreamArguments(item.callID)
			}
			item.arguments += event.Delta
		} else {
			item.arguments = event.Arguments
		}
		emit(adapter.StreamEvent{Type: adapter.EventToolCallArguments, Delta: item.delta()})
	case "response.output_text.delta", "response.refusal.delta", "response.reasoning_summary_text.delta":
		if event.Type == "response.reasoning_summary_text.delta" && (!wireString(event.JSON.Delta) || item.kind != "reasoning") {
			return nil
		}
		if !wireString(event.JSON.Delta) {
			return invalidResponse("stream delta requires a string")
		}
		if event.Type == "response.reasoning_summary_text.delta" {
			publish(adapter.EventThinking, event.Delta)
		} else {
			if item.kind != "message" || !wireIndex(event.JSON.ContentIndex) {
				return invalidResponse("invalid message content delta")
			}
			publish(adapter.EventContent, event.Delta)
		}
	case "response.content_part.added", "response.reasoning_summary_part.added":
		if event.Type == "response.reasoning_summary_part.added" {
			if item.kind != "reasoning" || event.Part.Type != "summary_text" || !wireString(event.Part.JSON.Text) {
				return nil
			}
			publish(adapter.EventThinking, event.Part.Text)
		} else {
			if item.kind == "reasoning" && event.Part.Type == "reasoning_text" && wireIndex(event.JSON.ContentIndex) && wireString(event.Part.JSON.Text) {
				return nil
			}
			if item.kind != "message" || !wireIndex(event.JSON.ContentIndex) {
				return invalidResponse("invalid message content part")
			}
			switch event.Part.Type {
			case "output_text":
				publish(adapter.EventContent, event.Part.Text)
			case "refusal":
				if !wireString(event.Part.JSON.Refusal) {
					return invalidResponse("refusal requires a string")
				}
				publish(adapter.EventContent, event.Part.Refusal)
			default:
			}
		}
	case "response.content_part.done", "response.output_text.done", "response.refusal.done",
		"response.reasoning_summary_part.done", "response.reasoning_summary_text.done",
		"response.reasoning_text.delta", "response.reasoning_text.done", "response.output_text.annotation.added":
		// 完整 item 和终态仍须与实际发布的正文、摘要一致；私有 reasoning 文本不作为摘要显示。
	default:
	}
	return nil
}

func (s *streamState) activeItem(event responses.ResponseStreamEventUnion) (*streamItem, error) {
	if !wireIndex(event.JSON.OutputIndex) || event.OutputIndex >= int64(len(s.items)) {
		return nil, invalidResponse("stream event has an unknown output index")
	}
	item := s.items[event.OutputIndex]
	if item.done != nil {
		return nil, invalidResponse("stream event has a closed or mismatched output item")
	}
	return item, nil
}

func (item *streamItem) delta() *adapter.ToolCallDelta {
	return &adapter.ToolCallDelta{ToolID: item.callID, ToolName: item.name, ArgumentsText: item.arguments}
}

func invalidStreamArguments(id string) error {
	return &adapter.APIError{Kind: "tool_args_invalid", Retryable: true, Message: "invalid or inconsistent streamed object arguments for function " + id}
}

func wireString(field respjson.Field) bool {
	return field.Valid() && strings.HasPrefix(strings.TrimSpace(field.Raw()), `"`)
}

func wireIndex(field respjson.Field) bool {
	index, err := strconv.ParseInt(strings.TrimSpace(field.Raw()), 10, 64)
	return field.Valid() && err == nil && index >= 0
}

func (s *streamState) finish(p Provider, params responses.ResponseNewParams) (adapter.Response, error) {
	if s.terminal == nil {
		return adapter.Response{}, &adapter.APIError{Kind: "incomplete_response", Retryable: true, Message: "OpenAI Responses stream ended without a terminal response"}
	}
	if s.terminal.Status != "completed" && s.terminal.Status != "" {
		return p.convertResponse(params, s.terminal)
	}
	if len(s.items) == 0 {
		return adapter.Response{}, invalidResponse("terminal output differs from streamed items")
	}
	output := make([]json.RawMessage, len(s.items))
	for i, item := range s.items {
		if item.done == nil {
			return adapter.Response{}, invalidResponse("terminal output differs from completed item " + item.id)
		}
		// 使用真正收到的 item.done；added 中的密文可能仍不完整，终态也可能不再重复该字段。
		// 同一 reasoning 的终态密文可与 item.done 不同；回放始终保留 item.done 原文。
		output[i] = slices.Clone(item.done)
	}
	for _, terminal := range s.terminal.Output {
		if terminal.Type != "function_call" {
			continue
		}
		matched := false
		for _, item := range s.items {
			if item.kind == "function_call" && item.callID == terminal.CallID && sameFunctionCall(item.done, json.RawMessage(terminal.RawJSON())) {
				matched = true
				break
			}
		}
		if !matched {
			return adapter.Response{}, invalidStreamArguments(terminal.CallID)
		}
	}
	var native map[string]json.RawMessage
	if err := json.Unmarshal([]byte(s.terminal.RawJSON()), &native); err != nil {
		return adapter.Response{}, invalidResponse("invalid terminal response")
	}
	var err error
	native["output"], err = json.Marshal(output)
	if err != nil {
		return adapter.Response{}, err
	}
	raw, err := json.Marshal(native)
	if err != nil {
		return adapter.Response{}, err
	}
	var complete responses.Response
	if err := json.Unmarshal(raw, &complete); err != nil {
		return adapter.Response{}, invalidResponse("invalid completed SDK response")
	}
	result, err := p.convertResponse(params, &complete)
	if err != nil {
		return adapter.Response{}, err
	}
	if result.Content != s.text.String() {
		return adapter.Response{}, invalidResponse("terminal text or summary differs from published deltas")
	}
	result.ThinkingContent = s.thinking.String()
	return result, nil
}

func sameFunctionCall(done, terminal json.RawMessage) bool {
	var left, right struct {
		Type      string `json:"type"`
		CallID    string `json:"call_id"`
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	}
	if json.Unmarshal(done, &left) != nil || json.Unmarshal(terminal, &right) != nil {
		return false
	}
	return left.Type == right.Type && left.CallID == right.CallID && left.Name == right.Name &&
		sameJSON(json.RawMessage(left.Arguments), json.RawMessage(right.Arguments))
}
