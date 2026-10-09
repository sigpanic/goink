package storage

import (
	"context"
	"encoding/json"
	"fmt"

	"gorm.io/gorm"
)

// WithEntityTurn 让附属记录的手动变更继承所属实体的轮次；显式 AI 轮次优先。
// 调用方必须传入当前事务，避免在单连接数据库中另取连接。
func WithEntityTurn(ctx context.Context, db *gorm.DB, table string, id int64) (context.Context, error) {
	if _, ok := getTurnInfo(ctx); ok {
		return ctx, nil
	}
	key, err := json.Marshal(map[string]int64{"id": id})
	if err != nil {
		return nil, fmt.Errorf("oplog: encode entity: %w", err)
	}
	var last OperationLogRecord
	if err := db.WithContext(ctx).Where("table_name = ? AND entity_id = ?", table, string(key)).
		Order("id DESC").Limit(1).Find(&last).Error; err != nil {
		return nil, fmt.Errorf("oplog: inherit entity turn: %w", err)
	}
	return WithTurn(ctx, last.SessionID, last.TurnID), nil
}
