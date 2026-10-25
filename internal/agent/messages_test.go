package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/sigpanic/goink/internal/agentcfg"
	"github.com/sigpanic/goink/internal/llm"
	"github.com/sigpanic/goink/internal/llm/adapter"
	"github.com/sigpanic/goink/internal/session"
)

func TestAppendMsgUsesStructuredHistory(t *testing.T) {
	for _, agentType := range []string{"main", "review"} {
		t.Run(agentType, func(t *testing.T) {
			a := newCompressTestAgent(t, "structured")
			opts := RunOptions{SessionID: "structured", TurnID: 7, ActiveVersion: 1, AgentType: agentType}
			if agentType != "main" {
				opts.SubTaskID = "subtask"
			}
			tokens := a.InitRunningTokens(nil)
			calls := buildToolCalls([]toolOutput{{call: llm.ToolCall{Name: "read", ID: "call_1", Arguments: json.RawMessage(`{"chapter":9007199254740993}`)}}})
			require.NoError(t, a.appendMsg(llm.Message{Role: "user", Content: "工具注入的提示"}, nil, &opts, tokens))
			require.NoError(t, a.appendMsg(llm.Message{Role: "assistant", Content: "读取章节", ThinkingContent: "显示思考", ToolCalls: calls}, map[string]any{
				"tool_displays": []map[string]any{{"tool_id": "call_1", "display_text": "读取章节"}},
			}, &opts, tokens))
			require.NoError(t, a.appendMsg(llm.Message{Role: "tool", ToolCallID: "call_1", ToolName: "read"}, nil, &opts, tokens))
			require.Equal(t, []llm.Message{
				{Role: "user", Content: "工具注入的提示"},
				{Role: "assistant", Content: "读取章节", ThinkingContent: "显示思考", ToolCalls: []llm.ToolCall{
					{ID: "call_1", Name: "read", Arguments: json.RawMessage(`{"chapter":9007199254740993}`)},
				}},
				{Role: "tool", ToolCallID: "call_1", ToolName: "read"},
			}, opts.Messages)
			var stored []session.Message
			require.NoError(t, a.db.Order("id").Find(&stored).Error)
			require.Len(t, stored, 3)
			for i, row := range stored {
				require.Equal(t, opts.TurnID, row.TurnID)
				require.Equal(t, agentType, row.AgentType)
				require.Equal(t, opts.SubTaskID, row.SubTaskID)
				require.Equal(t, agentType == "main", row.ToAPI)
				require.Equal(t, row.Role == "assistant", row.ToFrontend)
				message, err := row.ToLLMMessage()
				require.NoError(t, err)
				require.Equal(t, opts.Messages[i], message)
			}
			require.Contains(t, stored[1].ExtraMetadata, `"tool_displays"`)
			_, byRole, err := llm.CountMessagesTokens(opts.Messages)
			require.NoError(t, err)
			for role, count := range tokens {
				require.Equal(t, byRole[role], count, role)
			}
		})
	}
}

func TestPersistCompressionPreservesStructuredHistory(t *testing.T) {
	a := newCompressTestAgent(t, "structured")
	history := []llm.Message{
		{Role: "user", Content: "[2020-01-02 03:05] 继续"},
		{Role: "assistant", Content: "读取章节", ThinkingContent: "显示思考", ToolCalls: []llm.ToolCall{
			{ID: "call_1", Name: "read", Arguments: json.RawMessage(`{"chapter":9007199254740993}`)},
		}, State: &llm.ResponseState{
			Origin:   llm.MessageOrigin{AdapterName: adapter.ProtocolAnthropicMessages, ProviderName: "relay", ProviderType: "anthropic", Model: "model"},
			Complete: true, StopReason: "tool_use", Payload: json.RawMessage(`{"signature":"opaque-signature","unknown":[9007199254740993,{}]}`),
		}},
		{Role: "tool", ToolCallID: "call_1", ToolName: "read", Content: ""},
	}
	opts := RunOptions{SessionID: "structured", TurnID: 2, ActiveVersion: 1, AgentType: "main"}
	version, err := a.persistCompression(context.Background(), &opts, &agentcfg.SystemMessages{Identity: "system"}, "摘要", history)
	require.NoError(t, err)
	rows, err := a.session.GetMessagesForAPI(context.Background(), opts.SessionID, version)
	require.NoError(t, err)
	loaded := renderForAPI(t, rows)
	require.GreaterOrEqual(t, len(loaded), len(history))
	require.Equal(t, history, loaded[len(loaded)-len(history):])
	for _, row := range rows {
		require.False(t, row.ToFrontend)
	}
}

func TestPersistCompressionRollsBackInvalidStructuredMessage(t *testing.T) {
	a := newCompressTestAgent(t, "invalid")
	opts := RunOptions{SessionID: "invalid", ActiveVersion: 1}
	_, err := a.persistCompression(context.Background(), &opts, &agentcfg.SystemMessages{Identity: "system"}, "摘要", []llm.Message{
		{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "call_1", Name: "read", Arguments: json.RawMessage(`{"broken":`)}}},
	})
	require.ErrorContains(t, err, "invalid tool arguments JSON")
	sess, err := a.session.GetSession(context.Background(), opts.SessionID)
	require.NoError(t, err)
	require.Equal(t, 1, sess.ActiveVersion)
	var count int64
	require.NoError(t, a.db.Model(&session.Message{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestGenerateSummaryUsesSelectedProtocol(t *testing.T) {
	requests := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		var request struct{ Model string }
		if err := json.Unmarshal(body, &request); err != nil {
			t.Error(err)
		}
		if request.Model != "same-model" {
			t.Errorf("model = %q", request.Model)
		}
		if !json.Valid(body) || !containsCompressionPrompt(body) {
			t.Errorf("missing compression prompt: %s", body)
		}
		select {
		case requests <- r.URL.Path:
		default:
			t.Error("unexpected extra summary request")
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/messages" {
			_, _ = fmt.Fprint(w, `{"id":"msg_1","type":"message","role":"assistant","model":"same-model","content":[{"type":"text","text":"Claude 摘要"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"choices":[{"finish_reason":"stop","message":{"content":"Chat 摘要"}}]}`)
	}))
	t.Cleanup(server.Close)
	config := &llm.UserLLMConfig{Version: llm.ConfigVersion, Adapters: map[adapter.Protocol]llm.AdapterConfig{}}
	for protocol, path := range map[adapter.Protocol]string{adapter.ProtocolOpenAIChat: "/chat/completions", adapter.ProtocolAnthropicMessages: "/messages"} {
		providerType := "openai"
		if protocol == adapter.ProtocolAnthropicMessages {
			providerType = "anthropic"
		}
		config.Adapters[protocol] = llm.AdapterConfig{Providers: []adapter.Provider{{Name: "relay", ProviderType: providerType, APIKey: "test-key", ChatURL: server.URL + path}}}
	}
	a := newCompressTestAgent(t, "routing")
	var err error
	a.llm, err = llm.NewClient(config, a.logger)
	require.NoError(t, err)
	for _, tt := range []struct {
		protocol adapter.Protocol
		path     string
		summary  string
	}{
		{adapter.ProtocolOpenAIChat, "/chat/completions", "Chat 摘要"},
		{adapter.ProtocolAnthropicMessages, "/messages", "Claude 摘要"},
	} {
		t.Run(string(tt.protocol), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			opts := RunOptions{AdapterName: tt.protocol, ProviderName: "relay", Model: &llm.ModelInfo{ID: "same-model"}, Messages: []llm.Message{{Role: "system", Content: "system"}, {Role: "user", Content: "继续"}}}
			summary, retained, err := a.generateSummary(ctx, &opts)
			require.NoError(t, err)
			require.Equal(t, tt.summary, summary)
			require.Equal(t, tt.path, <-requests)
			require.Equal(t, opts.Messages[1:], retained)
			require.Len(t, opts.Messages, 2)
		})
	}
}

func containsCompressionPrompt(body []byte) bool {
	var request struct {
		Messages []struct{ Content json.RawMessage }
	}
	if json.Unmarshal(body, &request) != nil || len(request.Messages) == 0 {
		return false
	}
	content := request.Messages[len(request.Messages)-1].Content
	var text string
	if json.Unmarshal(content, &text) == nil {
		return text == compressionPrompt
	}
	var blocks []struct{ Text string }
	return json.Unmarshal(content, &blocks) == nil && len(blocks) > 0 && blocks[len(blocks)-1].Text == compressionPrompt
}
