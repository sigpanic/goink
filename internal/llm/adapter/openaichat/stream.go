package openaichat

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"syscall"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

// Stream 转换一次 Goink 请求并产出统一事件。
func (c *Adapter) Stream(ctx context.Context, input adapter.Request) <-chan StreamEvent {
	model, opts := input.Model, input.Options
	ch := make(chan StreamEvent, 32)
	go func() {
		defer close(ch)

		p, err := c.provider(input.ProviderName)
		if err != nil {
			adapter.SendEvent(ctx, ch, StreamEvent{Type: EventError, Error: err})
			return
		}
		messages, err := c.buildMessages(input.Messages)
		if err != nil {
			adapter.SendEvent(ctx, ch, StreamEvent{Type: EventError, Error: err})
			return
		}
		tools, err := buildTools(input.Tools)
		if err != nil {
			adapter.SendEvent(ctx, ch, StreamEvent{Type: EventError, Error: err})
			return
		}
		payload := c.buildPayload(&p, messages, tools, model, opts, true)

		body, err := json.Marshal(payload)
		if err != nil {
			if !adapter.SendEvent(ctx, ch, StreamEvent{Type: EventError, Error: fmt.Errorf("failed to marshal request body: %w", err)}) {
				return
			}
			return
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.ChatURL, bytes.NewReader(body))
		if err != nil {
			if !adapter.SendEvent(ctx, ch, StreamEvent{Type: EventError, Error: fmt.Errorf("failed to create HTTP request: %w", err)}) {
				return
			}
			return
		}

		// 组装请求头
		headers := map[string]string{
			"Content-Type":  "application/json",
			"Authorization": "Bearer " + p.APIKey,
		}
		if p.BuildHeaders != nil {
			headers = p.BuildHeaders(opts, headers)
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}

		resp, err := c.http.Do(req)
		if err != nil {
			// 判断是否首字节超时（ResponseHeaderTimeout 触发）
			isTimeout := false
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				isTimeout = true
			}

			msg := "网络连接失败，请检查网络后重试"
			retryable := true
			if isTimeout {
				msg = "服务器响应超时（首字节超过 10m），请稍后重试"
				c.logger.Warn("llm first byte timeout", "err", err, "url", req.URL.String())
			} else {
				// 非超时网络错误：区分永久性（DNS NXDOMAIN / 连接拒绝）与瞬时性（EOF / reset）
				var dnsErr *net.DNSError
				var opErr *net.OpError
				if (errors.As(err, &dnsErr) && dnsErr.IsNotFound) ||
					(errors.As(err, &opErr) && errors.Is(opErr.Err, syscall.ECONNREFUSED)) {
					retryable = false
					msg = "网络连接被拒绝或域名无法解析，请检查网络配置"
				}
				c.logger.Warn("llm network error", "err", err, "url", req.URL.String(), "retryable", retryable)
			}

			if !adapter.SendEvent(ctx, ch, StreamEvent{Type: EventError, Error: &APIError{
				StatusCode: 0,
				Message:    msg,
				Retryable:  retryable,
				Cause:      err,
			}}) {
				return
			}
			return
		}
		defer resp.Body.Close()

		// HTTP 错误
		if resp.StatusCode >= 400 {
			errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 8*1024))
			// 关键节点日志：HTTP 4xx/5xx
			bodyStr := string(errBody)
			if len(bodyStr) > 500 {
				bodyStr = bodyStr[:500]
			}
			c.logger.Warn("llm http error",
				"status", resp.StatusCode,
				"body", bodyStr,
				"url", req.URL.String())
			responseErr := parseDefaultError(errBody)
			if p.ParseError != nil {
				responseErr = p.ParseError(errBody)
			}
			msg := responseErr.Error()
			// 解析 Retry-After header（HTTP 429/503 等场景）
			// 支持两种格式：秒数（"120"）或 HTTP-date（"Wed, 21 Oct 2026 07:28:00 GMT"）
			retryAfter := adapter.ParseRetryAfter(resp.Header.Get("Retry-After"))
			if !adapter.SendEvent(ctx, ch, StreamEvent{Type: EventError, Error: &APIError{
				StatusCode: resp.StatusCode,
				Message:    msg,
				Retryable:  adapter.StatusRetryable(resp.StatusCode),
				RetryAfter: retryAfter,
				Cause:      responseErr,
			}}) {
				return
			}
			return
		}

		// SSE 逐行解析
		c.parseSSE(ctx, ch, resp, p, model)
	}()

	return ch
}

// buildPayload 组装 LLM API 请求体。
func (c *Adapter) buildPayload(
	p *Provider,
	messages []map[string]any,
	tools []map[string]any,
	model string,
	opts *CallOptions,
	stream bool,
) map[string]any {
	payload := map[string]any{
		"model":    model,
		"messages": messages,
		"stream":   stream,
	}

	if stream {
		payload["stream_options"] = map[string]any{"include_usage": true}
	}

	if len(tools) > 0 {
		payload["tools"] = tools
	}
	if opts != nil && opts.ToolChoice != nil {
		payload["tool_choice"] = opts.ToolChoice
	}

	// 保留旧 GenerateText 无显式选项时的最小请求；显式选项复用流式参数规则。
	if stream || opts != nil {
		// 从 ModelInfo 取模型默认值
		var um *ModelInfo
		for i := range p.Models {
			if p.Models[i].ID == model {
				um = &p.Models[i]
				break
			}
		}

		temperature := 0.7
		if p.Temperature != nil {
			temperature = *p.Temperature
		}
		// 自定义模型未配 MaxOutputTokens 时的安全默认值：64K。
		// 足够 thinking 模型的推理 + 输出，又不至于超出多数模型限制触发 400。
		maxTokens := 64000
		if opts != nil && opts.Temperature != nil {
			temperature = *opts.Temperature
		}
		if opts != nil && opts.MaxTokens != nil {
			maxTokens = *opts.MaxTokens
		} else if um != nil && um.MaxOutputTokens > 0 {
			maxTokens = um.MaxOutputTokens
		}
		payload["temperature"] = temperature
		payload["max_tokens"] = maxTokens

		// 推理/思考参数：支持思考的模型默认开启，opts 可覆盖
		thinkingEnabled := false
		reasoningEffort := ""
		if um != nil && um.SupportsThinking {
			thinkingEnabled = true
			if len(um.ReasoningLevels) > 0 {
				reasoningEffort = um.ReasoningLevels[0]
			}
		}
		if opts != nil {
			if opts.ThinkingEnabled != nil {
				thinkingEnabled = *opts.ThinkingEnabled
			}
			if opts.ReasoningEffort != nil {
				reasoningEffort = *opts.ReasoningEffort
			}
		}

		if thinkingEnabled {
			payload["thinking"] = map[string]string{"type": "enabled"}
			if reasoningEffort != "" {
				payload["reasoning_effort"] = reasoningEffort
			}
		}
	}

	// Provider 钩子改造
	if p.BuildRequest != nil {
		payload = p.BuildRequest(payload)
	}

	return payload
}

// parseSSE 解析 SSE 流，产出 StreamEvent。
func (c *Adapter) parseSSE(ctx context.Context, ch chan<- StreamEvent, resp *http.Response, p Provider, model string) {
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024) // 行最长 2MB

	// 工具调用累积缓冲区：按 index 对齐
	type accToolCall struct {
		id        string
		name      string
		arguments strings.Builder
	}
	accumulated := make([]accToolCall, 0, 4)
	var diag sseDiagnostics // 诊断追踪（hasContent/hasToolCalls/hasReason/headLines/tailLines 等）
	var stopReason string
	// usage 缓冲（diag.lastUsage）：部分 provider（如硅基流动）在每个 SSE chunk 都带 usage，
	// 而非只在末尾。这里只保留最后一个，流结束后统一发一次，避免每 chunk
	// 触发一次 DB 持久化（tokens.go updateUsage）和前端事件刷屏。

	for scanner.Scan() {
		if ctx.Err() != nil {
			return
		}
		line := scanner.Text()

		// 累积原始行样本用于 empty 诊断（前 10 行 + 后 10 行环形）
		if len(diag.headLines) < 10 {
			diag.headLines = append(diag.headLines, line)
		}
		diag.tailLines = append(diag.tailLines, line)
		if len(diag.tailLines) > 10 {
			diag.tailLines = diag.tailLines[1:]
		}

		// SSE data 行
		const prefix = "data: "
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		diag.dataLineCount++
		data := line[len(prefix):]

		// 流结束标记
		if data == "[DONE]" {
			diag.gotDone = true
			break
		}

		// 解析 JSON chunk
		var chunk map[string]any
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			c.logger.Warn("SSE JSON parse failed", "line", data, "error", err)
			continue
		}

		// 提取 usage：标准 provider 只在末尾 chunk 发一次，硅基流动等会每个 chunk 都发。
		// 只缓存最后一个，流结束后统一发一次（见循环外）。
		if usage, ok := chunk["usage"].(map[string]any); ok && usage != nil {
			diag.lastUsage = usage
		}

		// 提取 choices
		choices, ok := chunk["choices"].([]any)
		if !ok || len(choices) == 0 {
			continue
		}
		choice, ok := choices[0].(map[string]any)
		if !ok {
			continue
		}
		if reason, ok := choice["finish_reason"].(string); ok && reason != "" {
			stopReason = reason
		}
		delta, ok := choice["delta"].(map[string]any)
		if !ok {
			continue
		}

		// reasoning_content → EventThinking
		if reasoning, ok := delta["reasoning_content"].(string); ok && reasoning != "" {
			if !adapter.SendEvent(ctx, ch, StreamEvent{Type: EventThinking, Data: reasoning}) {
				return
			}
			diag.hasReason = true
		}

		// content → EventContent
		if content, ok := delta["content"].(string); ok && content != "" {
			if !adapter.SendEvent(ctx, ch, StreamEvent{Type: EventContent, Data: content}) {
				return
			}
			diag.hasContent = true
		}

		// tool_calls delta → 按 index 累积
		toolCalls, ok := delta["tool_calls"].([]any)
		if !ok {
			continue
		}
		for _, tcRaw := range toolCalls {
			tc, _ := tcRaw.(map[string]any)
			if tc == nil {
				continue
			}
			idxFloat, ok := tc["index"].(float64)
			if !ok || idxFloat < 0 {
				c.logger.Warn("tool call delta missing valid index", "delta", tc)
				continue
			}
			idx := int(idxFloat)

			// 扩展累积缓冲区
			for len(accumulated) <= idx {
				accumulated = append(accumulated, accToolCall{})
			}
			acc := &accumulated[idx]

			// ID（首次出现）
			if id, ok := tc["id"].(string); ok && id != "" && acc.id == "" {
				acc.id = id
				if !adapter.SendEvent(ctx, ch, StreamEvent{
					Type:  EventToolCallStart,
					Delta: &ToolCallDelta{ToolID: id},
				}) {
					return
				}
			}

			// function 子对象
			fn, ok := tc["function"].(map[string]any)
			if !ok {
				continue
			}

			// 名称（首次出现）
			if name, ok := fn["name"].(string); ok && name != "" && acc.name == "" {
				acc.name = name
				if !adapter.SendEvent(ctx, ch, StreamEvent{
					Type:  EventToolCallStart,
					Delta: &ToolCallDelta{ToolName: name, ToolID: acc.id},
				}) {
					return
				}
			}

			// arguments 增量追加
			if args, ok := fn["arguments"].(string); ok && args != "" {
				acc.arguments.WriteString(args)
				if !adapter.SendEvent(ctx, ch, StreamEvent{
					Type: EventToolCallArguments,
					Delta: &ToolCallDelta{
						ToolName:      acc.name,
						ToolID:        acc.id,
						ArgumentsText: acc.arguments.String(),
					},
				}) {
					return
				}
			}
		}
	}

	if ctx.Err() != nil {
		return
	}

	if err := scanner.Err(); err != nil {
		c.logger.Warn("sse stream interrupted", "err", err)
		if !adapter.SendEvent(ctx, ch, StreamEvent{Type: EventError, Error: &APIError{
			StatusCode: 0,
			Message:    fmt.Sprintf("SSE stream read error: %s", err),
			Retryable:  true,
			Cause:      err,
		}}) {
			return
		}
		// 流异常中断：不再做 tool call 收尾，避免与 invalidToolErrors 分支叠加
		// 产生第二个 EventError（scanner.Err 后 accumulated 数据不可信，保守丢弃）
		return
	}

	// 流正常结束：发送本次 stream 的最终 usage（只发一次，见循环内缓冲说明）。
	// 放在 tool call 收尾之前，与标准 provider「末尾 chunk 发 usage」的时序一致。
	if diag.lastUsage != nil {
		if !adapter.SendEvent(ctx, ch, StreamEvent{Type: EventUsage, Usage: diag.lastUsage}) {
			return
		}
	}

	// 流结束后，发送完整工具调用。参数保留原始 JSON，由 Registry 按目标类型反序列化。
	// 按 id 去重：DeepSeek 偶发会在不同 index 上发相同 id 的 tool_call，
	// 这里保留首次出现的 slot，跳过后续重复 id，避免主 agent 重复执行 tool（特别是 run_subagent 等耗时工具）。
	//
	// 全丢弃策略：如果本轮有任意 tool arguments 解析失败，不发任何 EventToolCallEnd
	// （成功的 tool call 也一起丢弃），直接发 EventError（Kind="tool_args_invalid"）。
	// agent 层收到后丢弃 responseBuffer/thinkingBuffer + appendMsg reminder + 走自动重试。
	// 这避免 "部分 tool call 执行了，部分没有" 导致的 assistant.tool_calls 协议矛盾。
	type validToolCall struct {
		idx int
		raw string
	}
	var validToolCalls []validToolCall
	var invalidToolErrors []string

	seenToolIDs := make(map[string]bool, len(accumulated))
	for i := range accumulated {
		acc := &accumulated[i]
		if acc.name == "" || acc.arguments.Len() == 0 {
			continue
		}
		// 按 id 去重（空 id 不去重，理论上不应出现）
		if acc.id != "" {
			if seenToolIDs[acc.id] {
				c.logger.Warn("duplicate tool_call_id from provider, skipping",
					"tool", acc.name, "tool_call_id", acc.id, "index", i)
				continue
			}
			seenToolIDs[acc.id] = true
		}
		raw := acc.arguments.String()
		if !json.Valid([]byte(raw)) {
			// 先 valid，不通过再 unmarshal 拿具体错误信息
			var invalidErr error
			var probe any
			if invalidErr = json.Unmarshal([]byte(raw), &probe); invalidErr == nil {
				// 理论不会走到这里（Valid 已返回 false），防御性代码
				invalidErr = fmt.Errorf("invalid JSON")
			}
			c.logger.Warn("tool arguments JSON invalid", "tool", acc.name, "raw", raw, "err", invalidErr)
			invalidToolErrors = append(invalidToolErrors, fmt.Sprintf("tool=%s: %s", acc.name, invalidErr.Error()))
			continue
		}
		validToolCalls = append(validToolCalls, validToolCall{idx: i, raw: raw})
	}

	// tool arguments 解析失败：全丢弃（不发送成功的 tool call），发 EventError（tool_args_invalid）
	if len(invalidToolErrors) > 0 {
		errMsg := buildToolArgsInvalidMsg(invalidToolErrors)
		if !adapter.SendEvent(ctx, ch, StreamEvent{Type: EventError, Error: &APIError{
			StatusCode: 0,
			Message:    errMsg,
			Retryable:  true,
			Kind:       "tool_args_invalid",
		}}) {
			return
		}
	} else {
		// 终态可由 finish_reason 或 [DONE] 确认；无终态 EOF 沿用旧行为，但不发布完整 State。
		if (stopReason != "" || diag.gotDone) && (diag.hasContent || len(validToolCalls) > 0) {
			if !adapter.SendEvent(ctx, ch, StreamEvent{
				Type: adapter.EventResponseState, State: responseState(p.Name, p.ProviderType, model, stopReason, len(validToolCalls) > 0),
			}) {
				return
			}
		}
		// 没有失败，发送完整的 tool call
		for _, vtc := range validToolCalls {
			acc := &accumulated[vtc.idx]
			if !adapter.SendEvent(ctx, ch, StreamEvent{
				Type: EventToolCallEnd,
				Delta: &ToolCallDelta{
					ToolName:      acc.name,
					ToolID:        acc.id,
					ArgumentsText: vtc.raw,
					ArgumentsJSON: json.RawMessage(vtc.raw),
				},
			}) {
				return
			}
			diag.hasToolCalls = true
		}

		// 零产出检测：流结束但未收到任何有效内容（content 或 tool call 收尾）。
		// 填充 HTTP 快照后通过 LogValuer 输出结构化诊断，区分限流软拒绝 / 推理失控 / 非标准格式。
		if !diag.hasContent && !diag.hasToolCalls {
			diag.statusCode = resp.StatusCode
			diag.contentType = resp.Header.Get("Content-Type")
			diag.contentLength = resp.Header.Get("Content-Length")
			diag.retryAfter = resp.Header.Get("Retry-After")
			diag.rateLimitRemain = resp.Header.Get("X-RateLimit-Remaining")
			c.logger.Warn("empty sse response", "diag", diag)
			if !adapter.SendEvent(ctx, ch, StreamEvent{Type: EventError, Error: &APIError{
				StatusCode: 0,
				Message:    "流式响应为空，服务商可能不支持流式请求或返回了非标准格式",
				Retryable:  true,
			}}) {
				return
			}
		}
	}
}

// buildToolArgsInvalidMsg 构造 tool arguments 解析失败的错误信息。
// 格式：tool arguments 解析失败（N 个）：tool=xxx: err; tool=yyy: err
// 截断到 200 字符，避免 reminder 过长。
func buildToolArgsInvalidMsg(errs []string) string {
	const maxLen = 200
	prefix := fmt.Sprintf("tool arguments 解析失败（%d 个）", len(errs))
	detail := strings.Join(errs, "; ")
	msg := prefix + "：" + detail
	if len(msg) > maxLen {
		msg = msg[:maxLen] + "..."
	}
	return msg
}

// parseDefaultError 按 OpenAI 标准格式解析错误响应体。
func parseDefaultError(body []byte) error {
	var resp struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &resp); err != nil || resp.Error.Message == "" {
		return fmt.Errorf("request failed: %s", string(body))
	}
	return fmt.Errorf("%s", resp.Error.Message)
}
