package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const httpDebugRedacted = "[REDACTED]"

type httpDebugTransport struct {
	base http.RoundTripper
	dir  string
	log  *slog.Logger
}

type httpDebugBodyState struct {
	Bytes         int64  `json:"bytes"`
	CapturedBytes int64  `json:"captured_bytes"`
	EOF           bool   `json:"eof"`
	Closed        bool   `json:"closed"`
	Complete      bool   `json:"complete"` // 仅表示 HTTP 正文完整落盘，不代表模型协议已成功完成。
	ReadError     string `json:"read_error,omitempty"`
	CloseError    string `json:"close_error,omitempty"`
	CaptureError  string `json:"capture_error,omitempty"`
}

type httpDebugMessage struct {
	Headers       http.Header        `json:"headers"`
	ContentLength int64              `json:"content_length"`
	Body          httpDebugBodyState `json:"body"`
}

type httpDebugResponse struct {
	httpDebugMessage
	ReceivedAt   time.Time `json:"received_at"`
	StatusCode   int       `json:"status_code"`
	Protocol     string    `json:"protocol"`
	Uncompressed bool      `json:"uncompressed"`
}

type httpDebugMetadata struct {
	ID        string    `json:"id"`
	StartedAt time.Time `json:"started_at"`
	UpdatedAt time.Time `json:"updated_at"`
	ElapsedMS int64     `json:"elapsed_ms"`
	Request   struct {
		httpDebugMessage
		Method string `json:"method"`
		URL    string `json:"url"`
		Host   string `json:"host"`
	} `json:"request"`
	Response       *httpDebugResponse `json:"response,omitempty"`
	TransportError string             `json:"transport_error,omitempty"`
	ContextError   string             `json:"context_error,omitempty"`
	CaptureError   string             `json:"capture_error,omitempty"`
}

type httpDebugBodyCapture struct {
	file   *os.File
	active int
}

type httpDebugCapture struct {
	mu       sync.Mutex
	dir      string
	log      *slog.Logger
	ctx      context.Context
	metadata httpDebugMetadata
	request  httpDebugBodyCapture
	response httpDebugBodyCapture
	secrets  []string
}

func withHTTPDebug(base http.RoundTripper) http.RoundTripper {
	enabled, _ := strconv.ParseBool(strings.TrimSpace(os.Getenv("GOINK_DEBUG")))
	if !enabled {
		return base
	}
	dir := os.Getenv("GOINK_DEBUG_DIR")
	if dir == "" {
		// 测试进程不向真实用户目录写入请求和响应；显式目录由测试自行清理。
		if testing.Testing() {
			dir = filepath.Join(os.TempDir(), fmt.Sprintf("goink-debug-test-%d", os.Getpid()))
		} else {
			home, err := os.UserHomeDir()
			if err != nil {
				slog.Warn("cannot enable LLM HTTP debug capture", "err", err)
				return base
			}
			dir = filepath.Join(home, "Goink_debug")
		}
	}
	return &httpDebugTransport{base: base, dir: dir, log: slog.Default()}
}

func (t *httpDebugTransport) CloseIdleConnections() {
	if closer, ok := t.base.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}

func (t *httpDebugTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	capture, err := t.newCapture(req)
	if err != nil {
		t.log.Warn("cannot create LLM HTTP debug capture", "err", err)
		return t.base.RoundTrip(req)
	}
	t.log.Debug("LLM HTTP debug capture", "dir", capture.dir)
	request := req.Clone(req.Context())
	if req.Body != nil && req.Body != http.NoBody {
		request.Body = &httpDebugReadCloser{ReadCloser: req.Body, capture: capture, request: true}
	}
	response, err := t.base.RoundTrip(request)
	capture.mu.Lock()
	defer capture.mu.Unlock()
	if err != nil {
		capture.metadata.TransportError = capture.errorText(err)
		capture.closeFile(&capture.request, &capture.metadata.Request.httpDebugMessage)
		capture.closeFile(&capture.response, nil)
	} else if response != nil {
		capture.metadata.Response = &httpDebugResponse{
			httpDebugMessage: httpDebugMessage{Headers: capture.redactHeaders(response.Header), ContentLength: response.ContentLength},
			ReceivedAt:       time.Now(),
			StatusCode:       response.StatusCode, Protocol: response.Proto, Uncompressed: response.Uncompressed,
		}
		if response.Body == nil || response.Body == http.NoBody {
			capture.metadata.Response.Body.EOF = true
			capture.closeFile(&capture.response, &capture.metadata.Response.httpDebugMessage)
		} else {
			response.Body = &httpDebugReadCloser{ReadCloser: response.Body, capture: capture}
		}
	} else {
		capture.closeFile(&capture.request, &capture.metadata.Request.httpDebugMessage)
		capture.closeFile(&capture.response, nil)
	}
	_ = capture.saveMetadata()
	return response, err
}

func (t *httpDebugTransport) newCapture(req *http.Request) (*httpDebugCapture, error) {
	if err := os.MkdirAll(t.dir, 0700); err != nil {
		return nil, err
	}
	started := time.Now()
	dir, err := os.MkdirTemp(t.dir, started.Format("20060102-150405.000000000")+"-")
	if err != nil {
		return nil, err
	}
	c := &httpDebugCapture{dir: dir, log: t.log, ctx: req.Context()}
	cleanup := func() {
		if c.request.file != nil {
			_ = c.request.file.Close()
		}
		if c.response.file != nil {
			_ = c.response.file.Close()
		}
		_ = os.RemoveAll(dir)
	}
	c.request.file, err = os.OpenFile(filepath.Join(dir, "request.body"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err == nil {
		c.response.file, err = os.OpenFile(filepath.Join(dir, "response.body"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	}
	if err != nil {
		cleanup()
		return nil, err
	}
	c.metadata.ID, c.metadata.StartedAt = filepath.Base(dir), started
	c.metadata.Request.Method, c.metadata.Request.Host = req.Method, req.Host
	c.metadata.Request.URL = c.redactURL(req.URL)
	c.metadata.Request.Headers = c.redactHeaders(req.Header)
	c.metadata.Request.ContentLength = req.ContentLength
	if req.Body == nil || req.Body == http.NoBody {
		c.metadata.Request.Body.EOF = true
		c.closeFile(&c.request, &c.metadata.Request.httpDebugMessage)
	}
	if err := c.saveMetadata(); err != nil {
		cleanup()
		return nil, err
	}
	return c, nil
}

type httpDebugReadCloser struct {
	io.ReadCloser
	capture *httpDebugCapture
	request bool
}

func (b *httpDebugReadCloser) body() (*httpDebugBodyCapture, *httpDebugMessage) {
	if b.request {
		return &b.capture.request, &b.capture.metadata.Request.httpDebugMessage
	}
	return &b.capture.response, &b.capture.metadata.Response.httpDebugMessage
}

// 仅复制调用方实际读取的字节；HTTP 自动解压时，记录解压后的响应正文。
func (b *httpDebugReadCloser) Read(p []byte) (int, error) {
	c := b.capture
	c.mu.Lock()
	body, _ := b.body()
	body.active++
	c.mu.Unlock()
	n, readErr := b.ReadCloser.Read(p)
	c.mu.Lock()
	defer c.mu.Unlock()
	body, message := b.body()
	body.active--
	message.Body.Bytes += int64(n)
	if n > 0 && body.file != nil {
		written, err := body.file.Write(p[:n])
		message.Body.CapturedBytes += int64(written)
		if err == nil && written != n {
			err = io.ErrShortWrite
		}
		if err != nil {
			message.Body.CaptureError = err.Error()
			c.captureError(err)
			c.closeFile(body, message)
		}
	}
	if errors.Is(readErr, io.EOF) {
		message.Body.EOF = true
	} else if readErr != nil {
		message.Body.ReadError = c.errorText(readErr)
	}
	if readErr != nil || message.Body.Closed {
		c.closeFile(body, message)
		_ = c.saveMetadata()
	}
	return n, readErr
}

func (b *httpDebugReadCloser) Close() error {
	err := b.ReadCloser.Close()
	c := b.capture
	c.mu.Lock()
	defer c.mu.Unlock()
	body, message := b.body()
	message.Body.Closed = true
	if err != nil {
		message.Body.CloseError = c.errorText(err)
	}
	c.closeFile(body, message)
	_ = c.saveMetadata()
	return err
}

func (c *httpDebugCapture) closeFile(body *httpDebugBodyCapture, message *httpDebugMessage) {
	if body.active != 0 {
		return
	}
	if body.file != nil {
		if err := body.file.Close(); err != nil {
			if message != nil {
				message.Body.CaptureError = err.Error()
			}
			c.captureError(err)
		}
		body.file = nil
	}
	if message != nil {
		state := &message.Body
		state.Complete = (state.EOF || (message.ContentLength > 0 && state.Bytes == message.ContentLength)) &&
			state.CapturedBytes == state.Bytes && state.ReadError == "" && state.CaptureError == ""
	}
}

func (c *httpDebugCapture) captureError(err error) {
	if c.metadata.CaptureError == "" {
		c.metadata.CaptureError = err.Error()
		c.log.Warn("LLM HTTP debug capture failed", "dir", c.dir, "err", err)
	}
}

// 调用方持有 mu；原子替换让进行中的抓包也始终保有可读取的元信息。
func (c *httpDebugCapture) saveMetadata() error {
	c.metadata.UpdatedAt = time.Now()
	c.metadata.ElapsedMS = time.Since(c.metadata.StartedAt).Milliseconds()
	if err := c.ctx.Err(); err != nil {
		c.metadata.ContextError = c.errorText(err)
	}
	raw, err := json.MarshalIndent(c.metadata, "", "  ")
	if err == nil {
		err = os.WriteFile(filepath.Join(c.dir, "meta.json.tmp"), raw, 0600)
	}
	if err == nil {
		err = os.Rename(filepath.Join(c.dir, "meta.json.tmp"), filepath.Join(c.dir, "meta.json"))
	}
	if err != nil {
		c.captureError(err)
	}
	return err
}

func httpDebugSensitive(name string) bool {
	name = strings.NewReplacer("-", "", "_", "").Replace(strings.ToLower(name))
	switch name {
	case "authorization", "proxyauthorization", "cookie", "setcookie", "xapikey", "xgoogapikey",
		"apikey", "key", "token", "accesstoken", "refreshtoken", "idtoken", "signature", "sig":
		return true
	default:
		return false
	}
}

func (c *httpDebugCapture) redactHeaders(headers http.Header) http.Header {
	copy := headers.Clone()
	for name, values := range copy {
		if httpDebugSensitive(name) {
			for i, value := range values {
				c.secrets = append(c.secrets, value)
				if strings.EqualFold(name, "Authorization") || strings.EqualFold(name, "Proxy-Authorization") {
					if _, token, ok := strings.Cut(value, " "); ok {
						c.secrets = append(c.secrets, token)
					}
				}
				values[i] = httpDebugRedacted
			}
		}
	}
	return copy
}

func (c *httpDebugCapture) redactURL(original *url.URL) string {
	copy := *original
	if copy.User != nil {
		c.secrets = append(c.secrets, copy.User.String(), copy.User.Username())
		if password, ok := copy.User.Password(); ok {
			c.secrets = append(c.secrets, password)
		}
		copy.User = url.User(httpDebugRedacted)
	}
	parts := strings.Split(copy.RawQuery, "&")
	for i, part := range parts {
		name, value, _ := strings.Cut(part, "=")
		decoded, err := url.QueryUnescape(name)
		if err != nil || httpDebugSensitive(decoded) {
			c.secrets = append(c.secrets, value)
			if decodedValue, err := url.QueryUnescape(value); err == nil {
				c.secrets = append(c.secrets, decodedValue)
			}
			parts[i] = name + "=" + url.QueryEscape(httpDebugRedacted)
		}
	}
	copy.RawQuery = strings.Join(parts, "&")
	copy.Fragment = ""
	return copy.String()
}

func (c *httpDebugCapture) errorText(err error) string {
	text := err.Error()
	sort.Slice(c.secrets, func(i, j int) bool { return len(c.secrets[i]) > len(c.secrets[j]) })
	for _, secret := range c.secrets {
		if secret != "" {
			text = strings.ReplaceAll(text, secret, httpDebugRedacted)
		}
	}
	return text
}
