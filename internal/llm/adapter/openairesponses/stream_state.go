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
	accumulator    responses.ResponseAccumulator
	responseID     string
	items          []*streamItem
	terminal       *responses.Response
	text, thinking strings.Builder
}

func (s *streamState) consume(event responses.ResponseStreamEventUnion, emit func(adapter.StreamEvent) bool) error {
	if event.Type == "error" {
		return streamError(json.RawMessage(event.RawJSON()))
	}
	if s.terminal != nil || (s.responseID == "" && event.Type != "response.created") {
		return invalidResponse("unexpected stream event " + event.Type)
	}
	// 固定 SDK 累积器接收 WebSocket 类型；相同 SSE 事件的 RawJSON 可无损解码，原事件仍用于实时显示。
	var observed responses.ResponsesServerEventUnion
	if err := json.Unmarshal([]byte(event.RawJSON()), &observed); err != nil {
		return invalidResponse("invalid SDK stream event")
	}
	if err := s.accumulator.AddEvent(observed); err != nil {
		return invalidResponse("SDK accumulation failed: " + err.Error())
	}
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
		if s.responseID != "" || !wireString(event.Response.JSON.ID) || event.Response.ID == "" ||
			event.Response.Object != "response" || event.Response.Status != "in_progress" || len(event.Response.Output) != 0 {
			return invalidResponse("invalid response.created")
		}
		s.responseID = event.Response.ID
		return nil
	case "response.in_progress":
		if len(s.items) != 0 || event.Response.ID != s.responseID || event.Response.Status != "in_progress" {
			return invalidResponse("unexpected response.in_progress")
		}
		return nil
	case "response.completed", "response.failed", "response.incomplete":
		if event.Response.ID != s.responseID || "response."+string(event.Response.Status) != event.Type {
			return invalidResponse("terminal response identity or status changed")
		}
		s.terminal = &event.Response
		return nil
	case "response.output_item.added":
		item := event.Item
		if !wireIndex(event.JSON.OutputIndex) || event.OutputIndex != int64(len(s.items)) || !wireString(item.JSON.ID) || item.ID == "" {
			return invalidResponse("invalid output item index or ID")
		}
		for _, previous := range s.items {
			if previous.id == item.ID || (item.Type == "function_call" && previous.callID == item.CallID) {
				return invalidResponse("duplicate streamed item or function call ID")
			}
		}
		tracked := &streamItem{id: item.ID, kind: item.Type, callID: item.CallID, name: item.Name, arguments: item.Arguments.OfString}
		switch item.Type {
		case "message":
			if item.Role != "assistant" || !item.JSON.Content.Valid() || len(item.Content) != 0 {
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
			return invalidResponse("unsupported streamed item " + item.Type)
		}
		s.items = append(s.items, tracked)
		return nil
	}
	item, err := s.activeItem(event)
	if err != nil {
		return err
	}
	switch event.Type {
	case "response.output_item.done":
		if event.Item.Type != item.kind || event.Item.ID != item.id ||
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
		for _, current := range s.accumulator.Snapshot().Output {
			if current.OutputIndex == event.OutputIndex {
				item.arguments = current.Arguments
				emit(adapter.StreamEvent{Type: adapter.EventToolCallArguments, Delta: item.delta()})
				return nil
			}
		}
		return invalidResponse("SDK accumulation lost function arguments")
	case "response.output_text.delta", "response.refusal.delta", "response.reasoning_summary_text.delta":
		if !wireString(event.JSON.Delta) {
			return invalidResponse("stream delta requires a string")
		}
		if event.Type == "response.reasoning_summary_text.delta" {
			if item.kind != "reasoning" || !wireIndex(event.JSON.SummaryIndex) {
				return invalidResponse("invalid reasoning summary delta")
			}
			publish(adapter.EventThinking, event.Delta)
		} else {
			if item.kind != "message" || !wireIndex(event.JSON.ContentIndex) {
				return invalidResponse("invalid message content delta")
			}
			publish(adapter.EventContent, event.Delta)
		}
	case "response.content_part.added", "response.reasoning_summary_part.added":
		if event.Type == "response.reasoning_summary_part.added" {
			if item.kind != "reasoning" || !wireIndex(event.JSON.SummaryIndex) || event.Part.Type != "summary_text" || !wireString(event.Part.JSON.Text) {
				return invalidResponse("invalid reasoning summary part")
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
				return invalidResponse("unsupported message part " + event.Part.Type)
			}
		}
	case "response.content_part.done", "response.output_text.done", "response.refusal.done",
		"response.reasoning_summary_part.done", "response.reasoning_summary_text.done",
		"response.reasoning_text.delta", "response.reasoning_text.done", "response.output_text.annotation.added":
		// 完整 item 和终态仍须与实际发布的正文、摘要一致；私有 reasoning 文本不作为摘要显示。
	default:
		return invalidResponse("unsupported stream event " + event.Type)
	}
	return nil
}

func (s *streamState) activeItem(event responses.ResponseStreamEventUnion) (*streamItem, error) {
	if !wireIndex(event.JSON.OutputIndex) || event.OutputIndex >= int64(len(s.items)) {
		return nil, invalidResponse("stream event has an unknown output index")
	}
	item := s.items[event.OutputIndex]
	if item.done != nil || (event.Type != "response.output_item.done" && (!wireString(event.JSON.ItemID) || event.ItemID != item.id)) {
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
	if s.terminal.Status != "completed" {
		return p.convertResponse(params, s.terminal)
	}
	if len(s.items) == 0 || len(s.terminal.Output) != len(s.items) {
		return adapter.Response{}, invalidResponse("terminal output differs from streamed items")
	}
	output := make([]json.RawMessage, len(s.items))
	for i, item := range s.items {
		if item.done == nil || !sameCompletedItem(item.done, json.RawMessage(s.terminal.Output[i].RawJSON())) {
			return adapter.Response{}, invalidResponse("terminal output differs from completed item " + item.id)
		}
		// 使用真正收到的 item.done；added 中的密文可能仍不完整，终态也可能不再重复该字段。
		output[i] = slices.Clone(item.done)
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
	if result.Content != s.text.String() || result.ThinkingContent != s.thinking.String() {
		return adapter.Response{}, invalidResponse("terminal text or summary differs from published deltas")
	}
	return result, nil
}

func sameCompletedItem(done, terminal json.RawMessage) bool {
	var left, right map[string]json.RawMessage
	if json.Unmarshal(done, &left) != nil || json.Unmarshal(terminal, &right) != nil || left == nil || right == nil {
		return false
	}
	if string(left["type"]) == `"reasoning"` && (len(right["encrypted_content"]) == 0 || string(right["encrypted_content"]) == "null") {
		right["encrypted_content"] = left["encrypted_content"]
	}
	raw, err := json.Marshal(right)
	return err == nil && sameJSON(done, raw)
}
