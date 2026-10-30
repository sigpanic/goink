package gemini

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	types "google.golang.org/genai/interactions/models/interactions"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

func buildTools(tools []adapter.ToolDefinition) ([]types.Tool, error) {
	var result []types.Tool
	seen := make(map[string]bool, len(tools))
	for _, tool := range tools {
		if strings.TrimSpace(tool.Name) == "" || seen[tool.Name] {
			return nil, fmt.Errorf("empty or duplicate Gemini tool name %q", tool.Name)
		}
		seen[tool.Name] = true
		var schema struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(tool.InputSchema, &schema); err != nil || schema.Type != "object" {
			return nil, fmt.Errorf("tool %q requires an object input schema", tool.Name)
		}
		result = append(result, types.NewTool(types.Function{
			Name: &tool.Name, Description: &tool.Description, Parameters: slices.Clone(tool.InputSchema),
		}))
	}
	return result, nil
}

// buildToolChoice 接受公共层现有的 OpenAI 形状，不透传未知对象。
func buildToolChoice(value any, tools []types.Tool) (types.ToolChoice, error) {
	var choice types.ToolChoice
	if len(tools) == 0 {
		return choice, fmt.Errorf("tool_choice requires tool definitions")
	}
	if mode, ok := value.(string); ok {
		switch mode {
		case "auto":
			return types.NewToolChoice(types.ToolChoiceTypeAuto), nil
		case "none":
			return types.NewToolChoice(types.ToolChoiceTypeNone), nil
		case "required":
			return types.NewToolChoice(types.ToolChoiceTypeAny), nil
		default:
			return choice, fmt.Errorf("unsupported tool_choice %q", mode)
		}
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
	if !slices.ContainsFunc(tools, func(tool types.Tool) bool {
		return tool.Function != nil && tool.Function.Name != nil && *tool.Function.Name == named.Function.Name
	}) {
		return choice, fmt.Errorf("tool_choice references undefined tool %q", named.Function.Name)
	}
	mode := types.ToolChoiceTypeAny
	return types.NewToolChoice(types.ToolChoiceConfig{AllowedTools: &types.AllowedTools{
		Mode: &mode, Tools: []string{named.Function.Name},
	}}), nil
}
