package session

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/sigpanic/goink/internal/llm/adapter"
	"github.com/sigpanic/goink/internal/storage"

	"gorm.io/gorm"
)

// Store 管理 Session/Message 持久化。DB 导出供调用方做简单 CRUD（Create/First/Append）。
type Store struct {
	DB     *gorm.DB
	logger *slog.Logger
}

// NewStore 创建 session 存储。
func NewStore(db *gorm.DB, logger *slog.Logger) *Store {
	return &Store{DB: db, logger: logger}
}

// ========== Session 查询 ==========

// GetSession 按 session_id 加载单个 session。
func (s *Store) GetSession(ctx context.Context, sessionID string) (*Session, error) {
	var sess Session
	if err := s.DB.WithContext(ctx).Where("session_id = ?", sessionID).First(&sess).Error; err != nil {
		return nil, fmt.Errorf("session store: get session: %w", err)
	}
	return &sess, nil
}

// ListSessionsOptions 是 ListSessions 的可选参数。
type ListSessionsOptions struct {
	PageParams storage.PageParams
	Search     string // 空=全部，非空=按消息内容 LIKE 模糊匹配
}

// ListSessions 按小说列出会话，updated_at 倒序，分页。Search 非空时搜索消息内容。
func (s *Store) ListSessions(ctx context.Context, novelID int64, opts ListSessionsOptions) (*storage.PageResult[Session], error) {
	pp := opts.PageParams
	pp.Normalize()

	if opts.Search == "" {
		return s.listAll(ctx, novelID, pp)
	}
	return s.search(ctx, novelID, opts.Search)
}

func (s *Store) listAll(ctx context.Context, novelID int64, pp storage.PageParams) (*storage.PageResult[Session], error) {
	q := s.DB.WithContext(ctx).Model(&Session{}).Where("novel_id = ?", novelID)

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("session store: count sessions: %w", err)
	}

	var sessions []Session
	if err := q.Order("updated_at DESC").Offset(pp.Offset()).Limit(pp.Size).Find(&sessions).Error; err != nil {
		return nil, fmt.Errorf("session store: list sessions: %w", err)
	}

	s.logger.Debug("session store: listed sessions", "novel_id", novelID, "total", total, "page", pp.Page)
	return storage.NewPageResult(sessions, total, pp.Page, pp.Size), nil
}

func (s *Store) search(ctx context.Context, novelID int64, search string) (*storage.PageResult[Session], error) {
	var sessions []Session
	if err := s.DB.WithContext(ctx).
		Distinct("sessions.*").
		Joins("JOIN messages ON messages.session_id = sessions.session_id").
		Where("sessions.novel_id = ? AND messages.content LIKE ?", novelID, "%"+search+"%").
		Order("sessions.updated_at DESC").
		Limit(100).
		Find(&sessions).Error; err != nil {
		return nil, fmt.Errorf("session store: search sessions: %w", err)
	}

	total := int64(len(sessions))
	s.logger.Debug("session store: searched sessions", "novel_id", novelID, "search", search, "total", total)
	return storage.NewPageResult(sessions, total, 1, 100), nil
}

// ========== Session 更新 ==========

// UpdateSessionTitle 只更新标题；空字符串保持现有标题。
func (s *Store) UpdateSessionTitle(ctx context.Context, sessionID, title string) error {
	if title == "" {
		return nil
	}

	result := s.DB.WithContext(ctx).Model(&Session{}).
		Where("session_id = ?", sessionID).
		Update("title", title)
	if result.Error != nil {
		return fmt.Errorf("session store: update title: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("session store: update title: %w", gorm.ErrRecordNotFound)
	}
	return nil
}

// UpdateSessionModelSelection 整体替换模型选择和推理等级，允许清空推理等级。
// 仅更新目标会话的对应列；模型可用性由上层校验，不创建缺失会话。
func (s *Store) UpdateSessionModelSelection(ctx context.Context, sessionID string, selection adapter.ModelSelection, reasoningEffort string) error {
	result := s.DB.WithContext(ctx).Model(&Session{}).
		Where("session_id = ?", sessionID).
		Updates(map[string]any{
			"adapter_name":     selection.AdapterName,
			"provider_name":    selection.ProviderName,
			"model_id":         selection.ModelID,
			"reasoning_effort": reasoningEffort,
		})
	if result.Error != nil {
		return fmt.Errorf("session store: update model selection: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("session store: update model selection: %w", gorm.ErrRecordNotFound)
	}
	return nil
}

// UpdateSessionUsage 更新最近一次 LLM 的 token 用量。
func (s *Store) UpdateSessionUsage(ctx context.Context, sessionID, usageJSON string) error {
	result := s.DB.WithContext(ctx).Model(&Session{}).
		Where("session_id = ?", sessionID).
		Update("usage", usageJSON)
	if result.Error != nil {
		return fmt.Errorf("session store: update usage: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("session store: update usage: %w", gorm.ErrRecordNotFound)
	}
	return nil
}

// BumpActiveVersion 递增 active_version 并返回新值。
func (s *Store) BumpActiveVersion(ctx context.Context, sessionID string) (int, error) {
	var sess Session
	if err := s.DB.WithContext(ctx).
		Where("session_id = ?", sessionID).
		First(&sess).Error; err != nil {
		return 0, fmt.Errorf("session store: bump version: %w", err)
	}

	sess.ActiveVersion++

	if err := s.DB.WithContext(ctx).Save(&sess).Error; err != nil {
		return 0, fmt.Errorf("session store: bump version: %w", err)
	}

	newV := sess.ActiveVersion
	s.logger.Debug("session store: bumped version", "session_id", sessionID, "new_version", newV)
	return newV, nil
}

// ========== Session 删除 ==========

// Delete 删除一个 session 及其全部关联数据：messages、operation_log。
// 单事务内执行，保证原子性。
//
// sessions 与 messages 表本身在 skipOperLog 名单内，不走 operation_log；
// operation_log 的清理用裸 DELETE（Table 模式无 Schema，不经 callback），
// 因此整个删除过程不产生新的日志记录，符合"session 删除不可回滚"的语义。
// 清理 operation_log 的原因：该 session 产生的领域元数据操作日志强绑定 session_id，
// session 删除后成为孤儿（回滚第一步即查 sessions.last_turn_id），无保留意义。
func (s *Store) Delete(ctx context.Context, sessionID string) error {
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("session_id = ?", sessionID).Delete(&Message{}).Error; err != nil {
			return fmt.Errorf("delete messages: %w", err)
		}
		if err := tx.Where("session_id = ?", sessionID).Delete(&Session{}).Error; err != nil {
			return fmt.Errorf("delete session: %w", err)
		}
		if err := tx.Table("operation_log").Where("session_id = ?", sessionID).Delete(nil).Error; err != nil {
			return fmt.Errorf("delete operation_log: %w", err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("session store: delete: %w", err)
	}
	s.logger.Debug("session store: deleted session", "session_id", sessionID)
	return nil
}

// ========== Message 查询 ==========

// NextTurn 原子递增 last_turn_id 并返回新值。
// agent loop 在每个 turn 开始时调用，一步完成递增 + 持久化。
func (s *Store) NextTurn(ctx context.Context, sessionID string) (int, error) {
	var turnID int
	if err := s.DB.WithContext(ctx).
		Raw("UPDATE sessions SET last_turn_id = last_turn_id + 1, updated_at = ? WHERE session_id = ? RETURNING last_turn_id", time.Now(), sessionID).
		Scan(&turnID).Error; err != nil {
		return 0, fmt.Errorf("session store: next turn: %w", err)
	}

	s.logger.Debug("session store: next turn", "session_id", sessionID, "turn_id", turnID)
	return turnID, nil
}

// GetMessagesForAPI 返回 LLM context 所需的消息。
func (s *Store) GetMessagesForAPI(ctx context.Context, sessionID string, version int) ([]Message, error) {
	var msgs []Message
	if err := s.DB.WithContext(ctx).
		Where("session_id = ? AND to_api = ? AND version = ?", sessionID, true, version).
		Order("created_at ASC").
		Find(&msgs).Error; err != nil {
		return nil, fmt.Errorf("session store: get api messages: %w", err)
	}
	return msgs, nil
}

// GetMessagesForFrontend 返回前端展示所需的消息。
func (s *Store) GetMessagesForFrontend(ctx context.Context, sessionID string) ([]Message, error) {
	var msgs []Message
	if err := s.DB.WithContext(ctx).
		Where("session_id = ? AND to_frontend = ?", sessionID, true).
		Order("created_at ASC").
		Find(&msgs).Error; err != nil {
		return nil, fmt.Errorf("session store: get frontend messages: %w", err)
	}
	return msgs, nil
}

// GetAllMessages 返回全部消息，审计/回退用。
func (s *Store) GetAllMessages(ctx context.Context, sessionID string) ([]Message, error) {
	var msgs []Message
	if err := s.DB.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Order("created_at ASC").
		Find(&msgs).Error; err != nil {
		return nil, fmt.Errorf("session store: get all messages: %w", err)
	}
	return msgs, nil
}
