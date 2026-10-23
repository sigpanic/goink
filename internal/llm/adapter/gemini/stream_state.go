package gemini

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"

	types "google.golang.org/genai/interactions/models/interactions"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

type streamState struct {
	interaction map[string]json.RawMessage
	usage       map[string]json.RawMessage
	steps       []*streamStep
	terminal    bool
	completed   bool
	snapshot    json.RawMessage
	text        strings.Builder
	thinking    strings.Builder
}

func (s *streamState) consume(raw json.RawMessage, emit func(adapter.StreamEvent) bool) error {
	var event struct {
		Type          string          `json:"event_type"`
		Interaction   json.RawMessage `json:"interaction"`
		InteractionID string          `json:"interaction_id"`
		Status        string          `json:"status"`
		Index         *int            `json:"index"`
		Step          json.RawMessage `json:"step"`
		Delta         json.RawMessage `json:"delta"`
		Usage         json.RawMessage `json:"usage"`
		Metadata      *struct {
			Usage json.RawMessage `json:"total_usage"`
		} `json:"metadata"`
		Error *types.Error `json:"error"`
	}
	if err := json.Unmarshal(raw, &event); err != nil {
		return invalidResponse("invalid SSE event JSON")
	}
	if event.Type == "error" {
		kind, message := "interaction_failed", "Gemini stream reported an error"
		if event.Error != nil {
			if event.Error.Code != nil {
				kind = *event.Error.Code
			}
			if event.Error.Message != nil {
				message = *event.Error.Message
			}
		}
		return &adapter.APIError{Kind: kind, Message: message}
	}
	if s.completed || (s.terminal && event.Type != "interaction.completed") {
		return invalidResponse("event after the interaction terminal status")
	}
	if event.Type == "interaction.created" {
		if s.interaction != nil {
			return invalidResponse("duplicate interaction.created")
		}
		if err := json.Unmarshal(event.Interaction, &s.interaction); err != nil || s.interaction == nil {
			return invalidResponse("missing created interaction")
		}
		var id, status string
		if json.Unmarshal(s.interaction["id"], &id) != nil || strings.TrimSpace(id) == "" ||
			json.Unmarshal(s.interaction["status"], &status) != nil || status != "in_progress" {
			return invalidResponse("invalid created interaction identity or status")
		}
		return s.updateUsage(s.interaction["usage"])
	}
	if s.interaction == nil {
		return invalidResponse("SSE event before interaction.created")
	}
	if event.Metadata != nil {
		if err := s.updateUsage(event.Metadata.Usage); err != nil {
			return err
		}
	}
	if err := s.updateUsage(event.Usage); err != nil {
		return err
	}
	publish := func(event adapter.StreamEvent) bool {
		if event.Type == adapter.EventContent {
			s.text.WriteString(event.Data)
		}
		if event.Type == adapter.EventThinking {
			s.thinking.WriteString(event.Data)
		}
		return emit(event)
	}
	switch event.Type {
	case "interaction.completed":
		var terminal map[string]json.RawMessage
		if err := json.Unmarshal(event.Interaction, &terminal); err != nil || terminal == nil {
			return invalidResponse("missing terminal interaction")
		}
		var id, status string
		if json.Unmarshal(terminal["id"], &id) != nil || json.Unmarshal(terminal["status"], &status) != nil {
			return invalidResponse("invalid terminal interaction identity or status")
		}
		if err := s.setStatus(id, status); err != nil {
			return err
		}
		if !s.terminal {
			return invalidResponse("interaction.completed has a nonterminal status")
		}
		s.snapshot = slices.Clone(terminal["steps"])
		for key, value := range terminal {
			s.interaction[key] = value
		}
		s.completed = true
		return s.updateUsage(terminal["usage"])
	case "interaction.status_update":
		return s.setStatus(event.InteractionID, event.Status)
	case "step.start", "step.delta", "step.stop":
		if event.Index == nil || *event.Index < 0 {
			return invalidResponse("SSE step requires a nonnegative index")
		}
		index := *event.Index
		if event.Type == "step.start" {
			if index != len(s.steps) {
				return invalidResponse("duplicate or out-of-order step.start index")
			}
			step, err := newStreamStep(event.Step, publish)
			if err != nil {
				return err
			}
			s.steps = append(s.steps, step)
			return nil
		}
		if index >= len(s.steps) || s.steps[index].stopped {
			return invalidResponse("SSE delta or stop refers to an inactive step")
		}
		step := s.steps[index]
		if event.Type == "step.delta" {
			return step.delta(event.Delta, publish)
		}
		return step.stop()
	default:
		return invalidResponse("unsupported SSE event " + event.Type)
	}
}

func (s *streamState) setStatus(id, status string) error {
	var expected string
	if json.Unmarshal(s.interaction["id"], &expected) != nil || expected != id {
		return invalidResponse("interaction ID changed during streaming")
	}
	if s.terminal {
		var previous string
		if json.Unmarshal(s.interaction["status"], &previous) != nil || previous != status {
			return invalidResponse("conflicting interaction terminal status")
		}
	}
	switch status {
	case "in_progress":
	case "completed", "requires_action":
		for _, step := range s.steps {
			if !step.stopped {
				return invalidResponse("interaction ended with an open step")
			}
		}
		s.terminal = true
	case "failed", "cancelled", "incomplete":
		return &adapter.APIError{Kind: "interaction_" + status, Message: "Gemini interaction is " + status}
	default:
		return invalidResponse("unsupported interaction status " + status)
	}
	encoded, err := json.Marshal(status)
	if err != nil {
		return err
	}
	s.interaction["status"] = encoded
	return nil
}

func (s *streamState) updateUsage(raw json.RawMessage) error {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "null" {
		return nil
	}
	if _, err := normalizeUsage(raw); err != nil {
		return err
	}
	var usage map[string]json.RawMessage
	if err := json.Unmarshal(raw, &usage); err != nil {
		return invalidResponse("invalid SSE usage")
	}
	if s.usage == nil {
		s.usage = make(map[string]json.RawMessage)
	}
	// 这些值是累计快照；同字段覆盖，缺省字段保留，不跨 chunk 相加。
	for key, value := range usage {
		s.usage[key] = value
	}
	return nil
}

func (s *streamState) finish(p Provider, params types.CreateModelInteraction) (adapter.Response, error) {
	if s.interaction == nil || !s.terminal {
		return adapter.Response{}, fmt.Errorf("gemini stream ended without a terminal interaction: %w", io.ErrUnexpectedEOF)
	}
	steps := make([]map[string]json.RawMessage, 0, len(s.steps))
	for _, step := range s.steps {
		if !step.stopped {
			return adapter.Response{}, invalidResponse("stream ended with an open step")
		}
		steps = append(steps, step.raw)
	}
	rawSteps, err := json.Marshal(steps)
	if err != nil {
		return adapter.Response{}, err
	}
	s.interaction["steps"] = rawSteps
	if s.usage != nil {
		raw, err := json.Marshal(s.usage)
		if err != nil {
			return adapter.Response{}, err
		}
		s.interaction["usage"] = raw
	}
	raw, err := json.Marshal(s.interaction)
	if err != nil {
		return adapter.Response{}, err
	}
	response, err := p.convertResponse(params, raw)
	if err != nil {
		return adapter.Response{}, err
	}
	if response.Content != s.text.String() || response.ThinkingContent != s.thinking.String() {
		return adapter.Response{}, invalidResponse("streamed text order does not match completed steps")
	}
	if len(s.snapshot) > 0 && strings.TrimSpace(string(s.snapshot)) != "null" {
		return s.withSnapshot(p, params, response)
	}
	return response, nil
}

// withSnapshot 保留终态附带的原生 steps，同时核对已发布内容、调用和签名。
func (s *streamState) withSnapshot(p Provider, params types.CreateModelInteraction, accumulated adapter.Response) (adapter.Response, error) {
	var snapshot []map[string]json.RawMessage
	if json.Unmarshal(s.snapshot, &snapshot) != nil || len(snapshot) != len(s.steps) {
		return adapter.Response{}, invalidResponse("terminal steps do not match streamed steps")
	}
	for i, step := range s.steps {
		if !jsonEqualScalar(step.raw["type"], snapshot[i]["type"]) || !jsonEqualScalar(step.raw["signature"], snapshot[i]["signature"]) {
			return adapter.Response{}, invalidResponse("terminal step type or signature changed")
		}
	}
	s.interaction["steps"] = s.snapshot
	raw, err := json.Marshal(s.interaction)
	if err != nil {
		return adapter.Response{}, err
	}
	response, err := p.convertResponse(params, raw)
	if err != nil {
		return adapter.Response{}, err
	}
	if response.Content != accumulated.Content || response.ThinkingContent != accumulated.ThinkingContent || len(response.ToolCalls) != len(accumulated.ToolCalls) {
		return adapter.Response{}, invalidResponse("terminal content does not match streamed content")
	}
	for i, call := range response.ToolCalls {
		previous := accumulated.ToolCalls[i]
		if call.ToolID != previous.ToolID || call.ToolName != previous.ToolName || !sameJSON(call.ArgumentsJSON, previous.ArgumentsJSON) {
			return adapter.Response{}, invalidResponse("terminal functions do not match streamed functions")
		}
	}
	return response, nil
}

func jsonEqualScalar(a, b json.RawMessage) bool {
	var left, right string
	if len(a) == 0 || len(b) == 0 {
		return len(a) == len(b)
	}
	return json.Unmarshal(a, &left) == nil && json.Unmarshal(b, &right) == nil && left == right
}
