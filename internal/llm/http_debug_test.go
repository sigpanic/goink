package llm

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/iotest"
	"time"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

type httpDebugRoundTripFunc func(*http.Request) (*http.Response, error)

func (f httpDebugRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type httpDebugTestBody struct {
	io.Reader
	reads, closes atomic.Int32
	closeError    error
}

func (b *httpDebugTestBody) Read(p []byte) (int, error) {
	b.reads.Add(1)
	return b.Reader.Read(p)
}

func (b *httpDebugTestBody) Close() error {
	b.closes.Add(1)
	return b.closeError
}

func newHTTPDebugTestTransport(t *testing.T, base http.RoundTripper) *httpDebugTransport {
	t.Helper()
	return &httpDebugTransport{base: base, dir: filepath.Join(t.TempDir(), "captures"), log: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

func httpDebugCaptureDirs(t *testing.T, root string) []string {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	var dirs []string
	for _, entry := range entries {
		if !entry.IsDir() {
			t.Fatalf("unexpected file in capture root: %s", entry.Name())
		}
		dirs = append(dirs, filepath.Join(root, entry.Name()))
	}
	return dirs
}

func readHTTPDebugMetadata(t *testing.T, dir string) (httpDebugMetadata, []byte) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "meta.json"))
	if err != nil {
		t.Fatal(err)
	}
	var metadata httpDebugMetadata
	if err := json.Unmarshal(raw, &metadata); err != nil {
		t.Fatal(err)
	}
	return metadata, raw
}

func readHTTPDebugBody(t *testing.T, dir, file string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, file))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestHTTPDebugSwitchAndClientWiring(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "not-created")
	t.Setenv("GOINK_DEBUG_DIR", dir)
	for _, value := range []string{"", "0", "false", "invalid"} {
		t.Setenv("GOINK_DEBUG", value)
		client := newHTTPClient()
		if _, ok := client.Transport.(*http.Transport); !ok || client.Timeout != 0 {
			t.Fatalf("disabled debug changed the HTTP client: %+v", client)
		}
		client.CloseIdleConnections()
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("disabled debug created its directory: %v", err)
	}
	for _, value := range []string{"1", "true"} {
		t.Setenv("GOINK_DEBUG", value)
		client := newHTTPClient()
		debug, ok := client.Transport.(*httpDebugTransport)
		if !ok || debug.dir != dir || client.Timeout != 0 {
			t.Fatalf("debug transport was not installed: %+v", client)
		}
		base, ok := debug.base.(*http.Transport)
		if !ok || base.ResponseHeaderTimeout != 10*time.Minute || base.DialContext == nil || base.Proxy == nil {
			t.Fatal("debug changed connection or timeout settings")
		}
		client.CloseIdleConnections()
	}
	t.Setenv("GOINK_DEBUG_DIR", "")
	debug := newHTTPClient().Transport.(*httpDebugTransport)
	if debug.dir != filepath.Join(os.TempDir(), fmt.Sprintf("goink-debug-test-%d", os.Getpid())) {
		t.Fatal("test binary used the real user debug directory")
	}
}

func TestHTTPDebugCapturesBodiesAndRedactsOnlyMetadata(t *testing.T) {
	const input = `{"messages":[{"content":"原始中文🙂"}],"tool_arguments":{"n":9007199254740993}}`
	const output = "event: message\r\ndata: {\"text\":\"正文🙂\"}\r\n\r\ndata: [DONE]\r\n\r\n"
	requestBody := &httpDebugTestBody{Reader: strings.NewReader(input)}
	responseBody := &httpDebugTestBody{Reader: strings.NewReader(output)}
	req, err := http.NewRequest(http.MethodPost, "https://user:password@provider.invalid/a%2Fb?route=a%2Fb&key=query%2Fsecret&tag=2&tag=1&API_KEY=second-secret", requestBody)
	if err != nil {
		t.Fatal(err)
	}
	req.ContentLength = int64(len(input))
	req.Header.Set("Authorization", "Bearer auth-secret")
	req.Header.Set("X-Goog-Api-Key", "google-secret")
	req.Header.Set("Cookie", "session=cookie-secret")
	req.Header.Set("Content-Type", "application/json")
	originalURL := req.URL.String()
	transport := newHTTPDebugTestTransport(t, httpDebugRoundTripFunc(func(sent *http.Request) (*http.Response, error) {
		if sent.URL.String() != originalURL || sent.Header.Get("Authorization") != "Bearer auth-secret" || sent.Context() != req.Context() {
			t.Error("capture changed the actual request")
		}
		body, err := io.ReadAll(sent.Body)
		if err != nil || string(body) != input {
			t.Errorf("actual request body changed: %q %v", body, err)
		}
		if err := sent.Body.Close(); err != nil {
			t.Error(err)
		}
		return &http.Response{StatusCode: 200, Proto: "HTTP/2.0", ContentLength: -1, Uncompressed: true,
			Header: http.Header{"Content-Type": {"text/event-stream"}, "Set-Cookie": {"session=response-secret"}}, Body: responseBody}, nil
	}))
	response, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	if responseBody.reads.Load() != 0 || req.Body != requestBody || req.Header.Get("X-Goog-Api-Key") != "google-secret" {
		t.Fatal("capture pre-read the response or mutated the caller")
	}
	dirs := httpDebugCaptureDirs(t, transport.dir)
	if len(dirs) != 1 {
		t.Fatalf("capture directories=%v", dirs)
	}
	dir := dirs[0]
	if got := readHTTPDebugBody(t, dir, "response.body"); got != "" {
		t.Fatalf("response was read ahead: %q", got)
	}
	raw, err := io.ReadAll(response.Body)
	if err != nil || string(raw) != output {
		t.Fatalf("response changed: %q %v", raw, err)
	}
	if err := response.Body.Close(); err != nil || responseBody.closes.Load() != 1 || requestBody.closes.Load() != 1 {
		t.Fatalf("body close behavior changed: %v", err)
	}
	if readHTTPDebugBody(t, dir, "request.body") != input || readHTTPDebugBody(t, dir, "response.body") != output {
		t.Fatal("capture changed raw body bytes")
	}
	metadata, encoded := readHTTPDebugMetadata(t, dir)
	for _, secret := range []string{"user", "password", "query%2Fsecret", "query/secret", "second-secret", "auth-secret", "google-secret", "cookie-secret", "response-secret"} {
		if strings.Contains(string(encoded), secret) {
			t.Errorf("metadata leaked credential %q", secret)
		}
	}
	if !strings.Contains(metadata.Request.URL, "route=a%2Fb&key=%5BREDACTED%5D&tag=2&tag=1") {
		t.Fatalf("metadata lost routing parameters or their order: %s", metadata.Request.URL)
	}
	if metadata.Request.Headers.Get("Authorization") != httpDebugRedacted || metadata.Response.Headers.Get("Set-Cookie") != httpDebugRedacted || response.Header.Get("Set-Cookie") != "session=response-secret" {
		t.Fatal("header redaction mutated the original response or failed")
	}
	if metadata.ID != filepath.Base(dir) || metadata.Response.StatusCode != 200 || !metadata.Response.Uncompressed || metadata.Response.Protocol != "HTTP/2.0" {
		t.Fatalf("missing HTTP metadata: %+v", metadata)
	}
	for _, body := range []httpDebugBodyState{metadata.Request.Body, metadata.Response.Body} {
		if !body.EOF || !body.Closed || !body.Complete || body.Bytes != body.CapturedBytes {
			t.Fatalf("complete body marked incomplete: %+v", body)
		}
	}
	if metadata.Request.Body.Bytes != int64(len(input)) || metadata.Response.Body.Bytes != int64(len(output)) {
		t.Fatal("body lengths were counted as characters instead of bytes")
	}
	if runtime.GOOS != "windows" {
		for _, name := range []string{"", "meta.json", "request.body", "response.body"} {
			info, err := os.Stat(filepath.Join(dir, name))
			want := os.FileMode(0600)
			if name == "" {
				want = 0700
			}
			if err != nil || info.Mode().Perm() != want {
				t.Fatalf("incorrect capture permissions for %q: %v %v", name, info, err)
			}
		}
	}
}

func TestHTTPDebugStreamsWithoutWaitingForEOFAndMarksEarlyClose(t *testing.T) {
	reader, writer := io.Pipe()
	t.Cleanup(func() { reader.Close(); writer.Close() })
	transport := newHTTPDebugTestTransport(t, httpDebugRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: reader, ContentLength: -1}, nil
	}))
	req, _ := http.NewRequest(http.MethodGet, "https://provider.invalid/stream", nil)
	ready := make(chan *http.Response, 1)
	go func() {
		response, err := transport.RoundTrip(req)
		if err != nil {
			t.Error(err)
		}
		ready <- response
	}()
	var response *http.Response
	select {
	case response = <-ready:
	case <-time.After(2 * time.Second):
		t.Fatal("RoundTrip waited for response body bytes")
	}
	const prefix = "data: first\n\n"
	written := make(chan error, 1)
	go func() { _, err := io.WriteString(writer, prefix); written <- err }()
	read := make(chan error, 1)
	go func() {
		buffer := make([]byte, len(prefix))
		_, err := io.ReadFull(response.Body, buffer)
		if err == nil && string(buffer) != prefix {
			err = fmt.Errorf("streamed bytes changed: %q", buffer)
		}
		read <- err
	}()
	select {
	case err := <-read:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("body read waited for the rest of the stream")
	}
	if err := <-written; err != nil {
		t.Fatal(err)
	}
	dir := httpDebugCaptureDirs(t, transport.dir)[0]
	if readHTTPDebugBody(t, dir, "response.body") != prefix {
		t.Fatal("stream prefix was not written before EOF")
	}
	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}
	metadata, _ := readHTTPDebugMetadata(t, dir)
	if !metadata.Response.Body.Closed || metadata.Response.Body.EOF || metadata.Response.Body.Complete || metadata.Response.Body.Bytes != int64(len(prefix)) {
		t.Fatalf("early close was marked complete: %+v", metadata.Response.Body)
	}
}

func TestHTTPDebugPreservesReadCloseAndTransportErrors(t *testing.T) {
	readError, closeError := errors.New("upstream read failed"), errors.New("upstream close failed")
	body := &httpDebugTestBody{Reader: io.MultiReader(strings.NewReader("partial"), iotest.ErrReader(readError)), closeError: closeError}
	transport := newHTTPDebugTestTransport(t, httpDebugRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: body, ContentLength: -1}, nil
	}))
	req, _ := http.NewRequest(http.MethodGet, "https://provider.invalid/stream", nil)
	response, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(response.Body)
	if !errors.Is(err, readError) || string(raw) != "partial" || !errors.Is(response.Body.Close(), closeError) {
		t.Fatal("capture replaced the original read or close error")
	}
	metadata, _ := readHTTPDebugMetadata(t, httpDebugCaptureDirs(t, transport.dir)[0])
	if metadata.Response.Body.Complete || metadata.Response.Body.ReadError != readError.Error() || metadata.Response.Body.CloseError != closeError.Error() {
		t.Fatalf("body failure not recorded: %+v", metadata.Response.Body)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	originalError := fmt.Errorf("request failed with auth-token and query-secret: %w", context.Canceled)
	transport = newHTTPDebugTestTransport(t, httpDebugRoundTripFunc(func(*http.Request) (*http.Response, error) {
		cancel()
		return nil, originalError
	}))
	req, _ = http.NewRequestWithContext(ctx, http.MethodGet, "https://provider.invalid/stream?key=query-secret", nil)
	req.Header.Set("Authorization", "Bearer auth-token")
	response, err = transport.RoundTrip(req)
	if response != nil || err != originalError || !errors.Is(err, context.Canceled) {
		t.Fatal("capture replaced the original transport error")
	}
	dir := httpDebugCaptureDirs(t, transport.dir)[0]
	metadata, encoded := readHTTPDebugMetadata(t, dir)
	if metadata.Response != nil || metadata.ContextError != context.Canceled.Error() || metadata.TransportError == "" || strings.Contains(string(encoded), "auth-token") || strings.Contains(string(encoded), "query-secret") {
		t.Fatalf("transport error was lost or leaked credentials: %s", encoded)
	}
}

func TestHTTPDebugDiskFailuresDoNotChangeRequests(t *testing.T) {
	t.Run("directory creation", func(t *testing.T) {
		var calls atomic.Int32
		transport := newHTTPDebugTestTransport(t, httpDebugRoundTripFunc(func(*http.Request) (*http.Response, error) {
			calls.Add(1)
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok"))}, nil
		}))
		if err := os.WriteFile(transport.dir, []byte("not a directory"), 0600); err != nil {
			t.Fatal(err)
		}
		req, _ := http.NewRequest(http.MethodGet, "https://provider.invalid/", nil)
		response, err := transport.RoundTrip(req)
		if err != nil || calls.Load() != 1 {
			t.Fatalf("capture failure blocked the request: %v", err)
		}
		raw, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil || string(raw) != "ok" {
			t.Fatalf("response changed after capture setup failure: %q %v", raw, err)
		}
	})
	t.Run("body write", func(t *testing.T) {
		var logs bytes.Buffer
		transport := newHTTPDebugTestTransport(t, httpDebugRoundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("still readable")), ContentLength: -1}, nil
		}))
		transport.log = slog.New(slog.NewTextHandler(&logs, nil))
		req, _ := http.NewRequest(http.MethodGet, "https://provider.invalid/", nil)
		response, err := transport.RoundTrip(req)
		if err != nil {
			t.Fatal(err)
		}
		capture := response.Body.(*httpDebugReadCloser).capture
		capture.response.file.Close()
		raw, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil || string(raw) != "still readable" {
			t.Fatalf("disk failure replaced response bytes: %q %v", raw, err)
		}
		metadata, _ := readHTTPDebugMetadata(t, capture.dir)
		if metadata.CaptureError == "" || metadata.Response.Body.CaptureError == "" || metadata.Response.Body.Complete || metadata.Response.Body.CapturedBytes != 0 || metadata.Response.Body.Bytes != int64(len(raw)) || !strings.Contains(logs.String(), "capture failed") {
			t.Fatalf("failed capture was marked complete: %+v", metadata)
		}
	})
}

func TestHTTPDebugConcurrentRequestsHaveSeparateCaptures(t *testing.T) {
	transport := newHTTPDebugTestTransport(t, httpDebugRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		raw, err := io.ReadAll(req.Body)
		req.Body.Close()
		if err != nil {
			return nil, err
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(raw)), ContentLength: int64(len(raw))}, nil
	}))
	var group sync.WaitGroup
	for i := range 12 {
		group.Go(func() {
			input := fmt.Sprintf("request %d", i)
			req, _ := http.NewRequest(http.MethodPost, "https://provider.invalid/", strings.NewReader(input))
			response, err := transport.RoundTrip(req)
			if err != nil {
				t.Error(err)
				return
			}
			raw, err := io.ReadAll(response.Body)
			response.Body.Close()
			if err != nil || string(raw) != input {
				t.Errorf("concurrent response changed: %q %v", raw, err)
			}
		})
	}
	group.Wait()
	dirs := httpDebugCaptureDirs(t, transport.dir)
	if len(dirs) != 12 {
		t.Fatalf("got %d capture directories", len(dirs))
	}
	seen := map[string]bool{}
	for _, dir := range dirs {
		request := readHTTPDebugBody(t, dir, "request.body")
		response := readHTTPDebugBody(t, dir, "response.body")
		metadata, _ := readHTTPDebugMetadata(t, dir)
		if request != response || seen[request] || !metadata.Request.Body.Complete || !metadata.Response.Body.Complete {
			t.Fatalf("captures were mixed or incomplete: %q %q %+v", request, response, metadata)
		}
		seen[request] = true
	}
}

func TestHTTPDebugCapturesActualOpenAIChatStream(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GOINK_DEBUG", "1")
	t.Setenv("GOINK_DEBUG_DIR", dir)
	const wire = "data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\ndata: [DONE]\n\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("debug redacted the transmitted API key")
		}
		io.Copy(io.Discard, req.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, wire)
	}))
	defer server.Close()
	client, err := NewClient(testUserConfig(adapter.Provider{Name: "debug", ProviderType: "openai", ChatURL: server.URL, APIKey: "test-key"}), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.http.CloseIdleConnections()
	var text string
	for event := range client.ChatStream(context.Background(), adapter.ProtocolOpenAIChat, "debug", "model", []Message{{Role: "user", Content: "hi"}}, nil, nil) {
		if event.Type == EventError {
			t.Fatal(event.Error)
		}
		text += event.Data
	}
	dirs := httpDebugCaptureDirs(t, dir)
	if len(dirs) != 1 || text != "hello" || readHTTPDebugBody(t, dirs[0], "response.body") != wire {
		t.Fatalf("OpenAI Chat stream or capture changed: text=%q dirs=%v", text, dirs)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(readHTTPDebugBody(t, dirs[0], "request.body")), &payload); err != nil || payload["model"] != "model" || payload["stream"] != true {
		t.Fatalf("did not capture the serialized provider request: %+v %v", payload, err)
	}
}

func TestHTTPDebugCapturesSDKResponsesWithoutReencoding(t *testing.T) {
	for _, tc := range []struct {
		protocol                        adapter.Protocol
		providerType, model, body, text string
	}{
		{adapter.ProtocolOpenAIResponses, "openai", "relay-model", rootResponsesResult, "responses"},
		{adapter.ProtocolAnthropicMessages, "anthropic", "relay-model", `{"id":"msg_test","type":"message","role":"assistant","model":"relay-model","content":[{"type":"text","text":"claude"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`, "claude"},
		{adapter.ProtocolGeminiInteractions, "google", "gemini-3.1-flash-lite", `{"object":"interaction","status":"completed","model":"gemini-3.1-flash-lite","steps":[{"type":"model_output","content":[{"type":"text","text":"gemini"}]}]}`, "gemini"},
	} {
		t.Run(string(tc.protocol), func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("GOINK_DEBUG", "1")
			t.Setenv("GOINK_DEBUG_DIR", dir)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				io.Copy(io.Discard, req.Body)
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, tc.body)
			}))
			defer server.Close()
			config := emptyUserConfig()
			config.Adapters[tc.protocol] = AdapterConfig{Providers: []adapter.Provider{{Name: "debug", ProviderType: tc.providerType, ChatURL: server.URL, APIKey: "sdk-key"}}}
			client, err := NewClient(config, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer client.http.CloseIdleConnections()
			text, err := client.GenerateText(context.Background(), tc.protocol, "debug", tc.model, []Message{{Role: "user", Content: "hi"}}, nil)
			if err != nil || text != tc.text {
				t.Fatalf("SDK generation changed: %q %v", text, err)
			}
			dirs := httpDebugCaptureDirs(t, dir)
			if len(dirs) != 1 || readHTTPDebugBody(t, dirs[0], "response.body") != tc.body {
				t.Fatal("SDK decoding changed or duplicated captured response bytes")
			}
			metadata, _ := readHTTPDebugMetadata(t, dirs[0])
			if !metadata.Response.Body.Complete || metadata.Response.Body.Bytes != int64(len(tc.body)) || metadata.Response.ReceivedAt.Before(metadata.StartedAt) {
				t.Fatalf("SDK response capture incomplete: %+v", metadata)
			}
		})
	}
}

func TestHTTPDebugCapturesHTTPDecompressedBodies(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GOINK_DEBUG", "1")
	t.Setenv("GOINK_DEBUG_DIR", dir)
	const body = "data: 压缩正文🙂\n\ndata: [DONE]\n\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get("Accept-Encoding") != "gzip" {
			t.Error("transport did not negotiate HTTP decompression")
		}
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Content-Type", "text/event-stream")
		compressed := gzip.NewWriter(w)
		io.WriteString(compressed, body)
		compressed.Close()
	}))
	defer server.Close()
	client := newHTTPClient()
	defer client.CloseIdleConnections()
	response, err := client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || string(raw) != body {
		t.Fatalf("gzip response changed: %q %v", raw, err)
	}
	capture := httpDebugCaptureDirs(t, dir)[0]
	metadata, _ := readHTTPDebugMetadata(t, capture)
	if readHTTPDebugBody(t, capture, "response.body") != body || !metadata.Response.Uncompressed || !metadata.Response.Body.Complete {
		t.Fatalf("did not capture decompressed SSE bytes: %+v", metadata)
	}
}

func TestHTTPDebugCancellationUnblocksRead(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GOINK_DEBUG", "1")
	t.Setenv("GOINK_DEBUG_DIR", dir)
	serverClosed := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: first\n\n")
		w.(http.Flusher).Flush()
		<-req.Context().Done()
		close(serverClosed)
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client := newHTTPClient()
	defer client.CloseIdleConnections()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	buffer := make([]byte, len("data: first\n\n"))
	if _, err := io.ReadFull(response.Body, buffer); err != nil {
		t.Fatal(err)
	}
	readDone := make(chan error, 1)
	go func() { _, err := response.Body.Read(make([]byte, 1)); readDone <- err }()
	cancel()
	select {
	case err := <-readDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation chain changed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("capture blocked cancellation")
	}
	response.Body.Close()
	select {
	case <-serverClosed:
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled request did not close server connection")
	}
	metadata, _ := readHTTPDebugMetadata(t, httpDebugCaptureDirs(t, dir)[0])
	if metadata.ContextError != context.Canceled.Error() || metadata.Response.Body.Complete || metadata.Response.Body.ReadError == "" {
		t.Fatalf("cancellation not recorded: %+v", metadata)
	}
}

type httpDebugIdleTransport struct {
	closed bool
}

func (*httpDebugIdleTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("unexpected request")
}

func (t *httpDebugIdleTransport) CloseIdleConnections() { t.closed = true }

func TestHTTPDebugForwardsIdleConnectionCleanup(t *testing.T) {
	base := &httpDebugIdleTransport{}
	client := &http.Client{Transport: newHTTPDebugTestTransport(t, base)}
	client.CloseIdleConnections()
	if !base.closed {
		t.Fatal("debug wrapper hid CloseIdleConnections")
	}
}
