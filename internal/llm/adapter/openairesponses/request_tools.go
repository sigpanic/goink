package openairesponses

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

func buildTools(tools []adapter.ToolDefinition) ([]responses.ToolUnionParam, error) {
	var result []responses.ToolUnionParam
	seen := make(map[string]bool, len(tools))
	for _, tool := range tools {
		if strings.TrimSpace(tool.Name) == "" || seen[tool.Name] {
			return nil, fmt.Errorf("empty or duplicate OpenAI Responses tool name %q", tool.Name)
		}
		seen[tool.Name] = true
		var schema struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(tool.InputSchema, &schema); err != nil || schema.Type != "object" {
			return nil, fmt.Errorf("tool %q requires an object input schema", tool.Name)
		}
		function := responses.FunctionToolParam{
			Name:        tool.Name,
			Description: param.NewOpt(tool.Description),
			Strict:      param.NewOpt(false),
		}
		// SDK Parameters 是 map[string]any；原文覆盖保留大整数和完整 schema。
		// strict=false 保持公共工具 schema 的可选字段语义，不强制改写 required。
		function.SetExtraFields(map[string]any{"parameters": slices.Clone(tool.InputSchema)})
		result = append(result, responses.ToolUnionParam{OfFunction: &function})
	}
	return result, nil
}

// buildToolChoice 接受公共层的 Chat Completions 形状，转换为 Responses 的扁平函数选择。
func buildToolChoice(value any, tools []responses.ToolUnionParam) (responses.ResponseNewParamsToolChoiceUnion, error) {
	var choice responses.ResponseNewParamsToolChoiceUnion
	if mode, ok := value.(string); ok {
		switch mode {
		case "auto", "none":
		case "required":
			if len(tools) == 0 {
				return choice, fmt.Errorf("required tool_choice requires tool definitions")
			}
		default:
			return choice, fmt.Errorf("unsupported tool_choice %q", mode)
		}
		choice.OfToolChoiceMode = param.NewOpt(responses.ToolChoiceOptions(mode))
		return choice, nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return choice, fmt.Errorf("encode tool_choice: %w", err)
	}
	var named struct {
		Type     string `json:"type"`
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&named); err != nil || named.Type != "function" || named.Function.Name == "" {
		return choice, fmt.Errorf("unsupported tool_choice; expected auto, none, required or a named function")
	}
	if !slices.ContainsFunc(tools, func(tool responses.ToolUnionParam) bool {
		return tool.OfFunction != nil && tool.OfFunction.Name == named.Function.Name
	}) {
		return choice, fmt.Errorf("tool_choice references undefined tool %q", named.Function.Name)
	}
	choice.OfFunctionTool = &responses.ToolChoiceFunctionParam{Name: named.Function.Name}
	return choice, nil
}
