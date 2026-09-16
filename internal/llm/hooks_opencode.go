package llm

import (
	"github.com/google/uuid"

	"github.com/sigpanic/goink/internal/version"
)

// opencodeBuildHeaders 适配 OpenCode 网关（Zen 与 Go 共用，https://opencode.ai/zen/...）的强制要求：
//  1. 自定义 User-Agent 标识客户端（不能用 Go-http-client 这类通用库名）
//  2. 每个请求带稳定的 x-opencode-session 会话 ID（缺失直接 400 MissingSessionID）
//
// 会话 ID 优先取 opts.SessionID（主对话链路为 Goink session_id，per-conversation 稳定，
// 压缩/子 agent 天然一致）；opts 为 nil 或 SessionID 为空（探测/提取等一次性请求）
// 时用 uuid v4 兜底，保证请求能过网关（无缓存收益，符合该场景语义）。
func opencodeBuildHeaders(opts *CallOptions, base map[string]string) map[string]string {
	sessionID := ""
	if opts != nil {
		sessionID = opts.SessionID
	}
	if sessionID == "" {
		sessionID = uuid.NewString()
	}
	base["x-opencode-session"] = sessionID
	base["User-Agent"] = "Goink/" + version.Version
	return base
}
