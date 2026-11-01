package openaichat

import (
	"log/slog"
	"strings"
)

// sseDiagnostics 追踪 parseSSE 流的诊断信息。
// 实现 slog.LogValuer 接口，日志中作为单个字段传入即展开为结构化字段组（嵌套在 diag 名下）。
// empty sse 时填充 HTTP 快照后传入日志，便于区分限流软拒绝 / 推理失控 / 非标准格式等场景。
type sseDiagnostics struct {
	// 循环追踪值（parseSSE 内累积）
	hasContent    bool           // 收到过 content 文本
	hasToolCalls  bool           // tool call 收尾成功（发过 EventToolCallEnd）
	hasReason     bool           // 收到过 reasoning_content
	dataLineCount int            // data: 行总数
	gotDone       bool           // 是否收到 [DONE]
	lastUsage     map[string]any // 最后一个 usage chunk（发 EventUsage 用）
	headLines     []string       // 前 10 行原始行
	tailLines     []string       // 后 10 行原始行（环形切片）

	// HTTP 快照（empty 分支从 resp 填入）
	statusCode      int
	contentType     string
	contentLength   string
	retryAfter      string
	rateLimitRemain string
}

// 编译时验证 sseDiagnostics 实现 slog.LogValuer 接口。
// 若 LogValue 方法签名被改坏，此行编译报错，防止运行时退化为 fmt 格式化。
var _ slog.LogValuer = sseDiagnostics{}

// LogValue 实现 slog.LogValuer，将诊断字段展开为结构化日志属性组。
// raw_head / raw_tail 每行截断到 500 rune，避免超长 JSON 错误体撑爆日志。
func (d sseDiagnostics) LogValue() slog.Value {
	const maxLineRunes = 500
	head := make([]string, len(d.headLines))
	for i, s := range d.headLines {
		head[i] = truncateLine(s, maxLineRunes)
	}
	tail := make([]string, len(d.tailLines))
	for i, s := range d.tailLines {
		tail[i] = truncateLine(s, maxLineRunes)
	}
	return slog.GroupValue(
		slog.Int("status_code", d.statusCode),
		slog.String("content_type", d.contentType),
		slog.String("content_length", d.contentLength),
		slog.String("retry_after", d.retryAfter),
		slog.String("x_ratelimit_remaining", d.rateLimitRemain),
		slog.Bool("has_content", d.hasContent),
		slog.Bool("has_tool_calls", d.hasToolCalls),
		slog.Bool("has_reason", d.hasReason),
		slog.Int("data_lines", d.dataLineCount),
		slog.Bool("got_done", d.gotDone),
		slog.Bool("has_usage", d.lastUsage != nil),
		slog.String("raw_head", strings.Join(head, "\n")),
		slog.String("raw_tail", strings.Join(tail, "\n")),
	)
}

// truncateLine 截断单行到 maxRunes 字符（按 rune 计数，避免截断 UTF-8 多字节字符中段）。
// 超长时保留前 maxRunes 个字符并追加 "..." 标记。
func truncateLine(s string, maxRunes int) string {
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	return string(r[:maxRunes]) + "..."
}
