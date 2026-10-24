package openairesponses

import (
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

// buildRequest 只构造 SDK 参数，不发送请求或修改调用方数据。
func (p Provider) buildRequest(req adapter.Request) (responses.ResponseNewParams, error) {
	params := responses.ResponseNewParams{
		Model:      req.Model,
		Store:      param.NewOpt(false),
		Background: param.NewOpt(false),
	}
	if strings.TrimSpace(req.Model) == "" {
		return params, fmt.Errorf("OpenAI Responses model must not be empty")
	}
	var policy, configured adapter.ModelInfo
	modelID := req.Model
	if modelID == "gpt-5.6" {
		modelID = "gpt-5.6-sol"
	}
	for _, model := range builtin["OpenAI"].Models {
		if model.ID == modelID {
			policy = model
			break
		}
	}
	for _, model := range p.Models {
		if model.ID == req.Model {
			configured = model
			break
		}
	}
	limit := policy.MaxOutputTokens
	if configured.MaxOutputTokens > 0 && (limit == 0 || configured.MaxOutputTokens < limit) {
		limit = configured.MaxOutputTokens
	}
	maxTokens := limit
	if maxTokens == 0 {
		maxTokens = 4096
	}
	if req.Options != nil && req.Options.MaxTokens != nil {
		maxTokens = *req.Options.MaxTokens
	}
	if maxTokens <= 0 || (limit > 0 && maxTokens > limit) {
		return params, fmt.Errorf("invalid max_tokens %d for model %q", maxTokens, req.Model)
	}
	params.MaxOutputTokens = param.NewOpt(int64(maxTokens))
	var err error
	params.Tools, err = buildTools(req.Tools)
	if err != nil {
		return params, err
	}
	if err := p.applyOptions(&params, policy, configured, req.Options); err != nil {
		return params, err
	}
	if err := p.buildMessages(&params, req.Messages); err != nil {
		return params, err
	}
	if len(params.Input.OfInputItemList) == 0 {
		return params, fmt.Errorf("OpenAI Responses history has no input items")
	}
	return params, nil
}

func (p Provider) applyOptions(params *responses.ResponseNewParams, policy, configured adapter.ModelInfo, opts *adapter.CallOptions) error {
	effort := ""
	if len(policy.ReasoningLevels) > 0 {
		effort = policy.ReasoningLevels[0]
		if len(configured.ReasoningLevels) > 0 {
			effort = configured.ReasoningLevels[0]
		}
	}
	if opts != nil {
		if len(policy.ReasoningLevels) == 0 && (opts.ThinkingEnabled != nil || opts.ReasoningEffort != nil) {
			return fmt.Errorf("reasoning configuration is not defined for model %q", params.Model)
		}
		if opts.ReasoningEffort != nil {
			effort = *opts.ReasoningEffort
		}
		if opts.ThinkingEnabled != nil {
			if !*opts.ThinkingEnabled {
				if opts.ReasoningEffort != nil && effort != "none" {
					return fmt.Errorf("disabled thinking conflicts with reasoning effort %q", effort)
				}
				effort = "none"
			} else if effort == "none" {
				if opts.ReasoningEffort != nil {
					return fmt.Errorf("enabled thinking conflicts with reasoning effort none")
				}
				levels := policy.ReasoningLevels
				if len(configured.ReasoningLevels) > 0 {
					levels = configured.ReasoningLevels
				}
				for _, level := range levels {
					if level != "none" {
						effort = level
						break
					}
				}
				if effort == "none" {
					return fmt.Errorf("model %q has no configured thinking effort", params.Model)
				}
			}
		}
	}
	if len(policy.ReasoningLevels) > 0 {
		if !slices.Contains(policy.ReasoningLevels, effort) ||
			(len(configured.ReasoningLevels) > 0 && !slices.Contains(configured.ReasoningLevels, effort)) {
			return fmt.Errorf("unsupported reasoning effort %q for model %q", effort, params.Model)
		}
		params.Reasoning.Effort = shared.ReasoningEffort(effort)
		if effort != "none" {
			params.Reasoning.Summary = shared.ReasoningSummaryAuto
		}
	}
	temperature := p.Temperature
	if opts != nil && opts.Temperature != nil {
		temperature = opts.Temperature
	}
	if temperature != nil {
		value := *temperature
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 2 {
			return fmt.Errorf("OpenAI Responses temperature must be between 0 and 2")
		}
		// 推理开启时不支持采样参数；未知型号交给服务端校验，不根据名称前缀猜能力。
		// https://developers.openai.com/api/docs/guides/latest-model
		if effort != "" && effort != "none" {
			return fmt.Errorf("model %q does not support temperature at reasoning effort %q", params.Model, effort)
		}
		params.Temperature = param.NewOpt(value)
	}
	if opts != nil && opts.ToolChoice != nil {
		choice, err := buildToolChoice(opts.ToolChoice, params.Tools)
		if err != nil {
			return err
		}
		params.ToolChoice = choice
	}
	return nil
}
