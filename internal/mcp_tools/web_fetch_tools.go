package mcp_tools

import (
	"context"
	"encoding/json"

	"github.com/sigpanic/goink/internal/web"
)

// WebFetchArgs 是 web_fetch 工具的参数。
type WebFetchArgs struct {
	URL  string        `json:"url" jsonschema:"required,description=要抓取的 HTTP 或 HTTPS 网页 URL,maxLength=8192" validate:"required,http_url,min=1,max=8192"`
	Mode web.FetchMode `json:"mode,omitempty" jsonschema:"description=local 为本地抓取，remote 为按次计费的远程抓取,enum=local,enum=remote,default=local" validate:"omitempty,oneof=local remote"`
}

// WebFetchTool 抓取网页内容，清洗后返回 markdown 格式正文。
type WebFetchTool struct{}

func (t *WebFetchTool) Name() string           { return "web_fetch" }
func (t *WebFetchTool) Category() ToolCategory { return CategoryWritingAssistant }
func (t *WebFetchTool) ExposeToLLM() bool      { return true }

func (t *WebFetchTool) Description() string {
	return "抓取指定网页的正文内容，返回 markdown 文本及 mode、truncated 状态。一次只能抓取一个 URL。" +
		"默认 mode=local，在本机请求并提取正文；本地抓取失败或内容不可用时，可用同一 URL、mode=remote 重试一次。" +
		"remote 使用远程 Fetch 服务，会按成功请求次数产生费用；远程仍失败或内容不可用时请更换来源，不要对同一 URL 重复付费重试。"
}

func (t *WebFetchTool) JSONSchema() json.RawMessage {
	return SchemaOf(WebFetchArgs{})
}

func (t *WebFetchTool) NewArgs() any { return &WebFetchArgs{} }

func (t *WebFetchTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*WebFetchArgs)

	mode := a.Mode
	if mode == "" {
		mode = web.LocalFetch
	}
	var result *web.FetchResult
	var err error
	if mode == web.RemoteFetch {
		if tc.RemoteFetch == nil {
			return &ToolResult{Error: "远程抓取未启用：请先在 LLM 设置中配置 DeepSeek"}, nil
		}
		result, err = tc.RemoteFetch.Fetch(ctx, a.URL)
	} else {
		result, err = web.Fetch(ctx, a.URL)
	}
	if err != nil {
		return webToolFailure("抓取", err)
	}

	return &ToolResult{
		Success: true,
		Data: map[string]any{
			"url":       result.URL,
			"title":     result.Title,
			"text":      result.Text,
			"mode":      result.Mode,
			"truncated": result.Truncated,
		},
	}, nil
}

// RegisterWebFetchTools 注册网页抓取工具。
func RegisterWebFetchTools(r *Registry) {
	r.Register(&WebFetchTool{})
}
