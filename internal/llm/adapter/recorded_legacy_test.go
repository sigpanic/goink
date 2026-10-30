package adapter_test

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

func TestRecordedLegacyResponsesConversation(t *testing.T) {
	b, err := os.ReadFile("openairesponses/testdata/rig/responses-stateless-tools.yaml")
	require.NoError(t, err)
	decoder := yaml.NewDecoder(bytes.NewReader(b))
	var exchanges []replayExchange
	for {
		var x struct {
			When struct{ Body string }
			Then struct{ Body string }
		}
		err := decoder.Decode(&x)
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		exchanges = append(exchanges, replayExchange{Request: replayDecode[replayObject](t, []byte(x.When.Body)), Wire: x.Then.Body})
	}
	require.Len(t, exchanges, 2)
	backend, requests := replayBackend(t, adapter.ProtocolOpenAIResponses, []string{exchanges[0].Wire, exchanges[1].Wire})
	req := responsesReplayRequest(t, exchanges[0].Request)
	for i, x := range exchanges {
		result := collectReplay(t, backend, req)
		assertStatelessRequest(t, (*requests)[i])
		assertReplayRequest(t, adapter.ProtocolOpenAIResponses, (*requests)[i], req)
		assertResponsesResult(t, x.Wire, result)
		assertNativeResponsesHistory(t, (*requests)[i], req.Messages)
		if i == 0 {
			require.Len(t, result.Message.ToolCalls, 2)
			req.Messages = append(req.Messages, storedReplayMessage(t, result.Message))
			req.Messages = append(req.Messages, responsesToolResults(t, exchanges[1].Request)...)
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
