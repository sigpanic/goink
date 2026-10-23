package gemini

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

type streamStep struct {
	raw           map[string]json.RawMessage
	kind          string
	id, name      string
	stopped       bool
	signed        bool
	argumentDelta bool
	arguments     strings.Builder
	text          strings.Builder
	content       []map[string]json.RawMessage
	summary       []json.RawMessage
}

func newStreamStep(raw json.RawMessage, emit func(adapter.StreamEvent) bool) (*streamStep, error) {
	step := &streamStep{}
	if json.Unmarshal(raw, &step.raw) != nil || step.raw == nil || json.Unmarshal(step.raw["type"], &step.kind) != nil {
		return nil, invalidResponse("invalid step.start payload")
	}
	switch step.kind {
	case "model_output":
		if data := step.raw["content"]; len(data) > 0 && json.Unmarshal(data, &step.content) != nil {
			return nil, invalidResponse("invalid initial model_output content")
		}
		var text strings.Builder
		for i, block := range step.content {
			var kind string
			var value *string
			if json.Unmarshal(block["type"], &kind) != nil || kind != "text" || json.Unmarshal(block["text"], &value) != nil || value == nil {
				return nil, invalidResponse("model_output only supports text content")
			}
			text.WriteString(*value)
			if i == len(step.content)-1 {
				step.text.WriteString(*value)
			}
		}
		if text.Len() > 0 {
			emit(adapter.StreamEvent{Type: adapter.EventContent, Data: text.String()})
		}
	case "thought":
		if data := step.raw["signature"]; len(data) > 0 {
			var signature string
			if json.Unmarshal(data, &signature) != nil {
				return nil, invalidResponse("thought signature must be a string")
			}
			step.signed = signature != ""
		}
		if data := step.raw["summary"]; len(data) > 0 && json.Unmarshal(data, &step.summary) != nil {
			return nil, invalidResponse("thought summary must be an array")
		}
		for _, content := range step.summary {
			if err := emitSummary(content, emit); err != nil {
				return nil, err
			}
		}
	case "function_call":
		if json.Unmarshal(step.raw["id"], &step.id) != nil || json.Unmarshal(step.raw["name"], &step.name) != nil ||
			strings.TrimSpace(step.id) == "" || strings.TrimSpace(step.name) == "" {
			return nil, toolArgumentsError("function_call requires an ID and name")
		}
		if data := step.raw["arguments"]; len(data) > 0 && !jsonObject(data) {
			return nil, toolArgumentsError("initial function arguments must be an object")
		}
		emit(adapter.StreamEvent{Type: adapter.EventToolCallStart, Delta: &adapter.ToolCallDelta{ToolID: step.id, ToolName: step.name}})
	default:
		return nil, invalidResponse("unsupported streamed step " + step.kind)
	}
	return step, nil
}

func (s *streamStep) delta(raw json.RawMessage, emit func(adapter.StreamEvent) bool) error {
	var delta struct {
		Type        string          `json:"type"`
		Text        *string         `json:"text"`
		Arguments   *string         `json:"arguments"`
		Signature   *string         `json:"signature"`
		Content     json.RawMessage `json:"content"`
		Annotations json.RawMessage `json:"annotations"`
	}
	if json.Unmarshal(raw, &delta) != nil {
		return invalidResponse("invalid step.delta payload")
	}
	switch {
	case s.kind == "model_output" && delta.Type == "text":
		if delta.Text == nil {
			return invalidResponse("text delta requires text")
		}
		s.text.WriteString(*delta.Text)
		if *delta.Text != "" {
			emit(adapter.StreamEvent{Type: adapter.EventContent, Data: *delta.Text})
		}
	case s.kind == "model_output" && delta.Type == "text_annotation_delta":
		if len(s.content) == 0 {
			s.content = []map[string]json.RawMessage{{"type": json.RawMessage(`"text"`)}}
		}
		var annotations, additional []json.RawMessage
		last := s.content[len(s.content)-1]
		if data := last["annotations"]; len(data) > 0 && json.Unmarshal(data, &annotations) != nil {
			return invalidResponse("invalid initial text annotations")
		}
		if json.Unmarshal(delta.Annotations, &additional) != nil {
			return invalidResponse("annotation delta requires an array")
		}
		encoded, err := json.Marshal(append(annotations, additional...))
		if err != nil {
			return err
		}
		last["annotations"] = encoded
	case s.kind == "thought" && delta.Type == "thought_summary" && !s.signed:
		if err := emitSummary(delta.Content, emit); err != nil {
			return err
		}
		s.summary = append(s.summary, delta.Content)
	case s.kind == "thought" && delta.Type == "thought_signature" && !s.signed:
		if delta.Signature == nil || *delta.Signature == "" {
			return invalidResponse("thought signature delta requires the original signature")
		}
		// Interactions 在 thought 结束前发送完整签名，不将多次签名当文本分片拼接。
		encoded, err := json.Marshal(*delta.Signature)
		if err != nil {
			return err
		}
		s.raw["signature"], s.signed = encoded, true
	case s.kind == "function_call" && delta.Type == "arguments_delta":
		if delta.Arguments == nil || *delta.Arguments == "" {
			return nil
		}
		if !s.argumentDelta {
			var initial map[string]json.RawMessage
			if data := s.raw["arguments"]; len(data) > 0 && json.Unmarshal(data, &initial) != nil {
				return toolArgumentsError("invalid initial function arguments")
			}
			if len(initial) > 0 {
				return toolArgumentsError("cannot append JSON fragments to complete initial arguments")
			}
			s.argumentDelta = true
		}
		s.arguments.WriteString(*delta.Arguments)
		emit(adapter.StreamEvent{Type: adapter.EventToolCallArguments, Delta: &adapter.ToolCallDelta{
			ToolID: s.id, ToolName: s.name, ArgumentsText: s.arguments.String(),
		}})
	default:
		return invalidResponse(fmt.Sprintf("unsupported delta %q for step %q", delta.Type, s.kind))
	}
	return nil
}

func (s *streamStep) stop() error {
	switch s.kind {
	case "model_output":
		if len(s.content) == 0 {
			s.content = []map[string]json.RawMessage{{"type": json.RawMessage(`"text"`)}}
		}
		text, err := json.Marshal(s.text.String())
		if err != nil {
			return err
		}
		s.content[len(s.content)-1]["text"] = text
		encoded, err := json.Marshal(s.content)
		if err != nil {
			return err
		}
		s.raw["content"] = encoded
	case "thought":
		if !s.signed {
			return invalidResponse("thought stopped without its signature")
		}
		if len(s.summary) > 0 {
			encoded, err := json.Marshal(s.summary)
			if err != nil {
				return err
			}
			s.raw["summary"] = encoded
		}
	case "function_call":
		if s.argumentDelta {
			s.raw["arguments"] = json.RawMessage(s.arguments.String())
		}
		if !jsonObject(s.raw["arguments"]) {
			return toolArgumentsError("function stopped without valid object arguments")
		}
	}
	s.stopped = true
	return nil
}

func emitSummary(raw json.RawMessage, emit func(adapter.StreamEvent) bool) error {
	var content struct {
		Type string  `json:"type"`
		Text *string `json:"text"`
	}
	if json.Unmarshal(raw, &content) != nil {
		return invalidResponse("invalid thought summary content")
	}
	switch content.Type {
	case "text":
		if content.Text == nil {
			return invalidResponse("thought summary requires text")
		}
		if *content.Text != "" {
			emit(adapter.StreamEvent{Type: adapter.EventThinking, Data: *content.Text})
		}
	case "image":
	default:
		return invalidResponse("unsupported thought summary content " + content.Type)
	}
	return nil
}

func toolArgumentsError(message string) error {
	return &adapter.APIError{Kind: "tool_args_invalid", Retryable: true, Message: "invalid Gemini function: " + message}
}
