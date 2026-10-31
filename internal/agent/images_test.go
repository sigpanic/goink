package agent

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sigpanic/goink/internal/attachment"
	"github.com/sigpanic/goink/internal/config"
	"github.com/sigpanic/goink/internal/llm"
	"github.com/sigpanic/goink/internal/llm/adapter"
)

func importAgentImage(t *testing.T) attachment.Ref {
	t.Helper()
	t.Setenv("GOINK_DATA_DIR", t.TempDir())
	var data bytes.Buffer
	require.NoError(t, png.Encode(&data, image.NewGray(image.Rect(0, 0, 64, 64))))
	meta, err := attachment.NewStore(config.AttachmentsDirPath()).Import(7, "picture.png", &data)
	require.NoError(t, err)
	return attachment.Ref{AttachmentID: meta.ID, Variant: attachment.Standard}
}

func TestRunCountsImagesBeforeWindowGuard(t *testing.T) {
	ref := importAgentImage(t)
	a, opts := stateTestAgent(t)
	opts.NovelID = 7
	opts.Model.SupportsVision = true
	opts.Model.ContextWindow = 1
	opts.Messages = []llm.Message{{Role: "user", Images: []attachment.Ref{ref}}}
	script := responseStateScript{t: t}
	_, err := a.run(context.Background(), opts, script.open, func(AgentEvent) {})
	require.ErrorContains(t, err, "95%")
	require.Empty(t, script.requests)
	require.Empty(t, stateRows(t, a))
}

func TestAppendImageResultCountsPixelsAndStoresOnlyReferences(t *testing.T) {
	ref := importAgentImage(t)
	a, opts := stateTestAgent(t)
	opts.NovelID = 7
	opts.Model.SupportsVision = true
	running := a.InitRunningTokens(nil)
	require.NoError(t, a.appendMsg(llm.Message{
		Role: "tool", ToolCallID: "read-image", ToolName: "read", Images: []attachment.Ref{ref},
	}, nil, &opts, running))
	require.Positive(t, running["tool"])
	require.Len(t, opts.Messages[len(opts.Messages)-1].ImageInputs, 1)
	rows := stateRows(t, a)
	require.Len(t, rows, 1)
	require.Contains(t, rows[0].ExtraMetadata, ref.AttachmentID)
	require.NotContains(t, rows[0].ExtraMetadata, "base64")
	require.NotContains(t, rows[0].ExtraMetadata, "Base64")
	loaded, err := rows[0].ToLLMMessage()
	require.NoError(t, err)
	require.Empty(t, loaded.ImageInputs)
	opts.Model.SupportsVision = false
	require.Error(t, a.appendMsg(llm.Message{Role: "tool", Images: []attachment.Ref{ref}}, nil, &opts, running))
	opts.Model.SupportsVision = true
	opts.NovelID = 8
	require.Error(t, a.appendMsg(llm.Message{Role: "tool", Images: []attachment.Ref{ref}}, nil, &opts, running))
	require.Len(t, stateRows(t, a), 1)
}

func TestGenerateSummaryResolvesImagesInNovelScope(t *testing.T) {
	ref := importAgentImage(t)
	requests := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		requests <- string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"choices":[{"finish_reason":"stop","message":{"content":"图片摘要"}}]}`)
	}))
	defer server.Close()
	model := llm.ModelInfo{ID: "vision", SupportsVision: true}
	a := newCompressTestAgent(t, "image-summary")
	var err error
	a.llm, err = llm.NewClient(&llm.UserLLMConfig{
		Version: llm.ConfigVersion,
		Adapters: map[adapter.Protocol]llm.AdapterConfig{
			adapter.ProtocolOpenAIChat: {Providers: []adapter.Provider{{
				Name: "vision", ProviderType: "openai", APIKey: "test-key", ChatURL: server.URL,
				Models: []llm.ModelInfo{model},
			}}},
		},
	}, a.logger)
	require.NoError(t, err)
	opts := RunOptions{
		SessionID: "image-summary", NovelID: 7, Model: &model,
		AdapterName: adapter.ProtocolOpenAIChat, ProviderName: "vision",
		Messages: []llm.Message{{Role: "user", Images: []attachment.Ref{ref}}},
	}
	summary, retained, err := a.generateSummary(context.Background(), &opts)
	require.NoError(t, err)
	require.Equal(t, "图片摘要", summary)
	require.Equal(t, opts.Messages, retained)
	body := <-requests
	require.Contains(t, body, "data:image/png;base64,")
	require.Contains(t, body, "上下文压缩助手")
	require.Empty(t, opts.Messages[0].ImageInputs)
}
