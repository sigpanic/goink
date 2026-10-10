package agent

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	wails "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/sigpanic/goink/internal/activity"
	"github.com/sigpanic/goink/internal/llm"
)

// InitRunningTokens 对初始消息列表逐条 token 计数，返回按 role 的分组统计。
func (a *Agent) InitRunningTokens(messages []map[string]any) map[string]int {
	tokens := map[string]int{"system": 0, "user": 0, "assistant": 0, "tool": 0}
	for _, m := range messages {
		role, _ := m["role"].(string)
		if _, ok := tokens[role]; ok {
			n, err := llm.CountMessageTokens(m)
			if err != nil {
				a.logger.Warn("token count failed", "role", role, "err", err)
			}
			tokens[role] += n
		}
	}
	return tokens
}

// updateUsage 计算 usage_ratio + 分角色 detail → 持久化到 session + 推送前端。
// 缓存命中 token 做 session 级累计，每次请求累加到历史值上。
func (a *Agent) updateUsage(ctx context.Context, apiUsage map[string]any, runningTokens map[string]int, opts RunOptions) {
	a.recordTokenUsage(ctx, apiUsage, opts)
	localTotal := runningTokens["system"] + runningTokens["user"] + runningTokens["assistant"] + runningTokens["tool"]
	apiTotal, _ := apiUsage["total_tokens"].(float64)

	detail := make(map[string]int)
	for role, tokens := range runningTokens {
		if localTotal > 0 && int(apiTotal) > 0 {
			detail[role] = int(float64(tokens) * apiTotal / float64(localTotal))
		} else {
			detail[role] = tokens
		}
	}

	// 累计 session 级缓存命中/未命中 token
	accHit, accMiss := float64(0), float64(0)
	if sess, err := a.session.GetSession(ctx, opts.SessionID); err == nil && sess.Usage != "" {
		var old map[string]any
		if json.Unmarshal([]byte(sess.Usage), &old) == nil {
			if v, _ := old["prompt_cache_hit_tokens"].(float64); v > 0 {
				accHit = v
			}
			if v, _ := old["prompt_cache_miss_tokens"].(float64); v > 0 {
				accMiss = v
			}
		}
	}
	if hit, _ := apiUsage["prompt_cache_hit_tokens"].(float64); hit > 0 {
		accHit += hit
	}
	if miss, _ := apiUsage["prompt_cache_miss_tokens"].(float64); miss > 0 {
		accMiss += miss
	}

	usage := map[string]any{
		"prompt_tokens":            apiUsage["prompt_tokens"],
		"completion_tokens":        apiUsage["completion_tokens"],
		"total_tokens":             apiUsage["total_tokens"],
		"prompt_cache_hit_tokens":  accHit,
		"prompt_cache_miss_tokens": accMiss,
		"context_window":           opts.Model.ContextWindow,
		"detail":                   detail,
	}
	if opts.Model.ContextWindow > 0 {
		usage["usage_ratio"] = float64(int(apiTotal)) / float64(opts.Model.ContextWindow) * 100
	}
	if accHit+accMiss > 0 {
		usage["cache_hit_ratio"] = accHit / (accHit + accMiss) * 100
	}

	if b, err := json.Marshal(usage); err == nil {
		if err := a.session.UpdateSessionUsage(ctx, opts.SessionID, string(b)); err != nil {
			a.logger.Warn("持久化 usage 失败", "err", err)
		}
	}

	wails.EventsEmit(ctx, "agent:"+strconv.Itoa(opts.TurnID), AgentEvent{
		TurnID:    opts.TurnID,
		Type:      EventUsage,
		Usage:     usage,
		Timestamp: time.Now(),
	})
}

// recordTokenUsage 仅累计原始单次 usage，缓存和推理明细不再加进总量。
func (a *Agent) recordTokenUsage(ctx context.Context, apiUsage map[string]any, opts RunOptions) {
	if opts.Model == nil {
		return
	}
	input, hasInput := apiUsage["prompt_tokens"].(float64)
	output, hasOutput := apiUsage["completion_tokens"].(float64)
	total, hasTotal := apiUsage["total_tokens"].(float64)
	hit, hasHit := apiUsage["prompt_cache_hit_tokens"].(float64)
	miss, hasMiss := apiUsage["prompt_cache_miss_tokens"].(float64)
	if !hasHit {
		details, _ := apiUsage["prompt_tokens_details"].(map[string]any)
		hit, hasHit = details["cached_tokens"].(float64)
	}
	details, _ := apiUsage["completion_tokens_details"].(map[string]any)
	reasoning, hasReasoning := details["reasoning_tokens"].(float64)
	if !hasInput && !hasOutput && !hasTotal && !hasHit && !hasMiss && !hasReasoning {
		return
	}
	if !hasTotal && hasInput && hasOutput {
		total = input + output
	}
	purpose := "chat"
	if opts.AgentType != "" && opts.AgentType != "main" {
		purpose = opts.AgentType
	}
	activity.NewStore(a.db, a.logger).AddLLMUsage(ctx, opts.ProviderName, opts.Model.ID, purpose, activity.TokenUsage{
		InputTokens:     int64(input),
		OutputTokens:    int64(output),
		TotalTokens:     int64(total),
		CacheHitTokens:  int64(hit),
		CacheMissTokens: int64(miss),
		ReasoningTokens: int64(reasoning),
	})
}
