package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"sync/atomic"
	"time"

	wails "github.com/wailsapp/wails/v2/pkg/runtime"

	"gorm.io/gorm"

	"novel/internal/agentcfg"
	"novel/internal/approval"
	"novel/internal/llm"
	"novel/internal/mcp_tools"
	"novel/internal/search"
	"novel/internal/session"
	"novel/internal/skill"
	"novel/internal/storage"
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
	if novelState, err := agentcfg.NovelState(a.db, req.NovelID); err == nil && novelState != "" {
		msgs = append(msgs, map[string]any{"role": "system", "content": novelState})
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
		MaxTurns:        50,
		Model:           parentOpts.Model,
		ProviderName:    parentOpts.ProviderName,
		ReasoningEffort: parentOpts.ReasoningEffort,
	}
	result, err := a.Run(ctx, subOpts)
	return result.FinalText, err
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
		opts.MaxTurns = 50
	}
	if opts.Model == nil {
		return AgentLoopResult{}, errors.New("agent: Model is required in RunOptions")
	}

	ctx = storage.WithTurn(ctx, opts.SessionID, opts.TurnID)

	loopCount := 0
	fullResponse := ""
	responseBuffer := ""
	thinkingBuffer := ""
	isThinking := false
	recentPatterns := make([]string, 0, 6)
	failCnt := make(map[string]int)
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

	interrupted := false

	for loopCount < opts.MaxTurns {
		toolOutputs := make([]toolOutput, 0)
		pendingInjects := make(map[string][]mcp_tools.InjectMessage)
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
		stream := a.llm.ChatStream(ctx, opts.ProviderName, opts.Messages, tools, opts.Model.ID, callOpts)

		// ---- SSE 流处理 ----
	streamLoop:
		for {
			select {
			case <-ctx.Done():
				interrupted = true
				a.flushInterruptedTools(stream, &opts, &toolOutputs)
				break streamLoop

			case event, ok := <-stream:
				if !ok {
					break streamLoop
				}

				switch event.Type {
				case llm.EventThinking:
					isThinking = true
					thinkingBuffer += event.Data
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
					responseBuffer += event.Data
					fullResponse += event.Data
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

					// 失败计数：仅系统异常计入
					if !result.Success && result.ErrKind == "system" {
						failCnt[name]++
					} else {
						failCnt[name] = 0
					}
					if failCnt[name] == 3 {
						content := fmt.Sprintf("<system-reminder>\n工具 %s 已连续失败 3 次，已被禁用，请不要再调用此工具。\n</system-reminder>", name)
						a.appendMsg("user", content, "", nil, &opts, runningTokens)
					}

					// 暂存 inject
					if len(result.Inject) > 0 {
						pendingInjects[id] = result.Inject
					}

					toolOutputs = append(toolOutputs, toolOutput{name: name, id: id, rawArgs: rawArgs, result: result, displayText: display.DisplayText, activityKind: display.ActivityKind})

				case llm.EventUsage:
					a.updateUsage(ctx, event.Usage, runningTokens, opts)

				case llm.EventError:
					// 流错误：保存 partial 后返回
					emit(AgentEvent{
						TurnID: opts.TurnID, Type: EventError,
						ErrMsg: FriendlyError(event.Error), Timestamp: time.Now(),
					})
					if responseBuffer != "" || thinkingBuffer != "" {
						a.appendMsg("assistant", responseBuffer, thinkingBuffer,
							nil, &opts, runningTokens)
					}
					return AgentLoopResult{FinalText: fullResponse, ThinkingContent: thinkingBuffer, TurnCount: loopCount}, event.Error
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
			if responseBuffer != "" || thinkingBuffer != "" {
				a.appendMsg("assistant", responseBuffer, thinkingBuffer,
					nil, &opts, runningTokens)
			} //此处持久化最终信息，主agent和subagent共享避免遗漏
			break
		}

		// 1. assistant + tool_calls + tool_displays

		a.appendMsg("assistant", responseBuffer, thinkingBuffer,
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

		if interrupted {
			break
		}

		// 4. 死循环检测
		patterns := append(recentPatterns, toolPattern(toolOutputs))
		if len(patterns) > 6 {
			patterns = patterns[1:]
		}
		if isStuckLoop(patterns, toolOutputs, loopCount) {
			content := "<system-reminder>\n系统检测到可能陷入重复调用。请基于已获取的信息直接开始写作，或明确告诉我你需要什么新的操作。\n</system-reminder>"
			a.appendMsg("user", content, "", nil, &opts, runningTokens)
			emit(AgentEvent{
				TurnID: opts.TurnID, Type: EventToolCall, Phase: "loop_detected", Timestamp: time.Now(),
			})
		}
		recentPatterns = patterns

		// 清空当前轮缓冲
		thinkingBuffer = ""
		responseBuffer = ""
		fullResponse = ""
		loopCount++
	}

	if interrupted {
		return AgentLoopResult{FinalText: fullResponse, ThinkingContent: thinkingBuffer, TurnCount: loopCount}, ctx.Err()
	}
	return AgentLoopResult{FinalText: fullResponse, ThinkingContent: thinkingBuffer, TurnCount: loopCount}, nil
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

	apiFormat := msg.ToAPIFormat()
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
	json.Unmarshal(raw, &m)
	return m
}
