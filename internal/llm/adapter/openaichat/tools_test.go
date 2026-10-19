package openaichat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

func TestToolDefinitionsAcrossBothModes(t *testing.T) {
	const schema = `{"type":"object","title":"original","properties":{"id":{"type":"integer","default":9007199254740993},"text":{"anyOf":[{"type":"string"},{"type":"null"}]}},"required":["id"],"additionalProperties":false}`
	for _, kind := range []string{"functions", "nil", "empty"} {
		t.Run(kind, func(t *testing.T) {
			var tools []adapter.ToolDefinition
			var wantTools []byte
			switch kind {
			case "functions":
				tools = []adapter.ToolDefinition{
					{Name: "lookup", Description: "读取条目", InputSchema: json.RawMessage(schema)},
					{Name: "list", InputSchema: json.RawMessage(`{}`)},
				}
				// 以原来的 OpenAI map 构造期望值，检查包装、字段和 schema 数值保持一致。
				var err error
				wantTools, err = json.Marshal([]map[string]any{
					{"type": "function", "function": map[string]any{
						"name": "lookup", "description": "hooked",
						"parameters": json.RawMessage(strings.Replace(schema, "original", "modified", 1)),
					}},
					{"type": "function", "function": map[string]any{"name": "list", "description": "", "parameters": json.RawMessage(`{}`)}},
				})
				if err != nil {
					t.Fatal(err)
				}
			case "empty":
				tools = []adapter.ToolDefinition{}
			}
			before, err := json.Marshal(tools)
			if err != nil {
				t.Fatal(err)
			}
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				var payload struct {
					Stream bool
					Tools  json.RawMessage
				}
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Error(err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				if !bytes.Equal(payload.Tools, wantTools) {
					t.Errorf("outgoing tools changed:\ngot  %s\nwant %s", payload.Tools, wantTools)
				}
				if payload.Stream {
					w.Header().Set("Content-Type", "text/event-stream")
					fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\ndata: [DONE]\n\n")
				} else {
					fmt.Fprint(w, `{"choices":[{"message":{"content":"ok"}}]}`)
				}
			}))
			defer server.Close()
			backend := newTestAdapter(t, adapter.Provider{ChatURL: server.URL}, server.Client())
			p := backend.providers["test"]
			p.BuildRequest = func(payload map[string]any) map[string]any {
				if kind == "functions" {
					functions := payload["tools"].([]map[string]any)
					function := functions[0]["function"].(map[string]any)
					function["description"] = "hooked"
					// 同时修改请求 map 和 schema 字节，验证两者均不共享调用方数据。
					parameters := function["parameters"].(json.RawMessage)
					index := bytes.Index(parameters, []byte("original"))
					if index < 0 {
						t.Error("a previous request changed the original tool schema")
						return payload
					}
					copy(parameters[index:], "modified")
				}
				return payload
			}
			backend.providers["test"] = p
			input := adapter.Request{ProviderName: "test", Model: "model", Tools: tools}
			response, err := backend.Generate(context.Background(), input)
			if err != nil || response.Content != "ok" {
				t.Fatalf("Generate: %+v, %v", response, err)
			}
			var content string
			for event := range backend.Stream(context.Background(), input) {
				if event.Type == EventError {
					t.Fatal(event.Error)
				}
				if event.Type == EventContent {
					content += event.Data
				}
			}
			after, err := json.Marshal(tools)
			if err != nil {
				t.Fatal(err)
			}
			if content != "ok" || requests.Load() != 2 || !bytes.Equal(before, after) {
				t.Fatalf("unexpected result or source mutation: content=%q requests=%d before=%s after=%s", content, requests.Load(), before, after)
			}
		})
	}
}

func TestBothModesRejectInvalidToolDefinitionsBeforeHTTP(t *testing.T) {
	var requests, hookCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	backend := newTestAdapter(t, adapter.Provider{ChatURL: server.URL}, server.Client())
	p := backend.providers["test"]
	p.BuildRequest = func(payload map[string]any) map[string]any {
		hookCalls.Add(1)
		return payload
	}
	backend.providers["test"] = p
	tests := []struct {
		name string
		tool adapter.ToolDefinition
	}{
		{"empty name", adapter.ToolDefinition{InputSchema: json.RawMessage(`{}`)}},
		{"blank name", adapter.ToolDefinition{Name: " \t", InputSchema: json.RawMessage(`{}`)}},
		{"missing schema", adapter.ToolDefinition{Name: "read"}},
		{"truncated schema", adapter.ToolDefinition{Name: "read", InputSchema: json.RawMessage(`{"type":`)}},
		{"null schema", adapter.ToolDefinition{Name: "read", InputSchema: json.RawMessage(`null`)}},
		{"array schema", adapter.ToolDefinition{Name: "read", InputSchema: json.RawMessage(`[]`)}},
		{"string schema", adapter.ToolDefinition{Name: "read", InputSchema: json.RawMessage(`"{}"`)}},
		{"boolean schema", adapter.ToolDefinition{Name: "read", InputSchema: json.RawMessage(`true`)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := adapter.Request{ProviderName: "test", Model: "model", Tools: []adapter.ToolDefinition{
				{Name: "valid", InputSchema: json.RawMessage(`{}`)}, tt.tool,
			}}
			if _, err := backend.Generate(context.Background(), input); err == nil || !strings.Contains(err.Error(), "tool 1") {
				t.Fatalf("invalid Generate tools: %v", err)
			}
			errorCount := 0
			for event := range backend.Stream(context.Background(), input) {
				if event.Type != EventError || event.Error == nil || !strings.Contains(event.Error.Error(), "tool 1") {
					t.Fatalf("unexpected Stream event: %+v", event)
				}
				errorCount++
			}
			if errorCount != 1 || requests.Load() != 0 || hookCalls.Load() != 0 {
				t.Fatalf("invalid tools: errors=%d requests=%d hooks=%d", errorCount, requests.Load(), hookCalls.Load())
			}
		})
	}
}
