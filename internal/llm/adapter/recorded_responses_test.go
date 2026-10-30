package adapter_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

func responsesOutput(t *testing.T, wire string) ([]json.RawMessage, replayObject) {
	t.Helper()
	var output []json.RawMessage
	var terminal replayObject
	for _, frame := range replayFrames(t, wire) {
		e := replayDecode[replayObject](t, frame)
		switch frameType(t, frame) {
		case "response.output_item.done":
			output = append(output, e["item"])
		case "response.completed":
			terminal = replayDecode[replayObject](t, e["response"])
		}
	}
	require.NotEmpty(t, terminal, "recording must contain a completed response")
	require.NotEmpty(t, output)
	return output, terminal
}

func responsesReplayRequest(t *testing.T, source replayObject) adapter.Request {
	t.Helper()
	req := adapter.Request{ProviderName: "recorded", Model: replayDecode[string](t, source["model"])}
	if text := source["instructions"]; len(text) > 0 && string(text) != "null" {
		req.Messages = append(req.Messages, adapter.Message{Role: "system", Content: replayDecode[string](t, text)})
	}
	for _, raw := range replayDecode[[]replayObject](t, source["input"]) {
		role := replayDecode[string](t, raw["role"])
		content := raw["content"]
		var text string
		if len(content) > 0 && content[0] == '"' {
			text = replayDecode[string](t, content)
		} else {
			for _, part := range replayDecode[[]replayObject](t, content) {
				text += replayDecode[string](t, part["text"])
			}
		}
		req.Messages = append(req.Messages, adapter.Message{Role: role, Content: text})
	}
	for _, tool := range replayDecode[[]replayObject](t, source["tools"]) {
		var description string
		if b := tool["description"]; len(b) > 0 && string(b) != "null" {
			description = replayDecode[string](t, b)
		}
		req.Tools = append(req.Tools, adapter.ToolDefinition{
			Name: replayDecode[string](t, tool["name"]), Description: description, InputSchema: tool["parameters"],
		})
	}
	return req
}

func responsesToolResults(t *testing.T, request replayObject) []adapter.Message {
	t.Helper()
	var results []adapter.Message
	for _, item := range replayDecode[[]replayObject](t, request["input"]) {
		if string(item["type"]) != `"function_call_output"` {
			continue
		}
		output := item["output"]
		text := string(output)
		if len(output) > 0 && output[0] == '"' {
			text = replayDecode[string](t, output)
		}
		results = append(results, adapter.Message{Role: "tool", ToolCallID: replayDecode[string](t, item["call_id"]), Content: text})
	}
	return results
}

func assertStatelessRequest(t *testing.T, got replayObject) {
	t.Helper()
	assert.JSONEq(t, `false`, string(got["store"]))
	assert.JSONEq(t, `false`, string(got["background"]))
	assert.JSONEq(t, `true`, string(got["stream"]))
	assert.JSONEq(t, `["reasoning.encrypted_content"]`, string(got["include"]))
	assert.NotContains(t, got, "previous_response_id")
	assert.NotContains(t, got, "conversation")
}

func assertResponsesResult(t *testing.T, wire string, result replayResult) {
	t.Helper()
	output, terminal := responsesOutput(t, wire)
	var text, summary strings.Builder
	var calls []adapter.ToolCall
	for _, raw := range output {
		item := replayDecode[replayObject](t, raw)
		switch string(item["type"]) {
		case `"message"`:
			for _, part := range replayDecode[[]replayObject](t, item["content"]) {
				text.WriteString(replayDecode[string](t, part["text"]))
			}
		case `"reasoning"`:
			for _, part := range replayDecode[[]replayObject](t, item["summary"]) {
				summary.WriteString(replayDecode[string](t, part["text"]))
			}
		case `"function_call"`:
			calls = append(calls, adapter.ToolCall{ID: replayDecode[string](t, item["call_id"]),
				Name: replayDecode[string](t, item["name"]), Arguments: json.RawMessage(replayDecode[string](t, item["arguments"]))})
		}
	}
	assert.Equal(t, text.String(), result.Message.Content)
	assert.Equal(t, summary.String(), result.Message.ThinkingContent)
	assert.Equal(t, calls, result.Message.ToolCalls)
	payload := replayDecode[replayObject](t, result.Message.State.Payload)
	assert.JSONEq(t, string(replayEncode(t, output)), string(payload["output"]))
	usage := replayDecode[map[string]any](t, terminal["usage"])
	assert.Equal(t, usage["input_tokens"], result.Usage["prompt_tokens"])
	assert.Equal(t, usage["output_tokens"], result.Usage["completion_tokens"])
	assert.Equal(t, usage["total_tokens"], result.Usage["total_tokens"])
	assert.Equal(t, usage["output_tokens_details"], result.Usage["completion_tokens_details"])
}

func assertNativeResponsesHistory(t *testing.T, got replayObject, messages []adapter.Message) {
	t.Helper()
	input := replayDecode[[]json.RawMessage](t, got["input"])
	var native []json.RawMessage
	for _, message := range messages {
		if message.State != nil {
			payload := replayDecode[replayObject](t, message.State.Payload)
			native = append(native, replayDecode[[]json.RawMessage](t, payload["output"])...)
		}
	}
	index := 0
	for _, item := range input {
		if index < len(native) && assertJSONContains(item, native[index]) {
			index++
		}
	}
	assert.Equal(t, len(native), index, "completed native items must survive storage and replay in order")
	var results []replayObject
	for _, raw := range input {
		item := replayDecode[replayObject](t, raw)
		if string(item["type"]) == `"function_call_output"` {
			results = append(results, item)
		}
	}
	byID := map[string]adapter.Message{}
	for _, message := range messages {
		if message.Role == "tool" {
			byID[message.ToolCallID] = message
		}
	}
	var tools []adapter.Message
	for _, message := range messages {
		for _, call := range message.ToolCalls {
			result, found := byID[call.ID]
			require.True(t, found, "missing result for tool %s", call.ID)
			tools = append(tools, result)
		}
	}
	require.Len(t, results, len(tools))
	for i, message := range tools {
		assert.Equal(t, message.ToolCallID, replayDecode[string](t, results[i]["call_id"]))
		assert.Equal(t, message.Content, replayDecode[string](t, results[i]["output"]))
	}
}

func assertJSONContains(got, want json.RawMessage) bool {
	var a, b any
	if json.Unmarshal(got, &a) != nil || json.Unmarshal(want, &b) != nil {
		return false
	}
	left, _ := json.Marshal(a)
	right, _ := json.Marshal(b)
	return string(left) == string(right)
}

func TestRecordedResponsesToolConversations(t *testing.T) {
	grafanaRequests := strings.Split(strings.TrimSpace(string(readReplay(t, "grafana/expected-requests.jsonl"))), "\n")
	var grafana []replayExchange
	for i, line := range grafanaRequests {
		request := replayDecode[replayObject](t, replayDecode[replayObject](t, []byte(line))["body"])
		wires := framedJSONLines(t, fmt.Sprintf("grafana/input-%d.chunks.txt", i+1))
		require.Len(t, wires, 1)
		grafana = append(grafana, replayExchange{Request: request, Wire: wires[0]})
	}
	for _, tc := range []struct {
		name      string
		exchanges []replayExchange
	}{
		{"llm_nonempty_arguments", replayCassette(t, "llm/test_responses_tool_use_streaming.yaml")},
		{"grafana_reasoning_and_tools", grafana},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Len(t, tc.exchanges, 2)
			var wires []string
			for _, x := range tc.exchanges {
				assert.JSONEq(t, `false`, string(x.Request["store"]))
				assert.JSONEq(t, `true`, string(x.Request["stream"]))
				assert.NotContains(t, x.Request, "previous_response_id")
				assert.NotContains(t, x.Request, "conversation")
				wires = append(wires, x.Wire)
			}
			backend, requests := replayBackend(t, adapter.ProtocolOpenAIResponses, wires)
			req := responsesReplayRequest(t, tc.exchanges[0].Request)
			for i, x := range tc.exchanges {
				result := collectReplay(t, backend, req)
				require.Len(t, *requests, i+1)
				assertStatelessRequest(t, (*requests)[i])
				assertReplayRequest(t, adapter.ProtocolOpenAIResponses, (*requests)[i], req)
				assertResponsesResult(t, x.Wire, result)
				assertNativeResponsesHistory(t, (*requests)[i], req.Messages)
				if i == 0 {
					require.Len(t, result.Message.ToolCalls, 1)
					call := result.Message.ToolCalls[0]
					require.Greater(t, len(result.Arguments[call.ID]), 1)
					assert.Equal(t, string(call.Arguments), result.Arguments[call.ID][len(result.Arguments[call.ID])-1])
					req.Messages = append(req.Messages, storedReplayMessage(t, result.Message))
					results := responsesToolResults(t, tc.exchanges[1].Request)
					require.Len(t, results, 1)
					assert.Equal(t, call.ID, results[0].ToolCallID)
					req.Messages = append(req.Messages, results...)
				}
			}
		})
	}
}

func TestRecordedResponsesEncryptedReasoningAcrossFourResponses(t *testing.T) {
	wires := framedJSONLines(t, "vercel/openai-reasoning-encrypted-content.1.chunks.txt")
	require.Len(t, wires, 4)
	backend, requests := replayBackend(t, adapter.ProtocolOpenAIResponses, wires)
	first := replayDecode[replayObject](t, replayFrames(t, wires[0])[0])
	source := replayDecode[replayObject](t, first["response"])
	source["input"] = replayEncode(t, []map[string]any{{"role": "user", "content": "Calculate ((12+7)*3)*10 using calculator."}})
	req := responsesReplayRequest(t, source)
	for i, wire := range wires {
		result := collectReplay(t, backend, req)
		assertStatelessRequest(t, (*requests)[i])
		assertReplayRequest(t, adapter.ProtocolOpenAIResponses, (*requests)[i], req)
		assertResponsesResult(t, wire, result)
		assertNativeResponsesHistory(t, (*requests)[i], req.Messages)
		if i == 0 {
			require.NotEmpty(t, result.Message.ThinkingContent)
			var added, done string
			for _, frame := range replayFrames(t, wire) {
				e := replayDecode[replayObject](t, frame)
				if item := e["item"]; len(item) > 0 {
					object := replayDecode[replayObject](t, item)
					if b := object["encrypted_content"]; len(b) > 0 {
						if frameType(t, frame) == "response.output_item.added" {
							added = replayDecode[string](t, b)
						}
						if frameType(t, frame) == "response.output_item.done" {
							done = replayDecode[string](t, b)
						}
					}
				}
			}
			assert.NotEmpty(t, added)
			assert.NotEmpty(t, done)
			assert.NotEqual(t, added, done)
		}
		if i < 3 {
			require.Len(t, result.Message.ToolCalls, 1)
			assert.JSONEq(t, []string{`{"a":12,"b":7,"op":"add"}`, `{"a":19,"b":3,"op":"multiply"}`, `{"a":57,"b":10,"op":"multiply"}`}[i], string(result.Message.ToolCalls[0].Arguments))
			req.Messages = append(req.Messages, storedReplayMessage(t, result.Message), adapter.Message{
				Role: "tool", ToolCallID: result.Message.ToolCalls[0].ID, Content: []string{"19", "57", "570"}[i],
			})
		} else {
			assert.Contains(t, result.Message.Content, "570")
		}
	}
	require.Len(t, *requests, 4)
}
