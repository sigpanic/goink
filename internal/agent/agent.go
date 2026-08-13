package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	wails "github.com/wailsapp/wails/v2/pkg/runtime"

	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/agentcfg"
	"github.com/sigpanic/goink/internal/approval"
	"github.com/sigpanic/goink/internal/llm"
	"github.com/sigpanic/goink/internal/mcp_tools"
	"github.com/sigpanic/goink/internal/search"
	"github.com/sigpanic/goink/internal/session"
	"github.com/sigpanic/goink/internal/skill"
	"github.com/sigpanic/goink/internal/storage"
)

// Agent 是对话编排核心，持有运行所需的所有基础设施。
type Agent struct {
	llm           *llm.Client
	registry      *mcp_tools.Registry
	session       *session.Store
	db            *gorm.DB
	approver      approval.Approver
	logger        *slog.Logger
	skillStore    *skill.Store
	searchService atomic.Pointer[search.Service]
	cancelMgr     *CancelManager
}

// RunOptions 是单次 Run() 的参数。
type RunOptions struct {
	TurnID          int
	SessionID       string
	NovelID         int64
	Messages        []map[string]any
	AllowedTools    map[string]bool
	ActiveVersion   int
	SubAgentVersion int // 子 Agent 内存版本计数器，不持久化
	Model           *llm.ModelInfo
	ProviderName    string
	AgentType       string
	SubTaskID       string // 子 Agent 事件路由 ID
	EventSeq        *int   // 共享事件序号，nil 时自建（主Agent）；子Agent传入父的指针
	MaxTurns        int
	ReasoningEffort string // 用户选择的推理等级
}

// New 创建 Agent 实例。
func New(llmClient *llm.Client, registry *mcp_tools.Registry, session *session.Store, db *gorm.DB, approver approval.Approver, logger *slog.Logger, skillStore *skill.Store, cancelMgr *CancelManager) *Agent {
	return &Agent{
		llm:        llmClient,
		registry:   registry,
		session:    session,
		db:         db,
		approver:   approver,
		logger:     logger,
		skillStore: skillStore,
		cancelMgr:  cancelMgr,
	}
}

// SetSearchService 设置搜索服务，在搜索服务初始化完成后由 App 调用。
func (a *Agent) SetSearchService(s *search.Service) { a.searchService.Store(s) }

// RegisterCancel 注册一个可取消的对话。
func (a *Agent) RegisterCancel(sessionID string, cancel context.CancelFunc) {
	a.cancelMgr.Register(CancelPrefixChat+sessionID, cancel)
}

// UnregisterCancel 对话结束后清理，只删不 cancel。
func (a *Agent) UnregisterCancel(sessionID string) {
	a.cancelMgr.Unregister(CancelPrefixChat + sessionID)
}

// Cancel 取消一个正在进行的对话。
func (a *Agent) Cancel(sessionID string) {
	a.cancelMgr.Cancel(CancelPrefixChat + sessionID)
}

// RunSubAgent 启动子 Agent 并返回最终报告文本。
func (a *Agent) RunSubAgent(ctx context.Context, parentOpts RunOptions, req mcp_tools.SubAgentRequest) (string, error) {
	at := agentTypeFromString(req.AgentType)
	sysPrompt := agentcfg.AgentIdentity(at)
	allowed := agentcfg.Allowlist(at)

	msgs := []map[string]any{
		{"role": "system", "content": sysPrompt},
	}
	// subagent 不需要 always/catalog（只读上下文），只注入 Profile + State
	if profile, err := agentcfg.NovelProfile(ctx, a.db, req.NovelID); err == nil && profile != "" {
		msgs = append(msgs, map[string]any{"role": "system", "content": profile})
	}
	if state, err := agentcfg.NovelState(ctx, a.db, req.NovelID); err == nil && state != "" {
		msgs = append(msgs, map[string]any{"role": "system", "content": state})
	}
	msgs = append(msgs, map[string]any{"role": "user", "content": req.Instruction})

	subOpts := RunOptions{
		TurnID:          parentOpts.TurnID,
		SessionID:       parentOpts.SessionID,
		NovelID:         req.NovelID,
		Messages:        msgs,
		AllowedTools:    allowed,
		ActiveVersion:   parentOpts.ActiveVersion,
		AgentType:       req.AgentType,
		SubTaskID:       req.ToolID,
		EventSeq:        parentOpts.EventSeq,
		MaxTurns:        100,
		Model:           parentOpts.Model,
		ProviderName:    parentOpts.ProviderName,
		ReasoningEffort: parentOpts.ReasoningEffort,
	}
	result, err := a.Run(ctx, subOpts)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			// 用户取消：返回明确文案（与 flushInterruptedTools 一致），供 run_subagent 存入 ToolResult.Error 供历史回放显示
			return result.FinalText, fmt.Errorf("%s", "操作被中断")
		}
		// 友好化 error，与 EventError 的 ErrMsg=FriendlyError(event.Error) 保持一致
		// 供 run_subagent 工具存入 ToolResult.Error，历史回放与实时流式显示一致
		// 截断到 200 字符：此处 error 经 run_subagent 转 ToolResult 后作为 tool response 进入 LLM 上下文，
		// 截断防服务商返回的过长 message 传播为注入载体（其余 AgentEvent.ErrMsg 调用方不进 LLM，不截断）
		return result.FinalText, fmt.Errorf("%s", truncateErrMsg(FriendlyError(err), 200))
	}
	return result.FinalText, nil
}

// agentTypeFromString 将字符串转为 AgentType。
func agentTypeFromString(s string) agentcfg.AgentType {
	switch s {
	case "review":
		return agentcfg.ReviewAgent
	case "memory":
		return agentcfg.MemoryAgent
	default:
		return agentcfg.MainAgent
	}
}

// Run 执行 Agent 循环，返回最终文本和轮数。
func (a *Agent) Run(ctx context.Context, opts RunOptions) (AgentLoopResult, error) {
	if opts.MaxTurns <= 0 {
		opts.MaxTurns = 100
	}
	if opts.Model == nil {
		return AgentLoopResult{}, errors.New("agent: Model is required in RunOptions")
	}

	ctx = storage.WithTurn(ctx, opts.SessionID, opts.TurnID)

	loopCount := 0
	var responseBuffer strings.Builder
	var thinkingBuffer strings.Builder
	isThinking := false
	recentPatterns := make([]string, 0, 6)
	tracker := newInterruptTracker() // 工具连续失败计数（system 5/args 5/全局 10 中断）
	runningTokens := a.InitRunningTokens(opts.Messages)
	tools := a.registry.OpenAI(opts.AllowedTools)
	agentEventName := "agent:" + strconv.Itoa(opts.TurnID)
	eventSeq := opts.EventSeq
	if eventSeq == nil {
		seq := 0
		eventSeq = &seq
		opts.EventSeq = eventSeq //回写 子agent才能共享这个值
	}
	emit := func(event AgentEvent) {
		*eventSeq++
		event.Seq = *eventSeq
		if event.Timestamp.IsZero() {
			event.Timestamp = time.Now()
		}
		event.SubTaskID = opts.SubTaskID
		wails.EventsEmit(ctx, agentEventName, event)
	}

	canceled := false    // 用户手动取消（ctx.Done）
	interrupted := false // 系统主动中断（工具连续失败/MaxTurns/死循环）
	var interruptErr error
	normalEnd := false // 正常结束（无 tool 输出 break），区分 MaxTurns 耗尽

	for loopCount < opts.MaxTurns {
		toolOutputs := make([]toolOutput, 0)
		pendingInjects := make(map[string][]mcp_tools.InjectMessage)
		// P2: 本轮 LLM 调用重试计数（不消耗 MaxTurns）
		retryCount := 0
		// tool_args_invalid 首次提醒标志：同一 turn 内只加一次 reminder，
		// 避免重试耗尽后 DB 残留多条孤立 user reminder 污染下轮 LLM 上下文
		reminderAdded := false
		// token 预算检查：每轮开始时，超限触发压缩
		if opts.Model.ContextWindow > 0 && float64(sumRunningTokens(runningTokens))/float64(opts.Model.ContextWindow) >= 0.8 {
			a.logger.Warn("token budget exceeded, triggering compression",
				"estimated", sumRunningTokens(runningTokens),
				"context_window", opts.Model.ContextWindow,
				"ratio", fmt.Sprintf("%.1f%%", float64(sumRunningTokens(runningTokens))/float64(opts.Model.ContextWindow)*100),
				"agent_type", opts.AgentType,
			)
			var compressErr error
			if opts.AgentType == "main" {
				compressErr = a.Compress(ctx, &opts, runningTokens)
			} else {
				compressErr = a.compressInMemory(ctx, &opts, runningTokens)
			}
			if compressErr != nil {
				a.logger.Warn("compression failed, continuing with original context", "err", compressErr)
			}
		}

		callOpts := &llm.CallOptions{}
		if opts.ReasoningEffort != "" {
			callOpts.ReasoningEffort = &opts.ReasoningEffort
		}
	RETRY_STREAM:
		// 记录本轮 streamLoop 第一个事件 seq（前端据此清空本轮 partial segments）
		// +1 是因为 emit 闭包是 `*eventSeq++; event.Seq = *eventSeq`（先自增再赋值）
		streamStartSeq := *eventSeq + 1
		stream := a.llm.ChatStream(ctx, opts.ProviderName, opts.Messages, tools, opts.Model.ID, callOpts)

		// ---- SSE 流处理 ----
	streamLoop:
		for {
			select {
			case <-ctx.Done():
				canceled = true
				a.flushInterruptedTools(stream, &opts, &toolOutputs)
				break streamLoop

			case event, ok := <-stream:
				if !ok {
					break streamLoop
				}

				switch event.Type {
				case llm.EventThinking:
					isThinking = true
					thinkingBuffer.WriteString(event.Data)
					emit(AgentEvent{
						TurnID: opts.TurnID, Type: EventThinking,
						Data: event.Data, Timestamp: time.Now(),
					})

				case llm.EventContent:
					if isThinking {
						emit(AgentEvent{
							TurnID: opts.TurnID, Type: EventThinkingDone, Timestamp: time.Now(),
						})
						isThinking = false
					}
					responseBuffer.WriteString(event.Data)
					emit(AgentEvent{
						TurnID: opts.TurnID, Type: EventContent,
						Data: event.Data, Timestamp: time.Now(),
					})

				case llm.EventToolCallStart:
					if isThinking {
						emit(AgentEvent{
							TurnID: opts.TurnID, Type: EventThinkingDone, Timestamp: time.Now(),
						})
						isThinking = false
					}
					name := event.Delta.ToolName
					id := event.Delta.ToolID
					display := a.buildDisplay(name, nil, mcp_tools.PhaseSelected, opts.NovelID)
					emit(AgentEvent{
						TurnID: opts.TurnID, Type: EventToolCall,
						ToolName: name, ToolID: id, Phase: "selected",
						DisplayText: display.DisplayText, ActivityKind: display.ActivityKind,
						Metadata: display.Metadata, Timestamp: time.Now(),
					})

				case llm.EventToolCallEnd:
					name := event.Delta.ToolName
					id := event.Delta.ToolID
					rawArgs := event.Delta.ArgumentsJSON

					args := parseArgs(rawArgs)
					display := a.buildDisplay(name, args, mcp_tools.PhaseExecuting, opts.NovelID)
					emit(AgentEvent{
						TurnID: opts.TurnID, Type: EventToolCall,
						ToolName: name, ToolID: id, Phase: "executing",
						ToolArgs: args, DisplayText: display.DisplayText, ActivityKind: display.ActivityKind,
						Metadata: display.Metadata, Timestamp: time.Now(),
					})

					tc := mcp_tools.ToolContext{
						DB:       a.db,
						NovelID:  opts.NovelID,
						ToolID:   id,
						Approver: a.approver,
						EmitApproval: func(toolID string, approvalType string, payload map[string]any) {
							emit(AgentEvent{
								TurnID: opts.TurnID, Type: EventToolCall,
								ToolName: name, ToolID: toolID, Phase: "awaiting_approval",
								Metadata: map[string]any{
									"approval_type": approvalType,
									"payload":       payload,
								},
								Timestamp: time.Now(),
							})
						},
						RunSubAgent: func(ctx context.Context, req mcp_tools.SubAgentRequest) (string, error) {
							return a.RunSubAgent(ctx, opts, req)
						},
						SkillStore:    a.skillStore,
						SearchService: a.searchService.Load(),
						WebSearch:     a.buildWebSearch(),
						Logger:        a.logger,
					}
					result := a.registry.Execute(ctx, name, rawArgs, tc, opts.AllowedTools)
					a.logger.Info("tool executed", "tool", name, "success", result.Success, "phase", map[bool]string{true: "completed", false: "failed"}[result.Success])

					phase := "completed"
					if !result.Success {
						phase = "failed"
					}
					display = a.buildDisplay(name, args, displayPhase(phase), opts.NovelID)
					metadata := display.Metadata
					if resultDataMergeTools[name] && result.Success && result.Data != nil {
						if metadata == nil {
							metadata = make(map[string]any)
						}
						for k, v := range result.Data {
							metadata[k] = v
						}
					}
					emit(AgentEvent{
						TurnID: opts.TurnID, Type: EventToolCall,
						ToolName: name, ToolID: id, Phase: phase,
						ToolArgs: args, Success: result.Success, ErrMsg: result.Error,
						DisplayText: display.DisplayText, ActivityKind: display.ActivityKind,
						Metadata: metadata, Timestamp: time.Now(),
					})

					// 失败计数：system 5 次/args 5 次/全局连续 10 次触发中断
					if result.Success {
						tracker.recordSuccess(name)
					} else {
						if interrupt, reason := tracker.recordFailure(name, result.ErrKind); interrupt {
							interrupted = true
							interruptErr = reason
							a.flushInterruptedTools(stream, &opts, &toolOutputs)
							break streamLoop
						}
					}

					// 暂存 inject
					if len(result.Inject) > 0 {
						pendingInjects[id] = result.Inject
					}

					toolOutputs = append(toolOutputs, toolOutput{name: name, id: id, rawArgs: rawArgs, result: result, displayText: display.DisplayText, activityKind: display.ActivityKind})

				case llm.EventUsage:
					a.updateUsage(ctx, event.Usage, runningTokens, opts)

				case llm.EventError:
					// 关键节点日志：agent 收到 EventError
					var apiErr *llm.APIError
					if errors.As(event.Error, &apiErr) {
						a.logger.Warn("agent event error",
							"err", event.Error,
							"status_code", apiErr.StatusCode,
							"retryable", apiErr.Retryable,
							"retry_after_ms", apiErr.RetryAfter.Milliseconds())
					} else {
						a.logger.Warn("agent event error", "err", event.Error)
					}

					// tool arguments 解析失败：丢弃所有输出 + appendMsg reminder + 走自动重试
					// 与 P2 重试的区别：需要 appendMsg user reminder 告诉 LLM 刚才 tool call 解析失败
					if apiErr != nil && apiErr.Kind == "tool_args_invalid" {
						// 丢弃本轮所有输出（content + thinking + tool_calls）
						responseBuffer.Reset()
						thinkingBuffer.Reset()
						isThinking = false

						// appendMsg user reminder，告诉 LLM 刚才 tool call 解析失败
						// 仅首次 tool_args_invalid 时追加，避免重试耗尽后残留多条孤立 reminder
						if !reminderAdded {
							reminder := buildToolArgsReminder(apiErr.Message)
							a.appendMsg("user", reminder, "", nil, &opts, runningTokens)
							reminderAdded = true
						}

						// 走自动重试路径（复用 P2 退避机制）
						if retryCount < maxRetries {
							retryCount++
							backoff := computeBackoff(retryCount, 0)
							a.logger.Warn("agent retrying llm call (tool_args_invalid)",
								"attempt", retryCount,
								"max_retries", maxRetries,
								"backoff_ms", backoff.Milliseconds())

							emit(AgentEvent{
								TurnID:       opts.TurnID,
								Type:         EventRetrying,
								Attempt:      retryCount,
								MaxRetries:   maxRetries,
								BackoffMs:    backoff.Milliseconds(),
								ErrMsg:       FriendlyError(event.Error),
								ClearFromSeq: streamStartSeq,
								Timestamp:    time.Now(),
							})

							timer := time.NewTimer(backoff)
							select {
							case <-ctx.Done():
								timer.Stop()
								return AgentLoopResult{FinalText: responseBuffer.String(), ThinkingContent: thinkingBuffer.String(), TurnCount: loopCount}, ctx.Err()
							case <-timer.C:
							}
							goto RETRY_STREAM
						}

						// 重试次数耗尽 → 失败兜底（不保存 partial，已经 Reset）
						// 主 turn 结局由 chat.go 统一 emit（chat:turn_outcome），子 agent 仍走 EventError 路由
						if opts.SubTaskID != "" {
							emit(AgentEvent{
								TurnID: opts.TurnID, Type: EventError,
								ErrMsg: FriendlyError(event.Error), Timestamp: time.Now(),
							})
						}
						return AgentLoopResult{FinalText: responseBuffer.String(), ThinkingContent: thinkingBuffer.String(), TurnCount: loopCount}, event.Error
					}

					// P2: 可恢复错误重试（不重试 ctx.Canceled / 不可恢复错误 / 非 *APIError）
					if apiErr != nil && apiErr.Retryable && retryCount < maxRetries {
						retryCount++
						backoff := computeBackoff(retryCount, apiErr.RetryAfter)
						a.logger.Warn("agent retrying llm call",
							"attempt", retryCount,
							"max_retries", maxRetries,
							"backoff_ms", backoff.Milliseconds(),
							"status_code", apiErr.StatusCode)

						// 通知前端：正在重试（附带本轮起点 seq，前端据此清空本轮 partial segments）
						emit(AgentEvent{
							TurnID:       opts.TurnID,
							Type:         EventRetrying,
							Attempt:      retryCount,
							MaxRetries:   maxRetries,
							BackoffMs:    backoff.Milliseconds(),
							ErrMsg:       FriendlyError(event.Error),
							ClearFromSeq: streamStartSeq,
							Timestamp:    time.Now(),
						})

						// 清空本轮 buffers，避免重试后 partial 内容重复累加。
						// Reset 等价于恢复到本轮 streamLoop 开始状态（buffer 在 streamLoop 开始时必为空：
						// 首次进入是外层 turn 顶部已确保空；重试进入是上次末已 Reset）。
						responseBuffer.Reset()
						thinkingBuffer.Reset()
						isThinking = false
						// toolOutputs / pendingInjects 在本轮 streamLoop 内，
						// EventError 发生时本轮 tool 还未执行（toolOutputs 为空），
						// 不需要清空

						timer := time.NewTimer(backoff)
						select {
						case <-ctx.Done():
							// 用户在退避期间取消 → 走 user_stopped 路径
							// 不 emit EventError（前端已设 status='stopped'）
							timer.Stop()
							return AgentLoopResult{FinalText: responseBuffer.String(), ThinkingContent: thinkingBuffer.String(), TurnCount: loopCount}, ctx.Err()
						case <-timer.C:
						}
						goto RETRY_STREAM
					}

					// 失败兜底：不可重试 或 重试次数耗尽 → 保存 partial 后返回
					// 主 turn 结局由 chat.go 统一 emit（chat:turn_outcome），子 agent 仍走 EventError 路由
					if opts.SubTaskID != "" {
						emit(AgentEvent{
							TurnID: opts.TurnID, Type: EventError,
							ErrMsg: FriendlyError(event.Error), Timestamp: time.Now(),
						})
					}
					finalText := responseBuffer.String()
					thinkingContent := thinkingBuffer.String()
					if responseBuffer.Len() > 0 || thinkingBuffer.Len() > 0 {
						a.appendMsg("assistant", finalText, thinkingContent,
							nil, &opts, runningTokens)
					}
					return AgentLoopResult{FinalText: finalText, ThinkingContent: thinkingContent, TurnCount: loopCount}, event.Error
				}
			}
		}

		// ---- 流结束，判断是否有工具调用 ----
		if len(toolOutputs) == 0 {
			if isThinking {
				emit(AgentEvent{
					TurnID: opts.TurnID, Type: EventThinkingDone, Timestamp: time.Now(),
				})
			}
			if responseBuffer.Len() > 0 || thinkingBuffer.Len() > 0 {
				a.appendMsg("assistant", responseBuffer.String(), thinkingBuffer.String(),
					nil, &opts, runningTokens)
			} //此处持久化最终信息，主agent和subagent共享避免遗漏
			normalEnd = true
			break
		}

		// 1. assistant + tool_calls + tool_displays

		a.appendMsg("assistant", responseBuffer.String(), thinkingBuffer.String(),
			map[string]any{
				"tool_calls":    buildToolCalls(toolOutputs),
				"tool_displays": buildToolDisplay(toolOutputs),
			}, &opts, runningTokens)

		// 2. tool 结果
		for _, to := range toolOutputs {
			a.appendMsg("tool", to.resultJSON(),
				"", map[string]any{"tool_call_id": to.id, "tool_name": to.name},
				&opts, runningTokens)
		}

		// 3. inject（role=user，<system-reminder> 包裹）
		for _, to := range toolOutputs {
			for _, inj := range pendingInjects[to.id] {
				content := "<system-reminder>\n" + inj.Content + "\n</system-reminder>"
				a.appendMsg(inj.Role, content, "", nil, &opts, runningTokens)
			}
		}

		if canceled || interrupted {
			break
		}

		// 4. 死循环检测
		patterns := append(recentPatterns, toolPattern(toolOutputs))
		if len(patterns) > 6 {
			patterns = patterns[1:]
		}
		if isStuckLoop(patterns, toolOutputs, loopCount) {
			// 命中死循环：直接中断对话（不再靠提醒继续）
			interrupted = true
			interruptErr = &LoopInterrupt{Reason: "检测到重复调用循环，已中断对话"}
			emit(AgentEvent{
				TurnID: opts.TurnID, Type: EventToolCall, Phase: "loop_detected", Timestamp: time.Now(),
			})
			break
		}
		recentPatterns = patterns

		// 清空当前轮缓冲
		thinkingBuffer.Reset()
		responseBuffer.Reset()
		loopCount++
	}

	if canceled {
		return AgentLoopResult{FinalText: responseBuffer.String(), ThinkingContent: thinkingBuffer.String(), TurnCount: loopCount}, ctx.Err()
	}
	if interrupted {
		return AgentLoopResult{FinalText: responseBuffer.String(), ThinkingContent: thinkingBuffer.String(), TurnCount: loopCount}, interruptErr
	}
	if !normalEnd {
		// MaxTurns 耗尽：循环条件退出，非正常 break，走 system_interrupted
		return AgentLoopResult{FinalText: responseBuffer.String(), ThinkingContent: thinkingBuffer.String(), TurnCount: loopCount}, &MaxTurnsInterrupt{Reason: "已达最大轮数，对话结束"}
	}
	return AgentLoopResult{FinalText: responseBuffer.String(), ThinkingContent: thinkingBuffer.String(), TurnCount: loopCount}, nil
}

// appendMsg 统一处理消息的内存追加 + 持久化 + token 计数。
// opts 必须传指针，因为 opts.Messages 需要被追加（Go 切片传值会丢失 append）。
func (a *Agent) appendMsg(role, content, thinkingContent string, extra map[string]any, opts *RunOptions, runningTokens map[string]int) {
	msg := &session.Message{
		SessionID:       opts.SessionID,
		TurnID:          opts.TurnID,
		AgentType:       opts.AgentType,
		SubTaskID:       opts.SubTaskID,
		Role:            role,
		Content:         content,
		ThinkingContent: thinkingContent,
		ExtraMetadata:   extraJSON(extra),
		Version:         opts.ActiveVersion,
		ToAPI:           opts.AgentType == "main",
		ToFrontend:      role == "assistant",
	}
	a.logger.Debug("appendMsg", "role", role, "agentType", opts.AgentType, "subTaskID", opts.SubTaskID, "turnID", opts.TurnID)
	if err := a.db.Create(msg).Error; err != nil {
		a.logger.Error("持久化消息失败", "role", role, "turnID", opts.TurnID, "err", err)
	}

	apiFormat := msg.ToAPIFormat(a.logger)
	opts.Messages = append(opts.Messages, apiFormat)
	n, err := llm.CountMessageTokens(apiFormat)
	if err != nil {
		a.logger.Warn("token count failed", "role", role, "err", err)
	}
	runningTokens[role] += n
}

// sumRunningTokens 计算各角色 token 总数。
func sumRunningTokens(tokens map[string]int) int {
	total := 0
	for _, n := range tokens {
		total += n
	}
	return total
}

// buildToolArgsReminder 构造 tool arguments 解析失败的 reminder 消息。
// 格式：<system-reminder> 包裹，告诉 LLM 刚才 tool call 解析失败，附带错误信息，提示转义。
// errMsg 来自 stream.go 的 buildToolArgsInvalidMsg（已截断 200 字符）。
func buildToolArgsReminder(errMsg string) string {
	return fmt.Sprintf("<system-reminder>\n你刚才尝试了工具调用但是解析错误：%s\n请重新调用，注意字符串值里的双引号需要用 \\\" 转义\n</system-reminder>", errMsg)
}

// displayPhase 将 completed/failed 字符串转为 DisplayPhase。
func displayPhase(phase string) mcp_tools.DisplayPhase {
	switch phase {
	case "completed":
		return mcp_tools.PhaseCompleted
	case "failed":
		return mcp_tools.PhaseFailed
	}
	return mcp_tools.PhaseCompleted
}

// buildWebSearch 构建 WebSearch 闭包，从当前 providers 中读取 DeepSeek 配置。
func (a *Agent) buildWebSearch() func(ctx context.Context, query string) (*llm.WebSearchResult, error) {
	providers := a.llm.Providers()
	ds, ok := providers["deepseek"]
	if !ok || ds.APIKey == "" {
		return nil
	}
	apiKey := ds.APIKey
	return func(ctx context.Context, query string) (*llm.WebSearchResult, error) {
		return llm.SearchWeb(ctx, apiKey, "", query)
	}
}

// extraJSON 将 map 序列化为 JSON 字符串存入 ExtraMetadata。
func extraJSON(extra map[string]any) string {
	if len(extra) == 0 {
		return ""
	}
	b, _ := json.Marshal(extra)
	return string(b)
}

// parseArgs 将 JSON args 解析为 map。
func parseArgs(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	return m
}
