package adapter_test

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

func TestRecordedLegacyResponsesSources(t *testing.T) {
	root := "openairesponses/testdata/rig"
	body, err := os.ReadFile(filepath.Join(root, "sources.json"))
	require.NoError(t, err)
	manifest := replayDecode[replayManifest](t, body)
	require.Len(t, manifest.Files, 2)
	assertReplaySources(t, root, manifest.Files)
}

func assertLegacyResponsesRequest(t *testing.T, got, source replayObject) {
	t.Helper()
	assertStatelessRequest(t, got)
	assert.JSONEq(t, string(source["model"]), string(got["model"]))
	assert.JSONEq(t, string(source["include"]), string(got["include"]))
	assert.JSONEq(t, string(source["tools"]), string(got["tools"]))
	input := replayDecode[[]replayObject](t, got["input"])
	want := replayDecode[[]replayObject](t, source["input"])
	require.Len(t, input, len(want)+1)
	assert.Equal(t, "system", replayDecode[string](t, input[0]["role"]))
	assert.JSONEq(t, string(source["instructions"]), string(input[0]["content"]))
	user := replayDecode[[]replayObject](t, want[0]["content"])
	require.Len(t, user, 1)
	assert.JSONEq(t, `"input_text"`, string(user[0]["type"]))
	assert.JSONEq(t, string(want[0]["role"]), string(input[1]["role"]))
	assert.JSONEq(t, string(user[0]["text"]), string(input[1]["content"]))
	for i := 1; i < len(want); i++ {
		if string(want[i]["type"]) == `"function_call_output"` {
			// status=completed 是录制中的可选字段；Goink 按公共文本工具结果编码。
			for _, field := range []string{"type", "call_id", "output"} {
				assert.JSONEq(t, string(want[i][field]), string(input[i+1][field]), "tool result %d field %s", i, field)
			}
		} else {
			assert.JSONEq(t, string(replayEncode(t, want[i])), string(replayEncode(t, input[i+1])), "native item %d", i)
		}
	}
}

func TestRecordedLegacyResponsesConversation(t *testing.T) {
	b, err := os.ReadFile("openairesponses/testdata/rig/responses-stateless-tools.yaml")
	require.NoError(t, err)
	decoder := yaml.NewDecoder(bytes.NewReader(b))
	var exchanges []replayExchange
	for {
		var x struct {
			When struct{ Path, Method, Body string }
			Then struct {
				Status int
				Header []struct{ Name, Value string }
				Body   string
			}
		}
		err := decoder.Decode(&x)
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		require.Equal(t, "/v1/responses", x.When.Path)
		require.Equal(t, "POST", x.When.Method)
		require.Equal(t, 200, x.Then.Status)
		contentType := ""
		for _, header := range x.Then.Header {
			if strings.EqualFold(header.Name, "Content-Type") {
				contentType = header.Value
			}
		}
		require.Contains(t, contentType, "text/event-stream")
		exchanges = append(exchanges, replayExchange{Request: replayDecode[replayObject](t, []byte(x.When.Body)), Wire: x.Then.Body})
	}
	require.Len(t, exchanges, 2)
	for _, exchange := range exchanges {
		assert.JSONEq(t, `false`, string(exchange.Request["store"]))
		assert.JSONEq(t, `true`, string(exchange.Request["stream"]))
		assert.JSONEq(t, `["reasoning.encrypted_content"]`, string(exchange.Request["include"]))
		assert.NotContains(t, exchange.Request, "previous_response_id")
		assert.NotContains(t, exchange.Request, "conversation")
		require.Len(t, replayDecode[[]replayObject](t, exchange.Request["tools"]), 2)
	}
	require.Len(t, replayDecode[[]replayObject](t, exchanges[0].Request["input"]), 1)
	backend, requests := replayBackend(t, adapter.ProtocolOpenAIResponses, []string{exchanges[0].Wire, exchanges[1].Wire})
	req := responsesReplayRequest(t, exchanges[0].Request)
	for i, x := range exchanges {
		result := collectReplay(t, backend, req)
		require.Len(t, *requests, i+1)
		assertLegacyResponsesRequest(t, (*requests)[i], x.Request)
		assertReplayRequest(t, adapter.ProtocolOpenAIResponses, (*requests)[i], req)
		assertResponsesResult(t, x.Wire, result)
		assertNativeResponsesHistory(t, (*requests)[i], req.Messages)
		assert.Equal(t, []string{"", "The labels are crimson-harbor and silver-orchard."}[i], result.Message.Content)
		assert.True(t, result.Message.HasThinking)
		assert.Empty(t, result.Message.ThinkingContent)
		assert.Equal(t, "completed", result.Message.State.StopReason)
		assert.Equal(t, []float64{160, 402}[i], result.Usage["prompt_tokens"])
		assert.Equal(t, []float64{226, 99}[i], result.Usage["completion_tokens"])
		assert.Equal(t, []float64{386, 501}[i], result.Usage["total_tokens"])
		assert.Equal(t, map[string]any{"reasoning_tokens": []float64{128, 64}[i]}, result.Usage["completion_tokens_details"])
		if i == 0 {
			require.Len(t, result.Message.ToolCalls, 2)
			req.Messages = append(req.Messages, storedReplayMessage(t, result.Message))
			results := responsesToolResults(t, exchanges[1].Request)
			require.Len(t, results, 2)
			slices.Reverse(results)
			req.Messages = append(req.Messages, results...)
		}
	}
}

func TestRecordedLegacyAnthropicStreams(t *testing.T) {
	files, err := filepath.Glob("../testdata/sse/anthropic/*.sse")
	require.NoError(t, err)
	require.Len(t, files, 6)
	for _, path := range files {
		t.Run(filepath.Base(path), func(t *testing.T) {
			body, err := os.ReadFile(path)
			require.NoError(t, err)
			wire := string(body)
			backend, _ := replayBackend(t, adapter.ProtocolAnthropicMessages, []string{wire})
			var wantText strings.Builder
			var wantCalls []string
			for _, frame := range replayFrames(t, wire) {
				e := replayDecode[replayObject](t, frame)
				if frameType(t, frame) == "content_block_start" {
					block := replayDecode[replayObject](t, e["content_block"])
					if string(block["type"]) == `"text"` {
						wantText.WriteString(replayDecode[string](t, block["text"]))
					}
					if string(block["type"]) == `"tool_use"` {
						wantCalls = append(wantCalls, replayDecode[string](t, block["id"]))
					}
				}
				if frameType(t, frame) == "content_block_delta" {
					delta := replayDecode[replayObject](t, e["delta"])
					if string(delta["type"]) == `"text_delta"` {
						wantText.WriteString(replayDecode[string](t, delta["text"]))
					}
				}
			}
			result := collectReplay(t, backend, adapter.Request{ProviderName: "recorded", Model: "legacy-recorded-model",
				Messages: []adapter.Message{{Role: "user", Content: "Replay the recorded response."}}})
			assert.Equal(t, wantText.String(), result.Message.Content)
			require.Len(t, result.Message.ToolCalls, len(wantCalls))
			for i, call := range result.Message.ToolCalls {
				assert.Equal(t, wantCalls[i], call.ID)
				assert.True(t, json.Valid(call.Arguments))
			}
		})
	}
}
