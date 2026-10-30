package adapter_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/sigpanic/goink/internal/llm/adapter"
	"github.com/sigpanic/goink/internal/llm/adapter/anthropic"
	"github.com/sigpanic/goink/internal/llm/adapter/openairesponses"
)

const replayRoot = "testdata/stream-replay"

type replayObject = map[string]json.RawMessage

type replayExchange struct {
	Request replayObject
	Wire    string
}

type replaySource struct{ File, URL, SHA256 string }

type replayManifest struct{ Files []replaySource }

func readReplay(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(replayRoot, name))
	require.NoError(t, err)
	return b
}

func replayDecode[T any](t *testing.T, b []byte) T {
	t.Helper()
	var v T
	require.NoError(t, json.Unmarshal(b, &v))
	return v
}

func replayEncode(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

func cassetteBody(t *testing.T, node yaml.Node) []byte {
	t.Helper()
	var value any
	require.NoError(t, node.Decode(&value))
	if object, ok := value.(map[string]any); ok {
		if content, exists := object["content"]; exists {
			value = content
		} else if raw, exists := object["string"]; exists {
			value = raw
		}
	}
	if raw, ok := value.(string); ok {
		b := []byte(raw)
		if bytes.HasPrefix(b, []byte{0x1f, 0x8b}) {
			r, err := gzip.NewReader(bytes.NewReader(b))
			require.NoError(t, err)
			b, err = io.ReadAll(r)
			require.NoError(t, err)
			require.NoError(t, r.Close())
		}
		return b
	}
	return replayEncode(t, value)
}

func replayCassette(t *testing.T, name string) []replayExchange {
	t.Helper()
	// Nodes accept both VCR's scalar/gzip bodies and cassetter's typed JSON/text bodies.
	var raw struct {
		Interactions []struct {
			Request struct {
				Method     string
				URI        string
				Body       yaml.Node
				ParsedBody yaml.Node `yaml:"parsed_body"`
			}
			Response struct{ Body yaml.Node }
		}
	}
	require.NoError(t, yaml.Unmarshal(readReplay(t, name), &raw))
	var exchanges []replayExchange
	for _, x := range raw.Interactions {
		require.Equal(t, "POST", x.Request.Method)
		require.Contains(t, x.Request.URI, "/v1/")
		node := x.Request.Body
		if x.Request.ParsedBody.Kind != 0 {
			node = x.Request.ParsedBody
		}
		exchanges = append(exchanges, replayExchange{
			Request: replayDecode[replayObject](t, cassetteBody(t, node)),
			Wire:    string(cassetteBody(t, x.Response.Body)),
		})
	}
	require.NotEmpty(t, exchanges)
	return exchanges
}

func replayFrames(t *testing.T, wire string) []json.RawMessage {
	t.Helper()
	var frames []json.RawMessage
	for _, line := range strings.Split(wire, "\n") {
		if strings.HasPrefix(line, "data:") {
			b := []byte(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			if string(b) != "[DONE]" {
				require.True(t, json.Valid(b), "invalid recorded event")
				frames = append(frames, b)
			}
		}
	}
	return frames
}

func frameType(t *testing.T, frame []byte) string {
	t.Helper()
	return replayDecode[string](t, replayDecode[replayObject](t, frame)["type"])
}

func framedJSONLines(t *testing.T, name string) []string {
	t.Helper()
	var rounds []string
	var wire strings.Builder
	for _, line := range strings.Split(strings.TrimSpace(string(readReplay(t, name))), "\n") {
		require.True(t, json.Valid([]byte(line)))
		if frameType(t, []byte(line)) == "response.created" && wire.Len() > 0 {
			rounds = append(rounds, wire.String())
			wire.Reset()
		}
		fmt.Fprintf(&wire, "data: %s\n\n", line)
	}
	if wire.Len() > 0 {
		rounds = append(rounds, wire.String())
	}
	return rounds
}

type replayRoundTripper func(*http.Request) (*http.Response, error)

func (f replayRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type shortReplayReader struct{ io.Reader }

func (r shortReplayReader) Read(b []byte) (int, error) {
	if len(b) > 7 {
		b = b[:7]
	}
	return r.Reader.Read(b)
}

func replayBackend(t *testing.T, protocol adapter.Protocol, wires []string) (adapter.ProtocolAdapter, *[]replayObject) {
	t.Helper()
	var requests []replayObject
	endpoint, providerType := "responses", "openai"
	if protocol == adapter.ProtocolAnthropicMessages {
		endpoint, providerType = "messages", "anthropic"
	}
	url := "https://offline.invalid/v1/" + endpoint
	client := &http.Client{Transport: replayRoundTripper(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != url || r.Method != http.MethodPost {
			return nil, fmt.Errorf("unexpected replay route: %s %s", r.Method, r.URL)
		}
		if protocol == adapter.ProtocolOpenAIResponses && r.Header.Get("Accept") != "text/event-stream" {
			return nil, fmt.Errorf("replay request did not negotiate SSE")
		}
		if protocol == adapter.ProtocolOpenAIResponses && r.Header.Get("Authorization") != "Bearer offline-only" ||
			protocol == adapter.ProtocolAnthropicMessages && r.Header.Get("X-Api-Key") != "offline-only" {
			return nil, fmt.Errorf("replay request lost configured credentials")
		}
		var body replayObject
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return nil, err
		}
		index := len(requests)
		requests = append(requests, body)
		if index >= len(wires) {
			return nil, fmt.Errorf("unexpected replay request %d", index+1)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}},
			Body: io.NopCloser(shortReplayReader{strings.NewReader(wires[index])})}, nil
	})}
	providers := adapter.ProviderMap{"recorded": {Name: "recorded", ProviderType: providerType, APIKey: "offline-only", ChatURL: url}}
	var backend adapter.ProtocolAdapter
	var err error
	if protocol == adapter.ProtocolAnthropicMessages {
		backend, err = anthropic.New(providers, client, nil)
	} else {
		backend, err = openairesponses.New(providers, client, nil)
	}
	require.NoError(t, err)
	return backend, &requests
}

type replayResult struct {
	Message   adapter.Message
	Usage     map[string]any
	Arguments map[string][]string
}

func collectReplay(t *testing.T, backend adapter.ProtocolAdapter, request adapter.Request) replayResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result := replayResult{Message: adapter.Message{Role: "assistant"}, Arguments: make(map[string][]string)}
	thinkingOpen := false
	for event := range backend.Stream(ctx, request) {
		switch event.Type {
		case adapter.EventError:
			require.NoError(t, event.Error)
		case adapter.EventContent:
			result.Message.Content += event.Data
		case adapter.EventThinking:
			result.Message.HasThinking, thinkingOpen = true, true
			result.Message.ThinkingContent += event.Data
		case adapter.EventThinkingDone:
			thinkingOpen = false
		case adapter.EventToolCallArguments:
			result.Arguments[event.Delta.ToolID] = append(result.Arguments[event.Delta.ToolID], event.Delta.ArgumentsText)
		case adapter.EventUsage:
			require.Nil(t, result.Usage, "usage must be published once")
			result.Usage = event.Usage
		case adapter.EventResponseState:
			require.Nil(t, result.Message.State)
			require.NotNil(t, result.Usage)
			result.Message.State = event.State
		case adapter.EventToolCallEnd:
			require.NotNil(t, result.Message.State, "tools must wait for complete replay state")
			result.Message.ToolCalls = append(result.Message.ToolCalls, adapter.ToolCall{
				ID: event.Delta.ToolID, Name: event.Delta.ToolName, Arguments: event.Delta.ArgumentsJSON,
			})
		}
	}
	require.NoError(t, ctx.Err())
	require.False(t, thinkingOpen)
	require.NotNil(t, result.Message.State)
	require.True(t, result.Message.State.Complete)
	assert.Equal(t, len(result.Message.ToolCalls) == 0, result.Message.State.TurnComplete)
	return result
}

func storedReplayMessage(t *testing.T, message adapter.Message) adapter.Message {
	t.Helper()
	loaded := replayDecode[adapter.Message](t, replayEncode(t, message))
	require.Equal(t, message, loaded)
	return loaded
}

func assertReplayRequest(t *testing.T, protocol adapter.Protocol, got replayObject, request adapter.Request) {
	t.Helper()
	assert.Equal(t, request.Model, replayDecode[string](t, got["model"]))
	tools := replayDecode[[]replayObject](t, got["tools"])
	require.Len(t, tools, len(request.Tools))
	schemaField := "parameters"
	if protocol == adapter.ProtocolAnthropicMessages {
		schemaField = "input_schema"
	}
	for i, expected := range request.Tools {
		assert.Equal(t, expected.Name, replayDecode[string](t, tools[i]["name"]))
		assert.Equal(t, expected.Description, replayDecode[string](t, tools[i]["description"]))
		assert.JSONEq(t, string(expected.InputSchema), string(tools[i][schemaField]))
		if protocol == adapter.ProtocolOpenAIResponses {
			assert.JSONEq(t, `false`, string(tools[i]["strict"]))
		}
	}
	if protocol == adapter.ProtocolOpenAIResponses {
		input := replayDecode[[]replayObject](t, got["input"])
		for i, message := range request.Messages {
			if message.Role != "system" && message.Role != "user" {
				break
			}
			require.Greater(t, len(input), i)
			assert.Equal(t, message.Role, replayDecode[string](t, input[i]["role"]))
			assert.Equal(t, message.Content, replayDecode[string](t, input[i]["content"]))
		}
	}
}

func assertReplaySources(t *testing.T, root string, sources []replaySource) map[string]bool {
	t.Helper()
	require.NotEmpty(t, sources)
	listed := map[string]bool{}
	for _, file := range sources {
		require.False(t, listed[file.File], "duplicate source: %s", file.File)
		listed[file.File] = true
		body, err := os.ReadFile(filepath.Join(root, file.File))
		require.NoError(t, err)
		assert.Equal(t, file.SHA256, fmt.Sprintf("%x", sha256.Sum256(body)), file.File)
		assert.Contains(t, file.URL, "raw.githubusercontent.com/")
	}
	return listed
}

func TestRecordedStreamSources(t *testing.T) {
	manifest := replayDecode[replayManifest](t, readReplay(t, "sources.json"))
	listed := assertReplaySources(t, replayRoot, manifest.Files)
	require.NoError(t, filepath.WalkDir(replayRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		name, err := filepath.Rel(replayRoot, path)
		if err != nil {
			return err
		}
		if name != "README.md" && name != "sources.json" {
			assert.True(t, listed[filepath.ToSlash(name)], "unlisted source: %s", name)
		}
		return nil
	}))
}
