package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// TestConnectionInput 是连通性测试的参数。
type TestConnectionInput struct {
	ProviderName string `json:"provider_name"`
	ChatURL      string `json:"chat_url"`
	APIKey       string `json:"api_key"`
	ModelID      string `json:"model_id"`
}

// expandChatURLCandidates 生成候选 URL 列表，用于多层 fallback 真测。
// 顺序（按可能性从高到低）：
//  1. 原样（用户填的完整端点，如 https://x.com/proxy/chat）
//  2. + /chat/completions（base URL，如 https://api.deepseek.com/v1）
//  3. + /v1/chat/completions（裸域名，如 https://1024token.club）
//  4. 去掉末尾 /chat/completions 再补 /v1/chat/completions（用户填 /v1/chat/completions 但实际端点是 /v1 的变体）
//  5. 去掉末尾 /v1/chat/completions 再补 /chat/completions（反向情况）
//
// 去重并清理（补 https://、TrimRight 末尾斜杠）。
func expandChatURLCandidates(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
		raw = "https://" + raw
	}
	raw = strings.TrimRight(raw, "/")

	var candidates []string
	seen := make(map[string]bool)
	add := func(s string) {
		if s != "" && !seen[s] {
			seen[s] = true
			candidates = append(candidates, s)
		}
	}

	// 1. 原样（假设是完整端点）
	add(raw)
	// 2. + /chat/completions（假设是 base URL）
	if !strings.HasSuffix(raw, "/chat/completions") {
		add(raw + "/chat/completions")
	}
	// 3. + /v1/chat/completions（仅在裸域名时，避免对带版本号的路径误补 /v1）
	if _, hostPart, ok := strings.Cut(raw, "://"); ok {
		if !strings.Contains(hostPart, "/") {
			add(raw + "/v1/chat/completions")
		}
	}
	// 4. 去掉末尾 /chat/completions 再补 /v1/chat/completions
	//    若 base 已以 /v1 结尾则跳过（避免 /v1/v1/chat/completions 重复）
	if base, ok := strings.CutSuffix(raw, "/chat/completions"); ok {
		if !strings.HasSuffix(base, "/v1") {
			add(base + "/v1/chat/completions")
		}
	}
	// 5. 去掉末尾 /v1/chat/completions 再补 /chat/completions
	if base, ok := strings.CutSuffix(raw, "/v1/chat/completions"); ok {
		add(base + "/chat/completions")
	}

	return candidates
}

// TestConnection 发送最小化请求验证 provider 连通性。
// 多层 fallback：对候选 URL 逐个真测（真 key + model + stream + max_tokens=1），
// 找到第一个返回 200 + text/event-stream + 有效 SSE chunk（含 choices 字段）的候选，
// 返回该 URL。全部失败返回 error，附带最后一个候选的错误信息。
//
// 返回值：
//   - (url, nil)：验证通过，url 是实际可用的端点（可能和入参不同，是 fallback 探测到的）
//   - ("", error)：所有候选均失败
//
// 设计目标：用户填啥都能 work。
//   - 裸域名 https://1024token.club → 探测到 /v1/chat/completions
//   - base URL https://api.deepseek.com/v1 → 探测到 /v1/chat/completions
//   - 完整端点 https://x.com/proxy/chat → 原样通过
//
// 调用方（前端）应将返回的 url 回写到 provider.chat_url，确保保存的 URL 和测试时一致。
func TestConnection(ctx context.Context, builtin map[string]Provider, input TestConnectionInput) (string, error) {
	chatURL := input.ChatURL
	buildHeaders := func(base map[string]string) map[string]string { return base }
	var buildRequest func(map[string]any) map[string]any

	if bp, ok := builtin[input.ProviderName]; ok {
		if chatURL == "" {
			chatURL = bp.ChatURL
		}
		if bp.BuildHeaders != nil {
			buildHeaders = bp.BuildHeaders
		}
		if bp.BuildRequest != nil {
			buildRequest = bp.BuildRequest
		}
	}

	candidates := expandChatURLCandidates(chatURL)
	if len(candidates) == 0 {
		return "", fmt.Errorf("URL 为空")
	}

	payload := map[string]any{
		"model": input.ModelID,
		"messages": []map[string]any{
			{"role": "user", "content": "hi"},
		},
		"max_tokens": 1,
		"stream":     true,
	}
	if buildRequest != nil {
		payload = buildRequest(payload)
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("序列化请求失败: %w", err)
	}

	headers := buildHeaders(map[string]string{
		"Content-Type":  "application/json",
		"Accept":        "text/event-stream",
		"Authorization": "Bearer " + input.APIKey,
	})

	// 共享 client，单候选 8s 超时（中转站通常 1-2s 响应，8s 足够且避免无限等待）
	client := &http.Client{}

	// 收集所有候选的错误，全部失败时一并返回，让用户看到每个候选的真实失败原因
	// （避免只显示最后一个候选的错误，掩盖原样 URL 的真实错误，如 401/余额不足）
	var allErrs []string
	for _, candidate := range candidates {
		probeCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
		err := probeCandidate(probeCtx, client, candidate, body, headers)
		cancel()
		if err == nil {
			return candidate, nil
		}
		allErrs = append(allErrs, fmt.Sprintf("候选 %s: %v", candidate, err))
	}
	return "", fmt.Errorf("所有候选 URL 均验证失败:\n%s", strings.Join(allErrs, "\n"))
}

// probeCandidate 对单个候选 URL 发真测请求，验证 200 + text/event-stream + 有效 SSE chunk。
// 返回 nil 表示通过；返回 error 表示失败（调用方用循环里的 candidate 作为成功 URL）。
//
// 验证链路：
//  1. HTTP 请求成功（网络可达）
//  2. 状态码 < 400（4xx/5xx 被拦下）
//  3. Content-Type 是 text/event-stream（中转站 SPA fallback 返回 200+HTML 会被拦下）
//  4. SSE 流中至少有一个 data: 行，且 JSON 含 choices 字段（空流/非标格式被拦下）
func probeCandidate(ctx context.Context, client *http.Client, url string, body []byte, headers map[string]string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("%s", summarizeErrorBody(resp.StatusCode, errBody))
	}

	// 检查 Content-Type 是 SSE 流。中转站 SPA 对不存在的路径可能返回 200 + HTML 首页，
	// 仅靠状态码会误判通过；这里强制要求 text/event-stream，HTML 错误页会被拦下。
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "text/event-stream") {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("响应不是 SSE 流 (Content-Type: %s), %s", ct, summarizeErrorBody(resp.StatusCode, errBody))
	}

	// 扫描 SSE 流，验证至少有一个 data: 行且 JSON 含 choices 字段。
	// 防止中转站返回空 SSE 流或非标格式（如 GPT-5 Responses API 的 event: 类型）。
	// 读到第一个有效 chunk 就 break，不等流结束，负担小。
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := line[len("data: "):]
		if data == "[DONE]" {
			break
		}
		var chunk map[string]any
		if err := json.Unmarshal([]byte(data), &chunk); err == nil {
			if _, ok := chunk["choices"]; ok {
				return nil
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("读取 SSE 流失败: %w", err)
	}
	return fmt.Errorf("SSE 流中未找到有效 chunk（可能模型不可用或返回非标准格式）")
}

// summarizeErrorBody 把 HTTP 错误响应体格式化为简洁的错误消息。
// 优先解析 OpenAI 兼容错误格式 {error: {message, type}}，提取 message（覆盖 OpenAI/Anthropic/Gemini/中转站）；
// 其次解析扁平格式（非标准中转站通用格式 + Spring Boot 默认错误格式）：
//   - {message, error (字符串)}：error 是错误码字符串
//   - {error (字符串), path}：Spring Boot 默认错误格式，带 path 让用户看出 URL 拼接错误
//
// HTML 错误页简化为 [HTML 错误页]（HTML 内容对诊断无价值）；其他格式原文截断到 200 字符。
//
// 示例：
//   - {"error":{"message":"Invalid API Key","type":"invalid_key"}}  → [401] Invalid API Key (invalid_key)
//   - {"message":"没找到对象","error":"url.not_found"}               → [404] 没找到对象 (url.not_found)
//   - {"error":"Not Found","path":"/v4/chat/completion/models"}     → [404] Not Found (path: /v4/chat/completion/models)
//   - <html><body>404 Not Found</body></html>                        → [404] [HTML 错误页，可能 URL 错误或被防火墙拦]
//   - 纯文本错误                                                    → [500] <原文截断到 200 字符>
func summarizeErrorBody(statusCode int, body []byte) string {
	// 1. 尝试 OpenAI 兼容错误格式 {error: {message, type}}
	// 主流服务商（OpenAI/Anthropic/Gemini/DeepSeek/Kimi/GLM/Qwen/MiMo 等）及中转站都遵循此格式
	var errResp struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &errResp); err == nil && errResp.Error.Message != "" {
		if errResp.Error.Type != "" {
			return fmt.Sprintf("[%d] %s (%s)", statusCode, errResp.Error.Message, errResp.Error.Type)
		}
		return fmt.Sprintf("[%d] %s", statusCode, errResp.Error.Message)
	}
	// 2. 尝试扁平格式（非标准中转站 + Spring Boot 默认错误格式）
	// 情况 A: {message, error (字符串)} — message 在顶层，error 是错误码字符串
	//   如 {"code":5,"error":"url.not_found","message":"没找到对象",...}
	// 情况 B: {error (字符串), path, ...} — Spring Boot 默认错误格式，无 message
	//   如 {"timestamp":"...","status":404,"error":"Not Found","path":"/v1/models"}
	//   path 让用户看出 URL 拼接错误（如 /v4/chat/completion/models ← chat URL 填错）
	// error 为对象时 json.Unmarshal 到 string 会失败，整个分支跳过，不会误伤 OpenAI 格式
	var flatResp struct {
		Message string `json:"message"`
		Error   string `json:"error"`
		Path    string `json:"path"`
	}
	if err := json.Unmarshal(body, &flatResp); err == nil {
		switch {
		case flatResp.Message != "" && flatResp.Error != "":
			// 情况 A: message + error 错误码
			return fmt.Sprintf("[%d] %s (%s)", statusCode, flatResp.Message, flatResp.Error)
		case flatResp.Message != "":
			return fmt.Sprintf("[%d] %s", statusCode, flatResp.Message)
		case flatResp.Error != "":
			// 情况 B: Spring Boot 格式，error 是 HTTP 描述，带 path 才有诊断价值
			if flatResp.Path != "" {
				return fmt.Sprintf("[%d] %s (path: %s)", statusCode, flatResp.Error, flatResp.Path)
			}
			return fmt.Sprintf("[%d] %s", statusCode, flatResp.Error)
		}
	}
	// 3. HTML 错误页 → 简短标识（HTML 全文对诊断无价值）
	// 用精确检测：含 <html/<head/<body 标签才算 HTML 错误页，避免把纯文本误判为 HTML
	if isHTMLPage(body) {
		return fmt.Sprintf("[%d] [HTML 错误页，可能 URL 错误或被防火墙拦]", statusCode)
	}
	// 4. 其他格式 → 原文截断到 200 字符
	s := strings.TrimSpace(string(body))
	if s == "" {
		return fmt.Sprintf("[%d] [空响应，可能 URL 错误或端点不存在]", statusCode)
	}
	if len(s) > 200 {
		s = s[:200] + "..."
	}
	return fmt.Sprintf("[%d] %s", statusCode, s)
}

// isHTMLPage 检测响应体是否为 HTML 页面（含 <html/<head/<body 标签）。
// 用于错误格式化时区分 HTML 错误页和纯文本错误。
func isHTMLPage(body []byte) bool {
	s := strings.ToLower(string(body))
	return strings.Contains(s, "<html") || strings.Contains(s, "<head") || strings.Contains(s, "<body")
}
