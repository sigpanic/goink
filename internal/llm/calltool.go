package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// CallToolStatus 描述一次 toolcall 调用过程中的状态，用于 OnStatus 回调推送。
type CallToolStatus string

const (
	CallToolThinking   CallToolStatus = "thinking"   // 模型推理中
	CallToolGenerating CallToolStatus = "generating" // 模型输出工具参数中
)

// CallToolOptions 是 CallTool 的可选参数。
type CallToolOptions struct {
	ReasoningEffort string               // 空则不设置
	MaxTokens       int                  // 0 表示走 ModelInfo 默认
	Attempts        int                  // 最大尝试次数，≤1 表示只试一次
	OnStatus        func(CallToolStatus) // 状态回调，可 nil
}

// CallTool 请求 LLM 调用指定工具并返回结构化 JSON。
//
// 部分供应商的 thinking mode 不兼容 tool_choice，因此只提供 tools，
// 由提示词约束模型调用目标工具。
//
// tools 数组由调用方组装（通常只含 1 个工具）；本函数负责流式接收、
// 重试、错误聚合。OnStatus 在 LLM 流事件时被调用，nil 时跳过。
//
// 返回值是 EventToolCallEnd 的 ArgumentsJSON（json.RawMessage），
// 调用方按目标类型 json.Unmarshal。
func (c *Client) CallTool(
	ctx context.Context,
	providerName, modelID string,
	messages []map[string]any,
	tools []map[string]any,
	toolName string,
	opts *CallToolOptions,
) (json.RawMessage, error) {
	callOpts := &CallOptions{}
	var onStatus func(CallToolStatus)
	attempts := 1
	if opts != nil {
		onStatus = opts.OnStatus
		if opts.ReasoningEffort != "" {
			callOpts.ReasoningEffort = &opts.ReasoningEffort
		}
		if opts.MaxTokens > 0 {
			mt := opts.MaxTokens
			callOpts.MaxTokens = &mt
		}
		if opts.Attempts > 1 {
			attempts = opts.Attempts
		}
	}

	var allErrs []error
	for i := 0; i < attempts; i++ {
		if i > 0 {
			select {
			case <-time.After(30 * time.Second):
				// continue retry
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		var lastErr error
		sentThinking := false
		events := c.ChatStream(ctx, providerName, messages, tools, modelID, callOpts)
		for evt := range events {
			switch evt.Type {
			case EventError:
				lastErr = evt.Error
			case EventThinking:
				if onStatus != nil && !sentThinking {
					sentThinking = true
					onStatus(CallToolThinking)
				}
			case EventToolCallStart:
				if onStatus != nil {
					onStatus(CallToolGenerating)
				}
			case EventToolCallEnd:
				if evt.Delta != nil && evt.Delta.ToolName == toolName && len(evt.Delta.ArgumentsJSON) > 0 {
					return evt.Delta.ArgumentsJSON, nil
				}
			}
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if lastErr == nil {
			lastErr = fmt.Errorf("LLM 未调用工具 %s", toolName)
		}
		allErrs = append(allErrs, lastErr)
	}
	return nil, errors.Join(allErrs...)
}
