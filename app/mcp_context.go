package app

import (
	"context"
	"errors"

	"github.com/sigpanic/goink/internal/llm"
	"github.com/sigpanic/goink/internal/llm/adapter"
	"github.com/sigpanic/goink/internal/mcp_tools"
)

// mcpCallContext 保留 MCP 请求生命周期，仅补充桌面运行时的上下文值。
type mcpCallContext struct {
	context.Context
	runtime context.Context
}

func (c mcpCallContext) Value(key any) any {
	if value := c.Context.Value(key); value != nil {
		return value
	}
	if c.runtime != nil {
		return c.runtime.Value(key)
	}
	return nil
}

func (a *App) mcpToolContext(ctx context.Context) (context.Context, mcp_tools.ToolContext) {
	// 勾选工具即授权直接调用；外部请求不使用聊天审批通道。
	return mcpCallContext{Context: ctx, runtime: a.ctx}, mcp_tools.ToolContext{
		SkillStore: a.skill, SearchService: a.searchService.Load(),
		WebSearch: func(ctx context.Context, query string) (*llm.WebSearchResult, error) {
			if a.llmClient == nil {
				return nil, errors.New("LLM 客户端未初始化")
			}
			provider, ok := a.llmClient.Providers()[adapter.ProtocolOpenAIChat]["DeepSeek"]
			if !ok || provider.APIKey == "" {
				return nil, errors.New("DeepSeek API key 未配置")
			}
			return llm.SearchWeb(ctx, provider.APIKey, "", query)
		},
	}
}
