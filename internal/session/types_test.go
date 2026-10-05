package session

import (
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestToAPIFormat_UserMessage(t *testing.T) {
	m := &Message{Role: "user", Content: "你好"}
	result := m.ToAPIFormat(discardLogger())
	if result["role"] != "user" {
		t.Errorf("role: got %v", result["role"])
	}
	if result["content"] != "你好" {
		t.Errorf("content: got %v", result["content"])
	}
	if _, exists := result["reasoning_content"]; exists {
		t.Error("user messages should not have reasoning_content")
	}
}

func TestToAPIFormat_AssistantWithThinking(t *testing.T) {
	m := &Message{
		Role:            "assistant",
		Content:         "回复内容",
		ThinkingContent: "思考过程",
	}
	result := m.ToAPIFormat(discardLogger())
	if result["role"] != "assistant" {
		t.Errorf("role: got %v", result["role"])
	}
	if result["reasoning_content"] != "思考过程" {
		t.Errorf("reasoning_content: got %v", result["reasoning_content"])
	}
}

func TestToAPIFormat_AssistantWithToolCalls(t *testing.T) {
	m := &Message{
		Role:          "assistant",
		ExtraMetadata: `{"tool_calls":[{"id":"1","function":{"name":"read","arguments":"{}"}}]}`,
	}
	result := m.ToAPIFormat(discardLogger())
	if tc, ok := result["tool_calls"]; !ok || tc == nil {
		t.Error("tool_calls should be present")
	}
	// 没有 thinking 但有 tool_calls → reasoning_content 为空字符串
	if rc, ok := result["reasoning_content"]; !ok || rc != "" {
		t.Errorf("reasoning_content should be empty string, got %v", rc)
	}
}

func TestToAPIFormat_ToolMessage(t *testing.T) {
	m := &Message{
		Role:          "tool",
		ExtraMetadata: `{"tool_call_id":"call_123","tool_name":"read"}`,
	}
	result := m.ToAPIFormat(discardLogger())
	if result["role"] != "tool" {
		t.Errorf("role: got %v", result["role"])
	}
	if result["tool_call_id"] != "call_123" {
		t.Errorf("tool_call_id: got %v", result["tool_call_id"])
	}
	if result["name"] != "read" {
		t.Errorf("name: got %v", result["name"])
	}
}

func TestToAPIFormat_SystemMessage(t *testing.T) {
	m := &Message{Role: "system", Content: "系统提示"}
	result := m.ToAPIFormat(discardLogger())
	if result["role"] != "system" {
		t.Errorf("role: got %v", result["role"])
	}
	// system 消息不应有任何额外字段
	if _, exists := result["reasoning_content"]; exists {
		t.Error("system messages should not have reasoning_content")
	}
}

func TestToAPIFormat_UserMessagePrefixedWithCreatedAt(t *testing.T) {
	// 固定一个明确的过去时间（time.Local 保证 .Local() 是同一挂钟值，不受测试机时区影响）。
	// 若实现误用 time.Now()，产物无法等于这个固定字节，本测试会立刻暴露回归。
	sentAt := time.Date(2020, 1, 2, 3, 4, 0, 0, time.Local)
	m := &Message{Role: "user", Content: "你好", CreatedAt: sentAt, ToFrontend: true}

	result := m.ToAPIFormat(discardLogger())
	if got, want := result["content"], "[2020-01-02 03:04] 你好"; got != want {
		t.Errorf("content: got %q, want %q", got, want)
	}
	// Content 字段本身不得被就地改写，否则会污染前端展示、审计查询与统计。
	if m.Content != "你好" {
		t.Errorf("Content mutated in place: got %q", m.Content)
	}
}

func TestToAPIFormat_UserMessageWithoutCreatedAtHasNoPrefix(t *testing.T) {
	// 未入库（CreatedAt 零值）的消息不加前缀，守住既有 ToAPIFormat 语义。
	m := &Message{Role: "user", Content: "你好", ToFrontend: true}
	result := m.ToAPIFormat(discardLogger())
	if got := result["content"]; got != "你好" {
		t.Errorf("content: got %q, want %q", got, "你好")
	}
}

// TestToAPIFormat_InjectedUserMessageHasNoPrefix 覆盖系统注入的 user 消息（ToFrontend=false）：
// 它们不是用户发言，不加时间前缀。压缩保留的副本也走这条路——其 Content 已含上次渲染的
// 前缀，若这里再加一层就会变成 "[压缩时间] [原始时间] 内容" 并逐次叠加。
func TestToAPIFormat_InjectedUserMessageHasNoPrefix(t *testing.T) {
	sentAt := time.Date(2020, 1, 2, 3, 4, 0, 0, time.Local)

	t.Run("系统和注入消息", func(t *testing.T) {
		m := &Message{Role: "user", Content: "上下文已压缩", CreatedAt: sentAt, ToFrontend: false}
		result := m.ToAPIFormat(discardLogger())
		if got := result["content"]; got != "上下文已压缩" {
			t.Errorf("content: got %q, want unprefixed", got)
		}
	})

	t.Run("压缩保留副本不重复加前缀", func(t *testing.T) {
		// 模拟压缩产物：Content 已带前缀，created_at 被 GORM 刷成压缩时刻，ToFrontend=false。
		m := &Message{
			Role:       "user",
			Content:    "[2020-01-02 03:04] 你好",
			CreatedAt:  time.Date(2021, 6, 7, 8, 9, 0, 0, time.Local),
			ToFrontend: false,
		}
		result := m.ToAPIFormat(discardLogger())
		if got, want := result["content"], "[2020-01-02 03:04] 你好"; got != want {
			t.Errorf("content: got %q, want %q (不应叠加)", got, want)
		}
	})
}

func TestToAPIFormat_NonUserMessagesKeepContentUnprefixed(t *testing.T) {
	sentAt := time.Date(2020, 1, 2, 3, 4, 0, 0, time.Local)
	cases := []struct {
		name string
		msg  *Message
	}{
		{"system", &Message{Role: "system", Content: "系统提示", CreatedAt: sentAt}},
		{"assistant", &Message{Role: "assistant", Content: "回复", CreatedAt: sentAt}},
		{"tool", &Message{Role: "tool", Content: "工具结果", CreatedAt: sentAt}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result := c.msg.ToAPIFormat(discardLogger())
			if got := result["content"]; got != c.msg.Content {
				t.Errorf("content: got %q, want unprefixed %q", got, c.msg.Content)
			}
		})
	}
}

// renderAPIMessages 是 loadAPIMessages 的最小复刻：按 DB 顺序把消息转为 API 格式并序列化。
// map 的键由 encoding/json 排序，因此字节输出是确定性的。
func renderAPIMessages(t *testing.T, msgs []*Message) string {
	t.Helper()
	arr := make([]map[string]any, 0, len(msgs))
	for _, m := range msgs {
		arr = append(arr, m.ToAPIFormat(discardLogger()))
	}
	b, err := json.Marshal(arr)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}

// TestToAPIFormat_HistoryPrefixStaysByteStable 验证前缀缓存最关键的性质：
// 同一段历史每轮渲染出的字节完全一致；追加新一轮用户消息后，旧历史仍是新请求的字节前缀。
// 该性质依赖时间戳取自不可变的 CreatedAt——若改用 time.Now()，第二轮的字节前缀就会从
// 时间戳处漂移，导致分叉点之后的全部历史缓存失效。
func TestToAPIFormat_HistoryPrefixStaysByteStable(t *testing.T) {
	history := []*Message{
		{Role: "system", Content: "你是网文写作助手"},
		{Role: "user", Content: "帮我看看第一章", CreatedAt: time.Date(2020, 1, 2, 3, 4, 0, 0, time.Local), ToFrontend: true},
		{Role: "assistant", Content: "好的，我先读一下"},
	}

	first := renderAPIMessages(t, history)
	second := renderAPIMessages(t, history)
	if first != second {
		t.Fatalf("同一历史两次渲染不一致，前缀缓存必然失效:\n first=%s\nsecond=%s", first, second)
	}

	// 模拟下一轮：原样带上历史，末尾追加一条新的用户消息。
	next := make([]*Message, len(history), len(history)+1)
	copy(next, history)
	next = append(next, &Message{
		Role:       "user",
		Content:    "继续",
		CreatedAt:  time.Date(2020, 1, 2, 3, 5, 0, 0, time.Local),
		ToFrontend: true,
	})

	nextJSON := renderAPIMessages(t, next)
	// 去掉旧 JSON 数组的收尾 ']'，新请求必须以该前缀开头（新增元素是其后追加的）。
	prefix := strings.TrimSuffix(first, "]")
	if !strings.HasPrefix(nextJSON, prefix) {
		t.Errorf("追加新消息后历史前缀漂移，缓存将从分叉点起全部失效:\nprefix=%s\n  next=%s", prefix, nextJSON)
	}
}
