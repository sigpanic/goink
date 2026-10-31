package agent

import (
	"context"
	"fmt"
	"maps"
	"strconv"
	"strings"
	"time"

	wails "github.com/wailsapp/wails/v2/pkg/runtime"
	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/agentcfg"
	"github.com/sigpanic/goink/internal/llm"
	"github.com/sigpanic/goink/internal/session"
)

const compressionPrompt = `<system-reminder>
你是上下文压缩助手。请基于完整对话历史生成结构化摘要，用于后续对话的上下文恢复。

## 已完成的事项
（每个一句话，最多 15 条，从最近的开始保留。不再重复执行的事项）

## 进行中（断点）
（最详细的部分：当前正在做什么、做到哪一步、下一步计划做什么。这是最重要的部分，请务必详尽）

## 用户偏好和要求
（从用户消息中提炼的核心写作风格、约束条件、反复强调的事项）

## 关键决策和设定变更
（已确认的情节走向、角色设定、世界观规则、命名等决定）

## 待办事项
（已计划但尚未开始的任务清单）
</system-reminder>`

const compressionReminder = "<system-reminder>\n上下文已压缩，请根据下面的摘要继续工作。\n</system-reminder>"

const maxUserRetain = 15
const minConversationTurns = 4

// generateSummary 调用 LLM 生成压缩摘要，返回摘要文本和应保留的历史消息。
func (a *Agent) generateSummary(ctx context.Context, opts *RunOptions) (string, []llm.Message, error) {
	msgs := make([]llm.Message, len(opts.Messages)+1)
	copy(msgs, opts.Messages)
	msgs[len(opts.Messages)] = llm.Message{Role: "user", Content: compressionPrompt}

	callOpts := &llm.CallOptions{SessionID: opts.SessionID, NovelID: opts.NovelID}
	if opts.ReasoningEffort != "" {
		callOpts.ReasoningEffort = &opts.ReasoningEffort
	}
	summary, err := a.llm.GenerateText(ctx, opts.AdapterName, opts.ProviderName, opts.Model.ID, msgs, callOpts)
	if err != nil {
		a.logger.Warn("压缩摘要生成失败，保持原上下文", "err", err)
		return "", nil, fmt.Errorf("compress: LLM summary failed: %w", err)
	}

	summary = strings.TrimSpace(summary)
	if summary == "" {
		a.logger.Warn("压缩摘要为空，保持原上下文")
		return "", nil, fmt.Errorf("compress: empty summary")
	}

	a.logger.Debug("压缩摘要生成成功", "summary_len", len(summary))
	retained := retainMessages(msgs[:len(opts.Messages)])
	return summary, retained, nil
}

// Compress 执行主 Agent 上下文压缩：调 LLM 生成摘要，重建 SkillCatalog/NovelState，保留近期关键消息。
// opts 不会被修改，成功后才赋值新的 Messages / ActiveVersion / runningTokens。
func (a *Agent) Compress(ctx context.Context, opts *RunOptions, runningTokens map[string]int) error {
	a.logger.Info("开始上下文压缩",
		"session_id", opts.SessionID,
		"turn_id", opts.TurnID,
		"estimated_tokens", sumRunningTokens(runningTokens),
		"msg_count", len(opts.Messages),
	)

	a.emitCompression(ctx, opts.TurnID, "compressing", "", "")

	summary, retained, err := a.generateSummary(ctx, opts)
	if err != nil {
		return err
	}

	// 重建系统消息（与 chat 走同一条路径 BuildSystemMessages，事务外构建）
	sysMsgs, buildErr := agentcfg.BuildSystemMessages(ctx, a.db, opts.NovelID, a.skillStore)
	if buildErr != nil {
		a.logger.Warn("压缩时 system 消息构建部分失败", "novel_id", opts.NovelID, "err", buildErr)
	}

	// 在事务中完成版本递增 + 全部 DB 写入
	newVersion, err := a.persistCompression(ctx, opts, sysMsgs, summary, retained)
	if err != nil {
		return fmt.Errorf("compress: persist failed: %w", err)
	}

	// 从 DB 加载新版本消息，与 Chat() 走同一条路径
	apiMsgs, err := a.session.GetMessagesForAPI(ctx, opts.SessionID, newVersion)
	if err != nil {
		return fmt.Errorf("compress: load messages after persist: %w", err)
	}
	messages := make([]llm.Message, len(apiMsgs))
	for i, m := range apiMsgs {
		messages[i], err = m.ToLLMRequestMessage()
		if err != nil {
			return fmt.Errorf("compress: render message: %w", err)
		}
	}
	messages, err = llm.PrepareImageMessages(opts.NovelID, *opts.Model, messages)
	if err != nil {
		return fmt.Errorf("compress: prepare images: %w", err)
	}
	opts.ActiveVersion = newVersion
	opts.Messages = messages

	newTokens := a.InitRunningTokens(opts.Messages)
	clear(runningTokens)
	maps.Copy(runningTokens, newTokens)

	a.emitCompression(ctx, opts.TurnID, "done", summary, "")

	a.logger.Info("上下文压缩完成",
		"session_id", opts.SessionID,
		"new_version", newVersion,
		"retained", len(retained),
		"new_msg_count", len(opts.Messages),
	)
	return nil
}

// compressInMemory 执行子 Agent 上下文压缩：纯内存操作，AgentIdentity/NovelState 不动，仅写边界标记到 DB。
func (a *Agent) compressInMemory(ctx context.Context, opts *RunOptions, runningTokens map[string]int) error {
	a.logger.Info("子Agent上下文压缩",
		"session_id", opts.SessionID,
		"turn_id", opts.TurnID,
		"agent_type", opts.AgentType,
		"sub_task_id", opts.SubTaskID,
		"estimated_tokens", sumRunningTokens(runningTokens),
		"msg_count", len(opts.Messages),
	)

	a.emitCompression(ctx, opts.TurnID, "compressing", "", opts.SubTaskID)

	summary, retained, err := a.generateSummary(ctx, opts)
	if err != nil {
		return err
	}

	// 提取头部 system 消息，不动
	sysEnd := 0
	for i, m := range opts.Messages {
		if m.Role == "system" {
			sysEnd = i + 1
		} else {
			break
		}
	}
	sysMsgs := make([]llm.Message, sysEnd)
	copy(sysMsgs, opts.Messages[:sysEnd])

	// 内存重建 opts.Messages
	newMsgs := make([]llm.Message, 0, sysEnd+2+len(retained))
	newMsgs = append(newMsgs, sysMsgs...)
	newMsgs = append(newMsgs,
		llm.Message{Role: "user", Content: compressionReminder},
		llm.Message{Role: "user", Content: "<system-reminder>\n" + summary + "\n</system-reminder>"},
	)
	newMsgs = append(newMsgs, retained...)
	opts.Messages = newMsgs

	// 边界标记
	if err := a.db.Create(&session.Message{
		SessionID:  opts.SessionID,
		TurnID:     opts.TurnID,
		Role:       "system",
		Content:    "",
		Version:    opts.ActiveVersion,
		ToAPI:      false,
		ToFrontend: true,
		EventType:  session.EventCompression,
		AgentType:  opts.AgentType,
		SubTaskID:  opts.SubTaskID,
	}).Error; err != nil {
		a.logger.Warn("子Agent压缩边界标记写入失败", "err", err)
	}

	newTokens := a.InitRunningTokens(opts.Messages)
	clear(runningTokens)
	maps.Copy(runningTokens, newTokens)

	opts.SubAgentVersion++

	a.emitCompression(ctx, opts.TurnID, "done", summary, opts.SubTaskID)

	a.logger.Info("子Agent上下文压缩完成",
		"session_id", opts.SessionID,
		"sub_agent_version", opts.SubAgentVersion,
		"retained", len(retained),
		"new_msg_count", len(opts.Messages),
	)
	return nil
}

// persistCompression 在事务中递增 active_version 并写入所有压缩消息。
func (a *Agent) persistCompression(ctx context.Context, opts *RunOptions, msgs *agentcfg.SystemMessages, summary string, retained []llm.Message) (int, error) {
	var newVersion int

	err := a.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var sess session.Session
		if err := tx.First(&sess, "session_id = ?", opts.SessionID).Error; err != nil {
			return fmt.Errorf("查询 session 失败: %w", err)
		}
		sess.ActiveVersion++
		if err := tx.Save(&sess).Error; err != nil {
			return fmt.Errorf("递增 active_version 失败: %w", err)
		}
		newVersion = sess.ActiveVersion

		msg := func(role, content string, toAPI, toFE bool, eventType session.EventType) error {
			return tx.Create(&session.Message{
				SessionID:  opts.SessionID,
				TurnID:     opts.TurnID,
				Role:       role,
				Content:    content,
				Version:    newVersion,
				ToAPI:      toAPI,
				ToFrontend: toFE,
				EventType:  eventType,
				AgentType:  "main",
			}).Error
		}

		// system 消息按 Identity/Always/Catalog/Profile/State 顺序写入
		sysContents := []struct {
			name    string
			content string
		}{
			{"AgentIdentity", msgs.Identity},
			{"AlwaysSkills", msgs.Always},
			{"SkillCatalog", msgs.Catalog},
			{"NovelProfile", msgs.Profile},
			{"NovelState", msgs.State},
		}
		for _, sc := range sysContents {
			if sc.content == "" {
				continue
			}
			if err := msg("system", sc.content, true, false, ""); err != nil {
				return fmt.Errorf("write %s: %w", sc.name, err)
			}
		}
		// 提醒语
		if err := msg("user", compressionReminder, true, false, ""); err != nil {
			return fmt.Errorf("write compression reminder: %w", err)
		}
		// 摘要
		if err := msg("user", "<system-reminder>\n"+summary+"\n</system-reminder>", true, false, ""); err != nil {
			return fmt.Errorf("write summary: %w", err)
		}
		// 保留消息副本
		for _, m := range retained {
			rm, err := llmMsgToMessage(m, opts.SessionID, opts.TurnID, newVersion)
			if err != nil {
				return fmt.Errorf("转换保留消息失败: %w", err)
			}
			if err := tx.Create(&rm).Error; err != nil {
				return fmt.Errorf("写入保留消息失败: %w", err)
			}
		}
		// 边界标记
		if err := msg("system", "", false, true, session.EventCompression); err != nil {
			return fmt.Errorf("write compression marker: %w", err)
		}

		return nil
	})

	return newVersion, err
}

// llmMsgToMessage 将已渲染的保留消息映射为新记录，避免重载时重复添加时间前缀。
func llmMsgToMessage(m llm.Message, sessionID string, turnID, version int) (session.Message, error) {
	msg := session.Message{
		SessionID:  sessionID,
		TurnID:     turnID,
		Version:    version,
		ToAPI:      true,
		ToFrontend: false,
		AgentType:  "main",
	}

	return msg.WithLLMMessage(m)
}

// emitCompression 推送压缩事件到前端。
func (a *Agent) emitCompression(ctx context.Context, turnID int, phase, summary, subTaskID string) {
	wails.EventsEmit(ctx, "agent:"+strconv.Itoa(turnID), AgentEvent{
		TurnID:           turnID,
		Type:             EventCompression,
		CompressionPhase: phase,
		Summary:          summary,
		SubTaskID:        subTaskID,
		Timestamp:        time.Now(),
	})
}

// retainMessages 筛选应保留的历史消息。
// 跳过前 N 条 system 消息，后续应用保留规则。
func retainMessages(messages []llm.Message) []llm.Message {
	if len(messages) == 0 {
		return nil
	}

	// 跳过前部 system 消息
	sysEnd := 0
	for _, m := range messages {
		if m.Role == "system" {
			sysEnd++
		} else {
			break
		}
	}

	history := messages[sysEnd:]
	if len(history) == 0 {
		return nil
	}

	// 找到所有 user 消息的位置
	userIdx := make([]int, 0)
	for i, m := range history {
		if m.Role == "user" {
			userIdx = append(userIdx, i)
		}
	}

	if len(userIdx) == 0 {
		return nil
	}

	// 保留最近 maxUserRetain 条 user 消息
	keepFrom := 0
	if len(userIdx) > maxUserRetain {
		keepFrom = userIdx[len(userIdx)-maxUserRetain]
	}

	// 确保至少保留 minConversationTurns 轮对话
	if len(userIdx) >= minConversationTurns {
		minKeep := userIdx[len(userIdx)-minConversationTurns]
		if minKeep < keepFrom {
			keepFrom = minKeep
		}
	}

	retained := make([]llm.Message, 0, len(history)-keepFrom)
	for _, m := range history[keepFrom:] {
		retained = append(retained, m.Clone())
	}

	return retained
}

// IsRunning 检查指定 session 是否有正在进行的 turn。
func (a *Agent) IsRunning(sessionID string) bool {
	return a.cancelMgr.IsRegistered(CancelPrefixChat + sessionID)
}
