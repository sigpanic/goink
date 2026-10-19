package openaichat

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

// buildTools 构造本次请求独占的 OpenAI 函数定义，供 provider hooks 继续处理。
// 只校验名称和 schema 的基本形状，不解析或裁剪 schema 内部的关键字。
func buildTools(tools []adapter.ToolDefinition) ([]map[string]any, error) {
	if tools == nil {
		return nil, nil
	}
	result := make([]map[string]any, 0, len(tools))
	for i, tool := range tools {
		if strings.TrimSpace(tool.Name) == "" {
			return nil, fmt.Errorf("tool %d: function name must not be empty", i)
		}
		schema := bytes.TrimSpace(tool.InputSchema)
		if !json.Valid(schema) || schema[0] != '{' {
			return nil, fmt.Errorf("tool %d (%q): input schema must be a JSON object", i, tool.Name)
		}
		result = append(result, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        tool.Name,
				"description": tool.Description,
				"parameters":  tool.Clone().InputSchema,
			},
		})
	}
	return result, nil
}
