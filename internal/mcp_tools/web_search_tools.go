package mcp_tools

import (
	"context"
	"encoding/json"
)

// WebSearchArgs 是 web_search 工具的参数。
type WebSearchArgs struct {
	Query string `json:"query" jsonschema:"required,description=搜索关键词或自然语言问题，支持 site: 等搜索语法,minLength=1,maxLength=128" validate:"required,min=1,max=128"`
}

// WebSearchTool 通过注入的搜索能力获取逐条网页摘要和来源链接。
type WebSearchTool struct{}

func (t *WebSearchTool) Name() string           { return "web_search" }
func (t *WebSearchTool) Category() ToolCategory { return CategoryWritingAssistant }
func (t *WebSearchTool) ExposeToLLM() bool      { return true }

func (t *WebSearchTool) Description() string {
	return "联网搜索真实信息，返回最多 10 条网页来源及各自的 summary。适用于需要实时数据、新闻、技术文档或超出模型知识范围的内容。" +
		"summary 是单个网页的相关摘要，需要自行综合并引用来源；truncated 为 true 表示该条摘要已截断。摘要不足或需要核对原文时，使用 web_fetch 读取该 URL。" +
		"DeepSeek 可选评分：authority 为来源权威性，0～4，≥3 表示较高；rerank_score 综合相关性、权威性和时效性，0～3，≥1.5 表示较优。缺失表示未提供或未知，评分仅作辅助，不代表事实正确率。"
}

func (t *WebSearchTool) JSONSchema() json.RawMessage {
	return SchemaOf(WebSearchArgs{})
}

func (t *WebSearchTool) NewArgs() any { return &WebSearchArgs{} }

func (t *WebSearchTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*WebSearchArgs)

	if tc.WebSearch == nil {
		return &ToolResult{Error: "网络搜索未启用：请先在 LLM 设置中配置 DeepSeek"}, nil
	}

	result, err := tc.WebSearch.Search(ctx, a.Query)
	if err != nil {
		return webToolFailure("搜索", err)
	}

	return &ToolResult{
		Success: true,
		Data: map[string]any{
			"query":   result.Query,
			"results": result.Results,
		},
	}, nil
}

// RegisterWebSearchTools 注册网络搜索工具。
func RegisterWebSearchTools(r *Registry) {
	r.Register(&WebSearchTool{})
}
