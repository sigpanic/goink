package adapter_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

// These are constructed read/edit protocol scenarios, not provider recordings or tool execution tests.
// The public field names follow ReadArgs/EditArgs; schemas omit descriptions irrelevant to transport.
var goinkReplayTools = []adapter.ToolDefinition{
	{Name: "read", Description: "读取章节或大纲", InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"include_lines":{"type":"boolean"},"start_line":{"type":"integer"},"end_line":{"type":"integer"}},"required":["path"]}`)},
	{Name: "edit", Description: "编辑章节正文", InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"change_type":{"type":"string"},"new_content":{"type":"string"}},"required":["path","change_type"]}`)},
}

func argumentPieces(text string, count int) []string {
	runes := []rune(text)
	pieces := make([]string, count)
	for i := range pieces {
		pieces[i] = string(runes[len(runes)*i/count : len(runes)*(i+1)/count])
	}
	return pieces
}

func deriveGoinkStream(t *testing.T, protocol adapter.Protocol, source, name, arguments, text string, round int) string {
	t.Helper()
	frames := replayFrames(t, source)
	argumentCount, textCount := 0, 0
	for _, raw := range frames {
		e := replayDecode[replayObject](t, raw)
		kind := frameType(t, raw)
		if kind == "response.function_call_arguments.delta" {
			argumentCount++
		}
		if kind == "response.output_text.delta" {
			textCount++
		}
		if kind == "content_block_delta" {
			d := replayDecode[replayObject](t, e["delta"])
			if string(d["type"]) == `"input_json_delta"` {
				argumentCount++
			}
			if string(d["type"]) == `"text_delta"` {
				textCount++
			}
		}
	}
	argumentChunks, textChunks := argumentPieces(arguments, argumentCount), argumentPieces(text, textCount)
	argumentIndex, textIndex := 0, 0
	var rewrite func(any)
	rewrite = func(value any) {
		switch node := value.(type) {
		case map[string]any:
			if cipher, ok := node["encrypted_content"].(string); ok && cipher != "" {
				node["encrypted_content"] = "synthetic-offline-reasoning"
			}
			if signature, ok := node["signature"].(string); ok && signature != "" {
				node["signature"] = "synthetic-offline-signature"
			}
			switch node["type"] {
			case "function_call":
				node["name"] = name
				if node["arguments"] != "" {
					node["arguments"] = arguments
				}
			case "tool_use":
				node["id"], node["name"] = fmt.Sprintf("toolu_goink_%d", round), name
			case "message":
				if protocol == adapter.ProtocolAnthropicMessages {
					node["id"] = fmt.Sprintf("msg_goink_%d", round)
				}
			case "output_text":
				if node["text"] != "" {
					node["text"] = text
				}
			}
			for _, child := range node {
				rewrite(child)
			}
		case []any:
			for _, child := range node {
				rewrite(child)
			}
		}
	}
	var wire strings.Builder
	for _, raw := range frames {
		e := replayDecode[map[string]any](t, raw)
		kind := e["type"].(string)
		rewrite(e)
		switch kind {
		case "response.function_call_arguments.delta":
			e["delta"] = argumentChunks[argumentIndex]
			argumentIndex++
		case "response.function_call_arguments.done":
			e["arguments"] = arguments
		case "response.output_text.delta":
			e["delta"] = textChunks[textIndex]
			textIndex++
		case "response.output_text.done":
			e["text"] = text
		case "content_block_delta":
			d := e["delta"].(map[string]any)
			if d["type"] == "input_json_delta" {
				d["partial_json"] = argumentChunks[argumentIndex]
				argumentIndex++
			}
			if d["type"] == "text_delta" {
				d["text"] = textChunks[textIndex]
				textIndex++
			}
		}
		if response, ok := e["response"].(map[string]any); ok {
			var tools []any
			for _, tool := range goinkReplayTools {
				tools = append(tools, map[string]any{"type": "function", "name": tool.Name, "description": tool.Description,
					"parameters": replayDecode[any](t, tool.InputSchema), "strict": false})
			}
			response["tools"], response["model"] = tools, "gpt-6.1-sol"
		}
		if protocol == adapter.ProtocolAnthropicMessages {
			fmt.Fprintf(&wire, "event: %s\n", kind)
		}
		fmt.Fprintf(&wire, "data: %s\n\n", replayEncode(t, e))
	}
	return wire.String()
}

func goinkReplayScenario(t *testing.T, protocol adapter.Protocol) (adapter.Request, []string, []string, []string) {
	t.Helper()
	chapter := strings.Repeat("林澈说：“别回头。”\n雨声掩住脚步，窗上留下了\\形划痕。\n", 90)
	arguments := []string{
		`{"path":"chapters/id_42.md","include_lines":true,"start_line":1,"end_line":2000}`,
		`{"path":"outlines/id_42.md","include_lines":false}`,
		string(replayEncode(t, map[string]any{"path": "chapters/id_42.md", "change_type": "full_replace", "new_content": chapter})),
	}
	results := []string{"1|林澈说：“别回头。”\n2|雨落在旧城。", "第42章大纲：保留窗上的划痕作为伏笔。", "已修改 chapters/id_42.md，保留了窗上的划痕。"}
	finalText := "已根据大纲修改第42章，保留人物伏笔。"
	var originals []string
	model := "gpt-6.1-sol"
	if protocol == adapter.ProtocolAnthropicMessages {
		model = "claude-sonnet-5-5"
		xs := goodallConversation(t)
		originals = []string{xs[0].Wire, xs[0].Wire, xs[0].Wire, xs[1].Wire}
	} else {
		originals = framedJSONLines(t, "vercel/openai-reasoning-encrypted-content.1.chunks.txt")
	}
	var wires []string
	for i, wire := range originals {
		name, args, text := "", "", finalText
		if i < 3 {
			name, args, text = []string{"read", "read", "edit"}[i], arguments[i], ""
		}
		wires = append(wires, deriveGoinkStream(t, protocol, wire, name, args, text, i))
	}
	request := adapter.Request{ProviderName: "recorded", Model: model,
		Messages: []adapter.Message{{Role: "system", Content: "你是网文写作助手。请先读取正文与大纲，再修改章节。"},
			{Role: "user", Content: "请修改第42章，保留窗上划痕的伏笔。"}}, Tools: goinkReplayTools}
	return request, wires, arguments, results
}

func TestGoinkToolReplayReadOutlineAndEdit(t *testing.T) {
	for _, protocol := range []adapter.Protocol{adapter.ProtocolOpenAIResponses, adapter.ProtocolAnthropicMessages} {
		t.Run(string(protocol), func(t *testing.T) {
			req, wires, arguments, results := goinkReplayScenario(t, protocol)
			backend, requests := replayBackend(t, protocol, wires)
			for i := range wires {
				result := collectReplay(t, backend, req)
				require.Len(t, *requests, i+1)
				got := (*requests)[i]
				assertReplayRequest(t, protocol, got, req)
				if protocol == adapter.ProtocolOpenAIResponses {
					assertStatelessRequest(t, got)
					assertNativeResponsesHistory(t, got, req.Messages)
					assert.JSONEq(t, `{"effort":"medium","summary":"auto"}`, string(got["reasoning"]))
				} else {
					assertAnthropicHistory(t, got, req.Messages)
					assert.JSONEq(t, `{"type":"adaptive","display":"summarized"}`, string(got["thinking"]))
				}
				if i == 3 {
					assert.Empty(t, result.Message.ToolCalls)
					assert.Equal(t, "已根据大纲修改第42章，保留人物伏笔。", result.Message.Content)
					continue
				}
				require.Len(t, result.Message.ToolCalls, 1)
				call := result.Message.ToolCalls[0]
				assert.Equal(t, []string{"read", "read", "edit"}[i], call.Name)
				assert.JSONEq(t, arguments[i], string(call.Arguments))
				pieces := result.Arguments[call.ID]
				require.Greater(t, len(pieces), 1)
				for j, piece := range pieces {
					assert.True(t, strings.HasPrefix(arguments[i], piece), "argument event %d must be cumulative", j)
				}
				assert.Equal(t, arguments[i], pieces[len(pieces)-1])
				req.Messages = append(req.Messages, storedReplayMessage(t, result.Message),
					adapter.Message{Role: "tool", ToolCallID: call.ID, ToolName: call.Name, Content: results[i]})
			}
		})
	}
}

func TestGoinkToolReplayTruncatedEditCannotExecute(t *testing.T) {
	for _, protocol := range []adapter.Protocol{adapter.ProtocolOpenAIResponses, adapter.ProtocolAnthropicMessages} {
		t.Run(string(protocol), func(t *testing.T) {
			req, wires, _, _ := goinkReplayScenario(t, protocol)
			wire := wires[2]
			// Re-encoding sorts keys, so locate the terminal by its parsed type rather than its byte order.
			var partial strings.Builder
			for _, frame := range replayFrames(t, wire) {
				kind := frameType(t, frame)
				if kind == "response.completed" || kind == "message_stop" {
					break
				}
				if protocol == adapter.ProtocolAnthropicMessages {
					fmt.Fprintf(&partial, "event: %s\n", kind)
				}
				fmt.Fprintf(&partial, "data: %s\n\n", frame)
			}
			backend, requests := replayBackend(t, protocol, []string{partial.String()})
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			var fault error
			for event := range backend.Stream(ctx, req) {
				assert.NotEqual(t, adapter.EventToolCallEnd, event.Type)
				assert.NotEqual(t, adapter.EventResponseState, event.Type)
				if event.Type == adapter.EventError {
					fault = event.Error
				}
			}
			require.NoError(t, ctx.Err())
			require.Error(t, fault)
			require.Len(t, *requests, 1)
		})
	}
}
