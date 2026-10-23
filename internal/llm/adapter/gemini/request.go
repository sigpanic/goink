package gemini

import (
	"fmt"
	"slices"
	"strings"

	types "google.golang.org/genai/interactions/models/interactions"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

// buildRequest 只构造无状态参数，不发请求或修改调用方的数据。
func (p Provider) buildRequest(req adapter.Request) (types.CreateModelInteraction, error) {
	store, background := false, false
	params := types.CreateModelInteraction{Model: types.Model(req.Model), Store: &store, Background: &background}
	if strings.TrimSpace(req.Model) == "" {
		return params, fmt.Errorf("gemini model must not be empty")
	}
	var policy, configured adapter.ModelInfo
	for _, model := range builtin["Google"].Models {
		if model.ID == req.Model {
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
	params.GenerationConfig = &types.GenerationConfig{MaxOutputTokens: &maxTokens}
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
	if len(params.Input.ArrayOfStep) == 0 {
		return params, fmt.Errorf("gemini history has no input steps")
	}
	return params, nil
}

func (p Provider) applyOptions(params *types.CreateModelInteraction, policy, configured adapter.ModelInfo, opts *adapter.CallOptions) error {
	temperature := p.Temperature
	if opts != nil && opts.Temperature != nil {
		temperature = opts.Temperature
	}
	// SDK 仍保留已弃用字段，但当前 v1 schema 不再提供 temperature。
	// https://ai.google.dev/api/interactions-api-v1
	if temperature != nil {
		return fmt.Errorf("temperature configuration is not supported by Gemini Interactions v1")
	}
	known := policy.ID != ""
	thinking := known && params.Model != "gemini-2.5-flash-lite"
	effort := ""
	if known && len(policy.ReasoningLevels) > 0 {
		effort = policy.ReasoningLevels[0]
		if len(configured.ReasoningLevels) > 0 {
			effort = configured.ReasoningLevels[0]
		}
	}
	if opts != nil {
		if !known && (opts.ThinkingEnabled != nil || opts.ReasoningEffort != nil) {
			return fmt.Errorf("thinking configuration is not defined for model %q", params.Model)
		}
		if opts.ReasoningEffort != nil {
			effort, thinking = *opts.ReasoningEffort, true
		}
		if opts.ThinkingEnabled != nil {
			thinking = *opts.ThinkingEnabled
			if !thinking && (params.Model != "gemini-2.5-flash-lite" || opts.ReasoningEffort != nil) {
				return fmt.Errorf("model %q cannot disable thinking through Interactions", params.Model)
			}
		}
		if opts.ToolChoice != nil {
			choice, err := buildToolChoice(opts.ToolChoice, params.Tools)
			if err != nil {
				return err
			}
			params.GenerationConfig.ToolChoice = &choice
		}
	}
	if thinking {
		if !slices.Contains(policy.ReasoningLevels, effort) || (len(configured.ReasoningLevels) > 0 && !slices.Contains(configured.ReasoningLevels, effort)) {
			return fmt.Errorf("unsupported reasoning effort %q for model %q", effort, params.Model)
		}
		level, summaries := types.ThinkingLevel(effort), types.ThinkingSummariesAuto
		params.GenerationConfig.ThinkingLevel = &level
		params.GenerationConfig.ThinkingSummaries = &summaries
	}
	return nil
}
