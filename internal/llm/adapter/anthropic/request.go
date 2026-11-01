package anthropic

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

// buildRequest 只构造请求参数，不发送请求，也不修改配置、消息或工具定义。
func (p Provider) buildRequest(req adapter.Request) (sdk.MessageNewParams, error) {
	params := sdk.MessageNewParams{Model: req.Model, MaxTokens: 4096}
	if strings.TrimSpace(req.Model) == "" {
		return params, fmt.Errorf("anthropic model must not be empty")
	}
	var model adapter.ModelInfo
	for _, candidate := range p.Models {
		if candidate.ID == req.Model {
			model = candidate
			break
		}
	}
	if model.MaxOutputTokens > 0 {
		params.MaxTokens = int64(model.MaxOutputTokens)
	}
	if req.Options != nil && req.Options.MaxTokens != nil {
		params.MaxTokens = int64(*req.Options.MaxTokens)
	}
	if params.MaxTokens <= 0 || (model.MaxOutputTokens > 0 && params.MaxTokens > int64(model.MaxOutputTokens)) {
		return params, fmt.Errorf("invalid max_tokens %d for model %q", params.MaxTokens, req.Model)
	}
	var err error
	params.Tools, err = buildTools(req.Tools)
	if err != nil {
		return params, err
	}
	if err := p.applyOptions(&params, model, req.Options); err != nil {
		return params, err
	}
	if err := p.buildMessages(&params, req.Messages); err != nil {
		return params, err
	}
	if len(params.Messages) == 0 || params.Messages[0].Role != sdk.MessageParamRoleUser {
		return params, fmt.Errorf("anthropic history must start with a user message")
	}
	if params.Messages[len(params.Messages)-1].Role != sdk.MessageParamRoleUser {
		return params, fmt.Errorf("anthropic assistant prefill is not supported")
	}
	return params, nil
}

func buildTools(tools []adapter.ToolDefinition) ([]sdk.ToolUnionParam, error) {
	var result []sdk.ToolUnionParam
	seen := make(map[string]bool, len(tools))
	for _, tool := range tools {
		if strings.TrimSpace(tool.Name) == "" || seen[tool.Name] {
			return nil, fmt.Errorf("empty or duplicate Anthropic tool name %q", tool.Name)
		}
		seen[tool.Name] = true
		var schema struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(tool.InputSchema, &schema); err != nil || schema.Type != "object" {
			return nil, fmt.Errorf("tool %q requires an object input_schema", tool.Name)
		}
		result = append(result, sdk.ToolUnionParam{OfTool: &sdk.ToolParam{
			Name: tool.Name, Description: sdk.String(tool.Description),
			InputSchema: param.Override[sdk.ToolInputSchemaParam](slices.Clone(tool.InputSchema)),
		}})
	}
	return result, nil
}

// 已验证的初期能力只覆盖内置的四个精确模型 ID，不根据中转展示名推断。
// https://platform.claude.com/docs/en/build-with-claude/thinking
// https://platform.claude.com/docs/en/build-with-claude/effort
func builtinModelPolicy(model string) (defaultEffort string, canDisable, canForce, known bool) {
	switch model {
	case "claude-opus-5-5":
		return "medium", false, false, true
	case "claude-sonnet-5-5", "claude-fable-5-1":
		return "high", false, false, true
	case "claude-haiku-5-5":
		return "medium", true, true, true
	default:
		return "", false, false, false
	}
}

func (p Provider) applyOptions(params *sdk.MessageNewParams, model adapter.ModelInfo, opts *adapter.CallOptions) error {
	effort, canDisable, canForce, known := builtinModelPolicy(params.Model)
	if known && len(model.ReasoningLevels) > 0 {
		effort = model.ReasoningLevels[0]
	}
	temperature := p.Temperature
	if opts != nil && opts.Temperature != nil {
		temperature = opts.Temperature
	}
	if temperature != nil {
		value := *temperature
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 1 {
			return fmt.Errorf("anthropic temperature must be between 0 and 1")
		}
		if known {
			if value != 1 {
				return fmt.Errorf("model %q does not support a non-default temperature", params.Model)
			}
		} else {
			params.Temperature = sdk.Float(value)
		}
	}
	thinking := known
	if opts != nil {
		if !known && (opts.ThinkingEnabled != nil || opts.ReasoningEffort != nil) {
			return fmt.Errorf("thinking configuration is not defined for model %q", params.Model)
		}
		if opts.ReasoningEffort != nil {
			effort = *opts.ReasoningEffort
		}
		if opts.ThinkingEnabled != nil {
			thinking = *opts.ThinkingEnabled
		}
	}
	if known {
		levels := []string{"low", "medium", "high", "xhigh", "max"}
		if !slices.Contains(levels, effort) || (len(model.ReasoningLevels) > 0 && !slices.Contains(model.ReasoningLevels, effort)) {
			return fmt.Errorf("unsupported reasoning effort %q for model %q", effort, params.Model)
		}
		params.OutputConfig.Effort = sdk.OutputConfigEffort(effort)
		if thinking {
			params.Thinking.OfAdaptive = &sdk.ThinkingConfigAdaptiveParam{Display: sdk.ThinkingConfigAdaptiveDisplaySummarized}
		} else {
			if !canDisable || effort == "xhigh" || effort == "max" {
				return fmt.Errorf("model %q cannot disable thinking at effort %q", params.Model, effort)
			}
			params.Thinking.OfDisabled = &sdk.ThinkingConfigDisabledParam{}
		}
	}
	if opts != nil && opts.ToolChoice != nil {
		choice, err := buildToolChoice(opts.ToolChoice, params.Tools)
		if err != nil {
			return err
		}
		if known && !canForce && (choice.OfAny != nil || choice.OfTool != nil) {
			return fmt.Errorf("model %q does not support forced tool use", params.Model)
		}
		params.ToolChoice = choice
	}
	return nil
}

// buildToolChoice 映射现有 OpenAI 形状，不扩展公共 any 契约或透传未知对象。
func buildToolChoice(value any, tools []sdk.ToolUnionParam) (sdk.ToolChoiceUnionParam, error) {
	var choice sdk.ToolChoiceUnionParam
	mode, stringChoice := value.(string)
	name := ""
	if !stringChoice {
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
		mode, name = "function", named.Function.Name
	}
	if len(tools) == 0 {
		return choice, fmt.Errorf("tool_choice requires tool definitions")
	}
	switch mode {
	case "auto":
		choice.OfAuto = &sdk.ToolChoiceAutoParam{}
	case "none":
		choice.OfNone = &sdk.ToolChoiceNoneParam{}
	case "required":
		choice.OfAny = &sdk.ToolChoiceAnyParam{}
	case "function":
		found := slices.ContainsFunc(tools, func(tool sdk.ToolUnionParam) bool { return tool.OfTool != nil && tool.OfTool.Name == name })
		if !found {
			return choice, fmt.Errorf("tool_choice references undefined tool %q", name)
		}
		choice.OfTool = &sdk.ToolChoiceToolParam{Name: name}
	default:
		return choice, fmt.Errorf("unsupported tool_choice %q", mode)
	}
	return choice, nil
}
