package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sigpanic/goink/internal/agent"
	"github.com/sigpanic/goink/internal/llm"
	"github.com/sigpanic/goink/internal/llm/adapter"
	"github.com/sigpanic/goink/internal/session"
)

func setupChatRoutingTest(t *testing.T, baseURL string) *App {
	t.Helper()
	a := setupLLMMessageTestApp(t)
	a.ctx = context.Background()
	require.NoError(t, a.db.AutoMigrate(&session.Session{}))
	config := &llm.UserLLMConfig{Version: llm.ConfigVersion, Adapters: map[adapter.Protocol]llm.AdapterConfig{}}
	for protocol, providerType := range map[adapter.Protocol]string{
		adapter.ProtocolOpenAIChat: "openai", adapter.ProtocolAnthropicMessages: "anthropic",
		adapter.ProtocolGeminiInteractions: "google", adapter.ProtocolOpenAIResponses: "openai",
	} {
		config.Adapters[protocol] = llm.AdapterConfig{Providers: []adapter.Provider{{
			Name: "relay", ProviderType: providerType, APIKey: "test-key", ChatURL: baseURL + "/" + string(protocol),
			Models: []llm.ModelInfo{{ID: "shared-model", Name: string(protocol)}, {ID: "only-" + string(protocol)}},
		}}}
	}
	var err error
	a.llmClient, err = llm.NewClient(config, a.logger)
	require.NoError(t, err)
	a.agent = agent.New(a.llmClient, nil, a.session, nil, a.db, nil, a.logger, nil, agent.NewCancelManager())
	return a
}

func TestChatAndCompressionRequireExplicitProtocol(t *testing.T) {
	a := setupChatRoutingTest(t, "https://example.invalid")
	require.NoError(t, a.db.Create(&session.Session{SessionID: "existing", ActiveVersion: 1}).Error)
	// 正确路由进入数据库步骤后立即失败，避免启动模型请求、Git 操作或 Wails 事件。
	require.NoError(t, a.db.Exec(`CREATE TRIGGER reject_routing_session BEFORE INSERT ON sessions
		BEGIN SELECT RAISE(ABORT, 'session write reached'); END`).Error)
	require.NoError(t, a.db.Exec(`CREATE TRIGGER reject_routing_turn BEFORE UPDATE ON sessions
		BEGIN SELECT RAISE(ABORT, 'turn allocation reached'); END`).Error)
	for _, tt := range []struct {
		name     string
		protocol adapter.Protocol
		model    string
		valid    bool
	}{
		{name: "chat", protocol: adapter.ProtocolOpenAIChat, model: "shared-model", valid: true},
		{name: "anthropic", protocol: adapter.ProtocolAnthropicMessages, model: "shared-model", valid: true},
		{name: "gemini", protocol: adapter.ProtocolGeminiInteractions, model: "shared-model", valid: true},
		{name: "responses", protocol: adapter.ProtocolOpenAIResponses, model: "shared-model", valid: true},
		{name: "missing protocol", model: "shared-model"},
		{name: "unknown protocol", protocol: "unknown", model: "shared-model"},
		{name: "foreign model", protocol: adapter.ProtocolAnthropicMessages, model: "only-openai_chat"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			selection := llm.ModelSelection{AdapterName: tt.protocol, ProviderName: "relay", ModelID: tt.model}
			_, err := a.Chat(ChatInput{ModelSelection: selection, Message: "继续"})
			if tt.valid {
				require.ErrorContains(t, err, "session write reached")
			} else {
				require.ErrorContains(t, err, "模型未找到")
			}
			_, err = a.CompressContext(CompressInput{SessionID: "existing", ModelSelection: selection})
			if tt.valid {
				require.ErrorContains(t, err, "turn allocation reached")
			} else {
				require.ErrorContains(t, err, "模型未找到")
			}
		})
	}
	sess, err := a.session.GetSession(a.ctx, "existing")
	require.NoError(t, err)
	require.Zero(t, sess.LastTurnID)
	var count int64
	require.NoError(t, a.db.Model(&session.Session{}).Count(&count).Error)
	require.Equal(t, int64(1), count)
	require.NoError(t, a.db.Model(&session.Message{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestGenerateTitleUsesSelectedProtocol(t *testing.T) {
	type request struct {
		path string
		body []byte
	}
	requests := make(chan request, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		select {
		case requests <- request{r.URL.Path, body}:
		default:
			t.Error("unexpected extra title request")
		}
		// 测试实际出站请求；错误响应让标题生成在更新数据库、发送 Wails 事件之前返回。
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"type":"error","error":{"type":"invalid_request_error","message":"stop after capture","code":400,"status":"INVALID_ARGUMENT"}}`)
	}))
	t.Cleanup(server.Close)
	a := setupChatRoutingTest(t, server.URL)
	for _, protocol := range []adapter.Protocol{adapter.ProtocolOpenAIChat, adapter.ProtocolAnthropicMessages, adapter.ProtocolGeminiInteractions, adapter.ProtocolOpenAIResponses} {
		t.Run(string(protocol), func(t *testing.T) {
			a.generateTitle("existing", "测试标题", protocol, "relay", "shared-model")
			select {
			case got := <-requests:
				require.Equal(t, "/"+string(protocol), got.path)
				var payload struct{ Model string }
				require.NoError(t, json.Unmarshal(got.body, &payload))
				require.Equal(t, "shared-model", payload.Model)
				require.Contains(t, string(got.body), "测试标题")
				require.Contains(t, string(got.body), "基于用户消息")
			default:
				t.Fatal("title generation did not reach the selected protocol endpoint")
			}
		})
	}
	require.Empty(t, requests, "unexpected extra title requests")
}
