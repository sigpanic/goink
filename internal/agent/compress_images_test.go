package agent

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sigpanic/goink/internal/agentcfg"
	"github.com/sigpanic/goink/internal/attachment"
	"github.com/sigpanic/goink/internal/llm"
)

func TestPersistCompressionPreservesImageReferences(t *testing.T) {
	const sessionID = "compress-images"
	a := newCompressTestAgent(t, sessionID)
	ctx := context.Background()
	images := []attachment.Ref{{AttachmentID: "picture", Variant: attachment.Standard}}
	messages := []llm.Message{
		{Role: "system", Content: "system"},
		{Role: "user", Images: images},
		{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "read-call", Name: "read", Arguments: []byte(`{"path":"attachments/picture"}`)}}},
		{Role: "tool", Content: "读取成功", ToolCallID: "read-call", ToolName: "read", Images: images},
	}
	retained := retainMessages(messages)
	retained[0].Images[0].AttachmentID = "copy-only"
	require.Equal(t, "picture", messages[1].Images[0].AttachmentID)
	retained = retainMessages(messages)
	opts := &RunOptions{SessionID: sessionID, TurnID: 2, ActiveVersion: 1, AgentType: "main"}
	version, err := a.persistCompression(ctx, opts, &agentcfg.SystemMessages{Identity: "system"}, "summary", retained)
	require.NoError(t, err)
	loaded, err := a.session.GetMessagesForAPI(ctx, sessionID, version)
	require.NoError(t, err)
	var restored []llm.Message
	for _, stored := range loaded {
		message, err := stored.ToLLMRequestMessage()
		require.NoError(t, err)
		if len(message.Images) > 0 {
			require.False(t, stored.ToFrontend)
			restored = append(restored, message)
		}
	}
	require.Equal(t, []llm.Message{messages[1], messages[3]}, restored)
}
