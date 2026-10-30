package session

import (
	"context"
	"encoding/json"
	"fmt"

	"gorm.io/gorm"
)

// MessagePageOptions 按完整 turn 向前翻页；零游标表示最新历史，Size 为轮数。
type MessagePageOptions struct {
	BeforeTurnID int `json:"before_turn_id"`
	Size         int `json:"size"`
}

// MessagePage 中的消息按轮次及轮内时间正序展示，下一页使用 NextBeforeTurnID 获取更早的轮次。
type MessagePage struct {
	Items            []Message `json:"items"`
	NextBeforeTurnID int       `json:"next_before_turn_id"`
	HasMore          bool      `json:"has_more"`
}

// GetMessagesPageForFrontend 先选可见轮次，再读取整轮消息，不限制轮内消息数。
// 两次查询共享读事务，避免回退或删除会话时得到不一致的轮次和消息。
func (s *Store) GetMessagesPageForFrontend(ctx context.Context, sessionID string, opts MessagePageOptions) (*MessagePage, error) {
	if opts.BeforeTurnID < 0 || opts.Size < 0 {
		return nil, fmt.Errorf("session store: message cursor and size must not be negative")
	}
	if opts.Size == 0 {
		opts.Size = 20
	}
	if opts.Size > 100 {
		opts.Size = 100
	}
	page := &MessagePage{Items: []Message{}}
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.Model(&Message{}).Where("session_id = ? AND to_frontend = ?", sessionID, true)
		if opts.BeforeTurnID > 0 {
			query = query.Where("turn_id < ?", opts.BeforeTurnID)
		}
		var turns []int
		if err := query.Distinct("turn_id").Order("turn_id DESC").Limit(opts.Size+1).Pluck("turn_id", &turns).Error; err != nil {
			return fmt.Errorf("select visible turns: %w", err)
		}
		if len(turns) == 0 {
			return nil
		}
		page.HasMore = len(turns) > opts.Size
		if page.HasMore {
			turns = turns[:opts.Size]
			page.NextBeforeTurnID = turns[len(turns)-1]
		}
		if err := tx.Where("session_id = ? AND to_frontend = ? AND turn_id IN ?", sessionID, true, turns).
			Order("turn_id ASC, created_at ASC, id ASC").Find(&page.Items).Error; err != nil {
			return fmt.Errorf("read visible turn messages: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("session store: get frontend message page: %w", err)
	}
	if err := projectFrontendMessages(page.Items); err != nil {
		return nil, fmt.Errorf("session store: project frontend message page: %w", err)
	}
	return page, nil
}

// projectFrontendMessages 只处理查询副本，保留显示和未知扩展字段的 JSON 数值精度。
func projectFrontendMessages(messages []Message) error {
	for i := range messages {
		if messages[i].ExtraMetadata == "" {
			continue
		}
		var metadata map[string]json.RawMessage
		if err := json.Unmarshal([]byte(messages[i].ExtraMetadata), &metadata); err != nil {
			return fmt.Errorf("message %d: decode frontend metadata: %w", messages[i].ID, err)
		}
		if _, exists := metadata["llm_state"]; !exists {
			continue
		}
		delete(metadata, "llm_state")
		messages[i].ExtraMetadata = ""
		if len(metadata) > 0 {
			data, err := json.Marshal(metadata)
			if err != nil {
				return fmt.Errorf("message %d: encode frontend metadata: %w", messages[i].ID, err)
			}
			messages[i].ExtraMetadata = string(data)
		}
	}
	return nil
}
