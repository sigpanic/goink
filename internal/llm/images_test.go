package llm

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sigpanic/goink/internal/attachment"
	"github.com/sigpanic/goink/internal/config"
	"github.com/sigpanic/goink/internal/llm/adapter"
)

func importTestImages(t *testing.T, novelID int64) ([]attachment.Ref, []string) {
	t.Helper()
	store := attachment.NewStore(config.AttachmentsDirPath())
	source := image.NewNRGBA(image.Rect(0, 0, 32, 48))
	source.SetNRGBA(2, 3, color.NRGBA{R: 255, A: 255})
	var refs []attachment.Ref
	var urls []string
	for _, format := range []string{"png", "jpeg"} {
		var data bytes.Buffer
		if format == "png" {
			require.NoError(t, png.Encode(&data, source))
		} else {
			require.NoError(t, jpeg.Encode(&data, source, nil))
		}
		meta, err := store.Import(novelID, "picture."+format, &data)
		require.NoError(t, err)
		ref := attachment.Ref{AttachmentID: meta.ID, Variant: attachment.Standard}
		encoded, info, err := store.Read(novelID, ref)
		require.NoError(t, err)
		refs = append(refs, ref)
		urls = append(urls, "data:"+info.MIMEType+";base64,"+base64.StdEncoding.EncodeToString(encoded))
	}
	return refs, urls
}

func TestVisionRequestsAcrossProtocols(t *testing.T) {
	t.Setenv("GOINK_DATA_DIR", t.TempDir())
	refs, urls := importTestImages(t, 7)
	for _, fixture := range loadSwitchFixtures(t) {
		for _, mode := range []string{"generate", "stream"} {
			for _, scenario := range []string{"mixed", "image-only", "follow-up", "tool-results"} {
				t.Run(string(fixture.Protocol)+"/"+mode+"/"+scenario, func(t *testing.T) {
					requests := make(chan map[string]any, 1)
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						require.Equal(t, http.MethodPost, r.Method)
						var wire map[string]any
						if err := json.NewDecoder(r.Body).Decode(&wire); err != nil {
							t.Error(err)
							http.Error(w, "invalid JSON", http.StatusBadRequest)
							return
						}
						requests <- wire
						if mode == "generate" {
							w.Header().Set("Content-Type", "application/json")
							_, _ = w.Write(fixture.TextResponse)
							return
						}
						w.Header().Set("Content-Type", "text/event-stream")
						for _, event := range fixture.StreamEvents {
							var compact bytes.Buffer
							if err := json.Compact(&compact, event); err != nil {
								t.Error(err)
								return
							}
							var label struct{ Type string }
							if err := json.Unmarshal(event, &label); err != nil {
								t.Error(err)
								return
							}
							if label.Type != "" {
								_, _ = fmt.Fprintf(w, "event: %s\n", label.Type)
							}
							_, _ = fmt.Fprintf(w, "data: %s\n\n", compact.Bytes())
						}
						if fixture.Protocol == adapter.ProtocolOpenAIChat {
							_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
						}
					}))
					defer server.Close()
					cfg := emptyUserConfig()
					cfg.Adapters[fixture.Protocol] = AdapterConfig{Providers: []adapter.Provider{{
						Name: "vision", ProviderType: fixture.ProviderType, APIKey: "test-key",
						ChatURL: server.URL + fixture.Path,
						Models:  []ModelInfo{{ID: "switch-model", SupportsVision: true}},
					}}}
					client, err := NewClient(cfg, slog.Default())
					require.NoError(t, err)
					history := []Message{{Role: "user", Content: "描述图片", Images: refs}}
					wantURLs := urls
					switch scenario {
					case "image-only":
						history[0].Content = ""
					case "follow-up":
						history = append(history, Message{Role: "assistant", Content: "初步描述"}, Message{Role: "user", Content: "再看一下细节"})
					case "tool-results":
						history = append(history,
							Message{Role: "assistant", ToolCalls: []ToolCall{
								{ID: "first", Name: "read", Arguments: json.RawMessage(`{}`)},
								{ID: "second", Name: "read", Arguments: json.RawMessage(`{}`)},
							}},
							Message{Role: "tool", Content: "第一张", ToolCallID: "first", ToolName: "read", Images: refs[:1]},
							Message{Role: "tool", ToolCallID: "second", ToolName: "read", Images: refs[1:]},
							Message{Role: "user", Content: "比较一下"},
						)
						wantURLs = append(append([]string(nil), urls...), urls...)
					}
					before, err := json.Marshal(history)
					require.NoError(t, err)
					opts := &CallOptions{NovelID: 7}
					var answer strings.Builder
					if mode == "generate" {
						text, err := client.GenerateText(context.Background(), fixture.Protocol, "vision", "switch-model", history, opts)
						require.NoError(t, err)
						answer.WriteString(text)
					} else {
						for event := range client.ChatStream(context.Background(), fixture.Protocol, "vision", "switch-model", history, nil, opts) {
							require.NoError(t, event.Error)
							if event.Type == EventContent {
								answer.WriteString(event.Data)
							}
						}
					}
					require.Equal(t, "续聊成功", answer.String())
					wire := <-requests
					require.Equal(t, wantURLs, imageURLs(wire))
					if scenario == "tool-results" && fixture.Protocol == adapter.ProtocolOpenAIChat {
						messages := wire["messages"].([]any)
						require.Len(t, messages, 6)
						require.Equal(t, "tool", messages[2].(map[string]any)["role"])
						require.Equal(t, "tool", messages[3].(map[string]any)["role"])
						require.Equal(t, "user", messages[4].(map[string]any)["role"])
						text := messages[4].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string)
						require.Contains(t, text, "first")
						require.Contains(t, text, "second")
					}
					after, err := json.Marshal(history)
					require.NoError(t, err)
					require.Equal(t, before, after)
					for _, message := range history {
						require.Empty(t, message.ImageInputs)
					}
				})
			}
		}
	}
}

func imageURLs(value any) []string {
	var urls []string
	switch node := value.(type) {
	case []any:
		for _, child := range node {
			urls = append(urls, imageURLs(child)...)
		}
	case map[string]any:
		switch node["type"] {
		case "image_url":
			return []string{node["image_url"].(map[string]any)["url"].(string)}
		case "input_image":
			return []string{node["image_url"].(string)}
		case "image":
			if source, ok := node["source"].(map[string]any); ok {
				return []string{"data:" + source["media_type"].(string) + ";base64," + source["data"].(string)}
			}
			return []string{"data:" + node["mime_type"].(string) + ";base64," + node["data"].(string)}
		}
		for _, child := range node {
			urls = append(urls, imageURLs(child)...)
		}
	}
	return urls
}

func TestPrepareImagesIncrementally(t *testing.T) {
	t.Setenv("GOINK_DATA_DIR", t.TempDir())
	refs, urls := importTestImages(t, 7)
	model := ModelInfo{ID: "vision", SupportsVision: true}
	history := []Message{{Role: "user", Images: refs[:1]}}
	prepared, err := PrepareImageMessages(7, model, history)
	require.NoError(t, err)
	before := prepared[0].Clone()
	require.NoError(t, os.RemoveAll(filepath.Join(config.AttachmentsDirPath(), "7", refs[0].AttachmentID)))

	again, err := PrepareImageMessages(7, model, prepared)
	require.NoError(t, err)
	require.Equal(t, prepared, again)
	require.Equal(t, urls[0], again[0].ImageInputs[0].DataURL())
	require.Empty(t, history[0].ImageInputs)

	mixed := []Message{prepared[0], {Role: "tool", Images: refs[1:]}}
	resolved, err := PrepareImageMessages(7, model, mixed)
	require.NoError(t, err)
	require.Equal(t, before, resolved[0])
	require.Equal(t, urls[1], resolved[1].ImageInputs[0].DataURL())
	require.Equal(t, before, mixed[0])
	require.Empty(t, mixed[1].ImageInputs)

	_, err = PrepareImageMessages(7, ModelInfo{ID: "text"}, prepared)
	require.ErrorContains(t, err, "不支持图片")
	_, err = PrepareImageMessages(0, model, prepared)
	require.ErrorContains(t, err, "小说 ID")
	invalid := prepared[0].Clone()
	invalid.Role = "assistant"
	_, err = PrepareImageMessages(7, model, []Message{invalid})
	require.ErrorContains(t, err, "不支持图片输入")
	invalid = prepared[0].Clone()
	invalid.ImageInputs[0].Base64 = ""
	_, err = PrepareImageMessages(7, model, []Message{invalid})
	require.ErrorContains(t, err, "缺少有效数据")
	require.Equal(t, before, prepared[0])
}

func TestPrepareImagesAndTokenBudget(t *testing.T) {
	t.Setenv("GOINK_DATA_DIR", t.TempDir())
	refs, _ := importTestImages(t, 7)
	history := []Message{{Role: "user", Images: refs}}
	model := ModelInfo{ID: "vision", SupportsVision: true}
	prepared, err := PrepareImageMessages(7, model, history)
	require.NoError(t, err)
	require.Empty(t, history[0].ImageInputs)
	tokens, err := CountMessageTokens(prepared[0])
	require.NoError(t, err)
	require.Greater(t, tokens, 0)
	prepared[0].ImageInputs[0].Base64 = strings.Repeat("a", 100_000)
	again, err := CountMessageTokens(prepared[0])
	require.NoError(t, err)
	require.Equal(t, tokens, again)
	_, err = CountMessageTokens(history[0])
	require.ErrorContains(t, err, "尚未解析")
	_, err = PrepareImageMessages(8, model, history)
	require.Error(t, err)
	_, err = PrepareImageMessages(0, model, history)
	require.ErrorContains(t, err, "小说 ID")
	_, err = PrepareImageMessages(7, ModelInfo{ID: "text"}, history)
	require.ErrorContains(t, err, "不支持图片")
	_, err = PrepareImageMessages(7, model, []Message{{Role: "assistant", Images: refs}})
	require.ErrorContains(t, err, "不支持图片输入")

	var encoded bytes.Buffer
	require.NoError(t, png.Encode(&encoded, image.NewGray(image.Rect(0, 0, 1, 1))))
	data := append(encoded.Bytes(), make([]byte, maxRequestImageBytes/2)...)
	store := attachment.NewStore(config.AttachmentsDirPath())
	meta, err := store.Import(7, "large.png", bytes.NewReader(data))
	require.NoError(t, err)
	ref := attachment.Ref{AttachmentID: meta.ID, Variant: attachment.Original}
	large := Message{Role: "user", Images: []attachment.Ref{ref}}
	preparedLarge, err := PrepareImageMessages(7, model, []Message{large})
	require.NoError(t, err)
	for _, tc := range []struct {
		name     string
		messages []Message
	}{
		{"unprepared", []Message{large, large}},
		{"prepared-first", []Message{preparedLarge[0], large}},
		{"prepared-last", []Message{large, preparedLarge[0]}},
		{"all-prepared", []Message{preparedLarge[0], preparedLarge[0]}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.messages[1].Role = "tool"
			_, err := PrepareImageMessages(7, model, tc.messages)
			require.ErrorContains(t, err, "8 MiB")
		})
	}
}

func TestVisionPreflightDoesNotSendUnsupportedOrMissingImages(t *testing.T) {
	t.Setenv("GOINK_DATA_DIR", t.TempDir())
	refs, _ := importTestImages(t, 7)
	for _, supportsVision := range []bool{false, true} {
		t.Run(fmt.Sprint(supportsVision), func(t *testing.T) {
			var attempts atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { attempts.Add(1) }))
			defer server.Close()
			cfg := emptyUserConfig()
			cfg.Adapters[adapter.ProtocolOpenAIChat] = AdapterConfig{Providers: []adapter.Provider{{
				Name: "vision", ProviderType: "openai", ChatURL: server.URL, APIKey: "key",
				Models: []ModelInfo{{ID: "test", SupportsVision: supportsVision}},
			}}}
			client, err := NewClient(cfg, slog.Default())
			require.NoError(t, err)
			messages := []Message{{Role: "user", Images: refs}}
			opts := &CallOptions{NovelID: 8}
			_, err = client.GenerateText(context.Background(), adapter.ProtocolOpenAIChat, "vision", "test", messages, opts)
			require.Error(t, err)
			var events []StreamEvent
			for event := range client.ChatStream(context.Background(), adapter.ProtocolOpenAIChat, "vision", "test", messages, nil, opts) {
				events = append(events, event)
			}
			require.Len(t, events, 1)
			require.Equal(t, EventError, events[0].Type)
			require.Error(t, events[0].Error)
			require.Zero(t, attempts.Load())
		})
	}
}
