package mcp_tools

import (
	"context"
	"encoding/json"

	"github.com/sigpanic/goink/internal/web"
)

// WebFetchArgs 是 web_fetch 工具的参数。
type WebFetchArgs struct {
	URL string `json:"url" jsonschema:"required,description=要抓取的网页 URL" validate:"required,url,min=1,max=2048"`
}

// WebFetchTool 抓取网页内容，清洗后返回 markdown 格式正文。
type WebFetchTool struct{}

func (t *WebFetchTool) Name() string           { return "web_fetch" }
func (t *WebFetchTool) Category() ToolCategory { return CategoryWritingAssistant }
func (t *WebFetchTool) ExposeToLLM() bool      { return true }

func (t *WebFetchTool) Description() string {
	return "抓取指定网页的正文内容，返回清洗后的 markdown 文本。适用于需要查看某个来源原文、深入了解细节或验证 web_search 结果时使用。一次只能抓取一个 URL。"
}

func (t *WebFetchTool) JSONSchema() json.RawMessage {
	return SchemaOf(WebFetchArgs{})
}

func (t *WebFetchTool) NewArgs() any { return &WebFetchArgs{} }

func (t *WebFetchTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*WebFetchArgs)

	result, err := web.Fetch(a.URL)
	if err != nil {
		return &ToolResult{Error: "抓取失败: " + err.Error()}, nil
	}

	return &ToolResult{
		Success: true,
		Data: map[string]any{
			"url":   result.URL,
			"title": result.Title,
			"text":  result.Text,
		},
	}, nil
}

// RegisterWebFetchTools 注册网页抓取工具。
func RegisterWebFetchTools(r *Registry) {
	r.Register(&WebFetchTool{})
}
