package adapter_test

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

func anthropicText(t *testing.T, content json.RawMessage) string {
	t.Helper()
	if len(content) > 0 && content[0] == '"' {
		return replayDecode[string](t, content)
	}
	var text string
	for _, part := range replayDecode[[]replayObject](t, content) {
		if string(part["type"]) == `"text"` {
			text += replayDecode[string](t, part["text"])
		}
	}
	return text
}

func anthropicReplayRequest(t *testing.T, source replayObject) adapter.Request {
	t.Helper()
	maxTokens := replayDecode[int](t, source["max_tokens"])
	effort := replayDecode[string](t, replayDecode[replayObject](t, source["output_config"])["effort"])
	req := adapter.Request{ProviderName: "recorded", Model: replayDecode[string](t, source["model"]),
		Options: &adapter.CallOptions{MaxTokens: &maxTokens, ReasoningEffort: &effort}}
	for _, message := range replayDecode[[]replayObject](t, source["messages"]) {
		req.Messages = append(req.Messages, adapter.Message{Role: replayDecode[string](t, message["role"]), Content: anthropicText(t, message["content"])})
	}
	for _, tool := range replayDecode[[]replayObject](t, source["tools"]) {
		req.Tools = append(req.Tools, adapter.ToolDefinition{Name: replayDecode[string](t, tool["name"]),
			Description: replayDecode[string](t, tool["description"]), InputSchema: tool["input_schema"]})
	}
	return req
}

func goodallConversation(t *testing.T) []replayExchange {
	t.Helper()
	var xs []replayExchange
	for _, prefix := range []string{"live_thinking_tool_use", "live_thinking_tool_use_turn2"} {
		xs = append(xs, replayExchange{
			Request: replayDecode[replayObject](t, readReplay(t, "goodall/"+prefix+".request.json")),
			Wire:    string(readReplay(t, "goodall/"+prefix+".sse")),
		})
	}
	return xs
}

func assertAnthropicHistory(t *testing.T, got replayObject, messages []adapter.Message) {
	t.Helper()
	actual := replayDecode[[]replayObject](t, got["messages"])
	for _, message := range messages {
		if message.State == nil {
			continue
		}
		payload := replayDecode[replayObject](t, message.State.Payload)
		found := false
		for _, item := range actual {
			if string(item["role"]) == `"assistant"` && assertJSONContains(item["content"], payload["content"]) {
				found = true
			}
		}
		assert.True(t, found, "signed native content must survive storage and request encoding")
	}
	var results []replayObject
	for _, message := range actual {
		for _, block := range replayDecode[[]replayObject](t, message["content"]) {
			if string(block["type"]) == `"tool_result"` {
				results = append(results, block)
			}
		}
	}
	var expected []adapter.Message
	for _, message := range messages {
		if message.Role == "tool" {
			expected = append(expected, message)
		}
	}
	require.Len(t, results, len(expected))
	for i, message := range expected {
		assert.Equal(t, message.ToolCallID, replayDecode[string](t, results[i]["tool_use_id"]))
		assert.Equal(t, message.Content, anthropicText(t, results[i]["content"]))
	}
}

func TestRecordedAnthropicSignedToolConversations(t *testing.T) {
	for _, tc := range []struct {
		name                       string
		exchanges                  []replayExchange
		textHashes, thinkingHashes [2]string
		input, output, reasoning   [2]float64
	}{
		{
			name:           "pydantic_adaptive_thinking",
			exchanges:      replayCassette(t, "pydantic/test_anthropic_tool_with_thinking.yaml"),
			textHashes:     [2]string{"108a22967663d4402b3f1f919391472f2f5669c1e560bf6338a5a324b810bbf4", "3730a46942bf30ba7f3c61c1bede5ddf72ef92053f1046d757255c89b9bcc2fb"},
			thinkingHashes: [2]string{"2d9c45825a3bc968869a2f0537f3bc1ad547525c2df14860404fd90fd13855be", "57ea392e860eba9a192fdb395a64ae80f432db633b5e084b148905cf50982fa7"},
			input:          [2]float64{354, 434}, output: [2]float64{71, 270}, reasoning: [2]float64{31, 108},
		},
		{
			name:           "goodall_thinking_cache_and_nonempty_arguments",
			exchanges:      goodallConversation(t),
			textHashes:     [2]string{"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", "372953b59af558fa60e29d7f0e11b03caaedbeed03dc33f18c6cf895ec819047"},
			thinkingHashes: [2]string{"251ea6e38e2a942c85abd5d64b5be002a7c5055066bf0404fb1409915a965eff", "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"},
			input:          [2]float64{519, 788}, output: [2]float64{236, 289}, reasoning: [2]float64{161, 0},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Len(t, tc.exchanges, 2)
			backend, requests := replayBackend(t, adapter.ProtocolAnthropicMessages, []string{tc.exchanges[0].Wire, tc.exchanges[1].Wire})
			req := anthropicReplayRequest(t, tc.exchanges[0].Request)
			for i := range tc.exchanges {
				result := collectReplay(t, backend, req)
				require.Len(t, *requests, i+1)
				got := (*requests)[i]
				assertReplayRequest(t, adapter.ProtocolAnthropicMessages, got, req)
				assert.JSONEq(t, `true`, string(got["stream"]))
				for _, key := range []string{"model", "thinking", "output_config", "max_tokens", "tools"} {
					assert.JSONEq(t, string(tc.exchanges[i].Request[key]), string(got[key]), key)
				}
				assertAnthropicHistory(t, got, req.Messages)
				assert.Equal(t, tc.textHashes[i], fmt.Sprintf("%x", sha256.Sum256([]byte(result.Message.Content))))
				assert.Equal(t, tc.thinkingHashes[i], fmt.Sprintf("%x", sha256.Sum256([]byte(result.Message.ThinkingContent))))
				assert.Equal(t, tc.input[i], result.Usage["prompt_tokens"])
				assert.Equal(t, tc.output[i], result.Usage["completion_tokens"])
				assert.Equal(t, tc.input[i]+tc.output[i], result.Usage["total_tokens"])
				assert.Equal(t, map[string]any{"reasoning_tokens": tc.reasoning[i]}, result.Usage["completion_tokens_details"])
				if i == 0 {
					require.Len(t, result.Message.ToolCalls, 1)
					messages := replayDecode[[]replayObject](t, tc.exchanges[1].Request["messages"])
					require.Len(t, messages, 3)
					payload := replayDecode[replayObject](t, result.Message.State.Payload)
					blocks := replayDecode[[]replayObject](t, payload["content"])
					expectedBlocks := replayDecode[[]replayObject](t, messages[1]["content"])
					require.Len(t, blocks, len(expectedBlocks))
					for j, expected := range expectedBlocks {
						for key, value := range expected {
							assert.JSONEq(t, string(value), string(blocks[j][key]), "native block %d field %s", j, key)
						}
					}
					signed := false
					for _, block := range blocks {
						if string(block["type"]) == `"thinking"` {
							signature := replayDecode[string](t, block["signature"])
							require.Greater(t, len(signature), 100)
							assert.NotContains(t, result.Message.Content, signature)
							assert.NotContains(t, result.Message.ThinkingContent, signature)
							signed = true
						}
					}
					require.True(t, signed)
					req.Messages = append(req.Messages, storedReplayMessage(t, result.Message))
					results := replayDecode[[]replayObject](t, messages[2]["content"])
					require.Len(t, results, 1)
					call := result.Message.ToolCalls[0]
					assert.Equal(t, call.ID, replayDecode[string](t, results[0]["tool_use_id"]))
					req.Messages = append(req.Messages, adapter.Message{Role: "tool", ToolCallID: call.ID,
						ToolName: call.Name, Content: anthropicText(t, results[0]["content"])})
				}
			}
		})
	}
}
