package session

import (
	"encoding/json"
	"log/slog"
	"time"
)

// Session 是对话会话的元数据容器。
// 不持有 messages——消息通过 Store 按 to_api/to_frontend/version 三条路径独立查询。
// active_version 配合 Message.version 实现上下文压缩和版本级回滚，压缩不删旧消息。
type Session struct {
	SessionID       string    `gorm:"column:session_id;primaryKey"                                 json:"session_id"`
	NovelID         int64     `gorm:"column:novel_id;index:idx_sessions_novel;not null"            json:"novel_id"`
	Title           string    `gorm:"column:title;not null;default:''"                            json:"title"`
	Model           string    `gorm:"column:model;not null;default:deepseek-v4-pro"               json:"model"`
	ReasoningEffort string    `gorm:"column:reasoning_effort"                                      json:"reasoning_effort,omitempty"` // "high" | "max" | ""，DeepSeek 推理深度
	Summary         string    `gorm:"column:summary;not null;default:''"                          json:"summary"`                     // 最新压缩摘要，每次压缩全量替换
	PendingChanges  string    `gorm:"column:pending_changes"                                       json:"pending_changes,omitempty"`  // JSON，待确认的编辑变更列表
	ExtraMetadata   string    `gorm:"column:extra_metadata"                                        json:"extra_metadata,omitempty"`   // JSON，扩展槽
	ActiveVersion   int       `gorm:"column:active_version;not null;default:1"                    json:"active_version"`              // 当前活跃的上下文代数
	LastTurnID      int       `gorm:"column:last_turn_id;not null;default:0"                      json:"last_turn_id"`                // 最后一个 turn 的编号，原子自增
	Usage           string    `gorm:"column:usage"                                                 json:"usage,omitempty"`            // JSON，最近一次 LLM 调用的 token 用量
	CreatedAt       time.Time `gorm:"column:created_at;autoCreateTime"                            json:"created_at"`
	UpdatedAt       time.Time `gorm:"column:updated_at;autoUpdateTime;index:idx_sessions_novel"   json:"updated_at"`
}

func (Session) TableName() string { return "sessions" }

// Message 是对话中的单条消息，append-only，永不修改永不删除。
//
// to_api 和 to_frontend 由写入方独立决定，不由 role 推导。四种 role 都可能有任意的可见性组合。
// version 标记该消息属于第几代上下文构建，配合 Session.active_version 做查询过滤。
// event_type 标记特殊事件（compression/interrupt/error），普通消息为空。
type Message struct {
	ID              int64     `gorm:"column:id;primaryKey;autoIncrement"                            json:"id"`
	SessionID       string    `gorm:"column:session_id;index;not null"                             json:"session_id"`
	TurnID          int       `gorm:"column:turn_id;not null;default:0;index"                      json:"turn_id"` // 所属 turn，回退时直接 DELETE WHERE turn_id
	Role            string    `gorm:"column:role;not null"                                          json:"role"`   // "system" | "user" | "assistant" | "tool"
	Content         string    `gorm:"column:content;not null"                                       json:"content"`
	ThinkingContent string    `gorm:"column:thinking_content"                                     json:"thinking_content,omitempty"`
	TokenCount      int       `gorm:"column:token_count;not null;default:0"                        json:"token_count"`
	ExtraMetadata   string    `gorm:"column:extra_metadata"                                         json:"extra_metadata,omitempty"` // JSON：tool_calls / tool_call_id / display_text 等
	Version         int       `gorm:"column:version;not null;default:1;index"                      json:"version"`                   // 压缩代数，查询时 = session.active_version
	ToAPI           bool      `gorm:"column:to_api;not null;default:0;index"                       json:"to_api"`                    // LLM context 是否需要此消息。注：default:0 原因同上，子 agent 的 ToAPI=false 不能被默认值覆盖
	ToFrontend      bool      `gorm:"column:to_frontend;not null;default:0;index"                  json:"to_frontend"`               // 前端是否需要渲染此消息。注：default:0 必须与 Go false 一致，否则 GORM 跳过零值时 DB 填入默认值 1 导致泄漏
	// EventType: "" | "compression" | "user_stopped" | "system_interrupted" | "error"
	// - "": 正常消息
	// - "compression": 上下文压缩边界
	// - "user_stopped": 用户手动停止（context.Canceled）
	// - "system_interrupted": 后端主动中断（MaxTurns/死循环检测）
	// - "error": 服务商错误（HTTP 4xx/5xx/网络/超时）
	EventType       string    `gorm:"column:event_type"                                             json:"event_type,omitempty"`
	AgentType       string    `gorm:"column:agent_type;not null;default:'main';index"              json:"agent_type"`                // "main" | "review" | "memory"
	SubTaskID       string    `gorm:"column:sub_task_id;index"                                     json:"sub_task_id,omitempty"`     // run_subagent 的 tool call ID，前端路由子 Agent 消息用
	CreatedAt       time.Time `gorm:"column:created_at;autoCreateTime;index"                       json:"created_at"`
}

func (Message) TableName() string { return "messages" }

// ToAPIFormat 转为 OpenAI Chat Completions 兼容的消息格式。
// 从 ExtraMetadata 提取 tool_calls、thinking_content、tool_call_id 拼入对应字段。
func (m *Message) ToAPIFormat(logger *slog.Logger) map[string]any {
	payload := map[string]any{
		"role":    m.Role,
		"content": m.Content,
	}

	if m.Role == "assistant" {
		if m.ThinkingContent != "" {
			payload["reasoning_content"] = m.ThinkingContent
		}
		var meta map[string]any
		if m.ExtraMetadata != "" {
			if err := json.Unmarshal([]byte(m.ExtraMetadata), &meta); err != nil {
				logger.Error("corrupt ExtraMetadata", "message_id", m.ID, "raw", m.ExtraMetadata, "err", err)
			}
		}
		if meta != nil {
			if tc, ok := meta["tool_calls"]; ok {
				payload["tool_calls"] = tc
				if m.ThinkingContent == "" {
					payload["reasoning_content"] = ""
				}
			}
		}
	}

	if m.Role == "tool" {
		var meta map[string]any
		if m.ExtraMetadata != "" {
			if err := json.Unmarshal([]byte(m.ExtraMetadata), &meta); err != nil {
				logger.Error("corrupt ExtraMetadata", "message_id", m.ID, "raw", m.ExtraMetadata, "err", err)
			}
		}
		if meta != nil {
			if id, ok := meta["tool_call_id"]; ok {
				payload["tool_call_id"] = id
			}
			if name, ok := meta["tool_name"]; ok {
				payload["name"] = name
			}
		}
	}

	return payload
}
