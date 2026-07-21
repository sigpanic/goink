package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"gorm.io/gorm"
)

// ── 上下文传递 ──────────────────────────────────────────────

// TurnInfo 记录当前 turn 的 session 和 turn 编号，用于 operation_log 定位回退范围。
type TurnInfo struct {
	SessionID string
	TurnID    int
}

type turnKeyType struct{}

// WithTurn 将 turn 信息注入 context。agent loop 在 turn 开始时调用。
func WithTurn(ctx context.Context, sessionID string, turnID int) context.Context {
	return context.WithValue(ctx, turnKeyType{}, TurnInfo{SessionID: sessionID, TurnID: turnID})
}

// getTurnInfo 从 context 提取 turn 信息，不存在时返回空值。
func getTurnInfo(ctx context.Context) (TurnInfo, bool) {
	info, ok := ctx.Value(turnKeyType{}).(TurnInfo)
	return info, ok
}

// withoutTurn 返回一个藏掉 TurnInfo 的 context，其他值原样透传。
// 回退操作使用此 context，避免 reverseOne 中的 GORM 写操作被 callback 再次记录。
type stripTurnContext struct{ context.Context }

func (c *stripTurnContext) Value(key any) any {
	if _, ok := key.(turnKeyType); ok {
		return nil
	}
	return c.Context.Value(key)
}

func withoutTurn(ctx context.Context) context.Context {
	return &stripTurnContext{Context: ctx}
}

// ── 操作日志记录 ────────────────────────────────────────────

// OperationLogRecord 操作日志表的一行。
// 不通过 GORM AutoMigrate 创建，由 migrate.go 统一管理建表。
type OperationLogRecord struct {
	ID        int64     `gorm:"column:id;primaryKey;autoIncrement"`
	SessionID string    `gorm:"column:session_id;not null;index:idx_oplog_rollback,priority:1"`
	TurnID    int       `gorm:"column:turn_id;not null;index:idx_oplog_rollback,priority:2"`
	Operation string    `gorm:"column:operation;not null"`                                    // "create" | "update" | "delete"
	Table     string    `gorm:"column:table_name;not null;index:idx_oplog_entity,priority:1"` // 目标表名，如 "characters"
	EntityID  string    `gorm:"column:entity_id;not null;index:idx_oplog_entity,priority:2"`  // JSON 化的主键条件，如 {"id":5} 或 {"novel_id":1,"scope":"next"}
	OldValues string    `gorm:"column:old_values"`                                            // JSON，create 时为 ""
	NewValues string    `gorm:"column:new_values"`                                            // JSON，delete 时为 ""
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"`
}

func (OperationLogRecord) TableName() string { return "operation_log" }

// ── GORM Callback 注册 ──────────────────────────────────────

// RegisterOplogHooks 在 *gorm.DB 上注册写操作拦截回调。
// 调用时机：storage.Open 初始化 DB 后调用一次。
//
// 【注意事项】
//  1. 批量更新（db.Where(...).Updates(...)）不走单行 callback（无 Schema），不会自动记录。
//     需要此类操作的 store 方法必须显式调用独立的日志方法。
//  2. 裸 SQL（db.Exec(...)）不走 callback，同样不会自动记录。
//     当前所有 store 均无上述两种情况；保留此注释防止后续踩坑。
func RegisterOplogHooks(db *gorm.DB) error {
	// BeforeCreate: UPSERT 检测——PK 值已设置的实体可能是 ON CONFLICT 更新已有行，
	// 先查旧值存入 InstanceSet。自增主键（值为零）的普通 INSERT 跳过此步。
	if err := db.Callback().Create().Before("gorm:before_create").Register("oplog:before_create", beforeCreate); err != nil {
		return fmt.Errorf("register oplog:before_create: %w", err)
	}

	// AfterCreate: INSERT（或 UPSERT）→ 根据 beforeCreate 查到的旧值决定 operation 类型
	if err := db.Callback().Create().After("gorm:after_create").Register("oplog:after_create", afterCreate); err != nil {
		return fmt.Errorf("register oplog:after_create: %w", err)
	}

	// BeforeUpdate: 取旧值暂存 InstanceSet
	if err := db.Callback().Update().Before("gorm:before_update").Register("oplog:before_update", beforeUpdate); err != nil {
		return fmt.Errorf("register oplog:before_update: %w", err)
	}

	// AfterUpdate: 用暂存旧值 + 新值 → 记录
	if err := db.Callback().Update().After("gorm:after_update").Register("oplog:after_update", afterUpdate); err != nil {
		return fmt.Errorf("register oplog:after_update: %w", err)
	}

	// BeforeDelete: 取旧值暂存（逻辑同 beforeUpdate）
	if err := db.Callback().Delete().Before("gorm:before_delete").Register("oplog:before_delete", beforeDelete); err != nil {
		return fmt.Errorf("register oplog:before_delete: %w", err)
	}

	// AfterDelete: 用暂存旧值 → 记录
	if err := db.Callback().Delete().After("gorm:after_delete").Register("oplog:after_delete", afterDelete); err != nil {
		return fmt.Errorf("register oplog:after_delete: %w", err)
	}

	return nil
}

// ── Create 回调 ─────────────────────────────────────────────

func beforeCreate(db *gorm.DB) {
	// 只对设置了完整 PK 值的实体查旧行（自然键实体可能是 UPSERT）。
	// 自增主键（值 = 零值）跳过，避免无效 SELECT。
	// map/slice 等无 Schema 的 Dest 也直接跳过。
	if db.Statement.Schema == nil {
		return
	}
	pkMap := getPKValues(db)
	if !hasNonZeroPK(pkMap) {
		return
	}

	oldRow := fetchOldRow(db)
	db.InstanceSet("oplog:before_create_old", oldRow)
}

func afterCreate(db *gorm.DB) {
	if skipOperLog(db) {
		return
	}
	info, ok := getTurnInfo(db.Statement.Context)
	if !ok {
		// 前端 CRUD 无 TurnInfo：查 operation_log 继承同实体上一条记录的 session/turn，
		// 使回滚时用户编辑与 AI 操作一起撤销。无历史记录则 fallback 为 turn_id=0（不被回滚）。
		info = resolveTurnForEntity(db)
	}

	newJSON := serializeDest(db)
	if newJSON == "" {
		return
	}

	oldRow, _ := db.InstanceGet("oplog:before_create_old")
	if oldRow != nil {
		// UPSERT 更新了已有行 → 按 update 记录
		oldJSON := toJSON(oldRow)
		if oldJSON == newJSON {
			return
		}
		record := buildRecord(db, info, "update", oldJSON, newJSON)
		if err := db.Session(&gorm.Session{NewDB: true, SkipHooks: true}).Create(&record).Error; err != nil {
			db.AddError(fmt.Errorf("operation log write (update upsert): %w", err))
		}
	} else {
		// 真正的 INSERT
		record := buildRecord(db, info, "create", "", newJSON)
		if err := db.Session(&gorm.Session{NewDB: true, SkipHooks: true}).Create(&record).Error; err != nil {
			db.AddError(fmt.Errorf("operation log write (create): %w", err))
		}
	}
}

// ── Update 回调 ─────────────────────────────────────────────

func beforeUpdate(db *gorm.DB) {
	oldRow := fetchOldRow(db)
	db.InstanceSet("oplog:update_old", oldRow)
}

func afterUpdate(db *gorm.DB) {
	if skipOperLog(db) {
		return
	}
	info, ok := getTurnInfo(db.Statement.Context)
	if !ok {
		info = resolveTurnForEntity(db)
	}

	oldRow, _ := db.InstanceGet("oplog:update_old")
	newJSON := serializeDest(db)
	if newJSON == "" {
		return
	}

	var oldJSON string
	if oldRow != nil {
		oldJSON = toJSON(oldRow)
	} else {
		// 旧行查不到（可能被并发删除），无法可靠记录，跳过
		return
	}

	if oldJSON == newJSON {
		return
	}

	record := buildRecord(db, info, "update", oldJSON, newJSON)
	if err := db.Session(&gorm.Session{NewDB: true, SkipHooks: true}).Create(&record).Error; err != nil {
		db.AddError(fmt.Errorf("operation log write (update): %w", err))
	}
}

// ── Delete 回调 ─────────────────────────────────────────────

func beforeDelete(db *gorm.DB) {
	oldRow := fetchOldRow(db)
	db.InstanceSet("oplog:delete_old", oldRow)
}

func afterDelete(db *gorm.DB) {
	if skipOperLog(db) {
		return
	}
	info, ok := getTurnInfo(db.Statement.Context)
	if !ok {
		info = resolveTurnForEntity(db)
	}

	oldRow, _ := db.InstanceGet("oplog:delete_old")
	if oldRow == nil {
		return
	}

	oldJSON := toJSON(oldRow)
	if oldJSON == "" {
		return
	}

	record := buildRecord(db, info, "delete", oldJSON, "")
	if err := db.Session(&gorm.Session{NewDB: true, SkipHooks: true}).Create(&record).Error; err != nil {
		db.AddError(fmt.Errorf("operation log write (delete): %w", err))
	}
}

// ── 回调辅助 ────────────────────────────────────────────────

// resolveTurnForEntity 为无 TurnInfo 的前端 CRUD 操作确定 session/turn 归属。
// 策略：查询 operation_log 中同一实体的最近一条记录，继承其 session_id/turn_id。
// 这样用户在前端编辑 AI 创建的实体时，该编辑归入 AI 所在的 turn，回滚时一起撤销。
// 如果该实体从未被 AI 操作过，返回零值 TurnInfo（turn_id=0, session_id=""），
// 记录仍写入 operation_log 但回滚时自然排除（WHERE turn_id >= ? 不匹配 0）。
func resolveTurnForEntity(db *gorm.DB) TurnInfo {
	if db.Statement.Schema == nil {
		return TurnInfo{}
	}
	table := db.Statement.Schema.Table
	entityID := buildEntityID(db)
	if entityID == "{}" {
		return TurnInfo{}
	}

	var last OperationLogRecord
	err := db.Session(&gorm.Session{NewDB: true, SkipHooks: true}).
		Where("table_name = ? AND entity_id = ?", table, entityID).
		Order("id DESC").
		Limit(1).
		Find(&last).Error
	if err != nil || last.ID == 0 {
		return TurnInfo{}
	}
	return TurnInfo{SessionID: last.SessionID, TurnID: last.TurnID}
}

// buildRecord 从 Statement 提取公共字段，组装 OperationLogRecord。
func buildRecord(db *gorm.DB, info TurnInfo, op, oldJSON, newJSON string) OperationLogRecord {
	return OperationLogRecord{
		TurnID:    info.TurnID,
		SessionID: info.SessionID,
		Operation: op,
		Table:     db.Statement.Schema.Table,
		EntityID:  buildEntityID(db),
		OldValues: oldJSON,
		NewValues: newJSON,
	}
}

// fetchOldRow 在执行 UPDATE/DELETE/UPSERT 前拿到当前 DB 中的完整行。
// 使用 db.Session(&gorm.Session{NewDB: true, SkipHooks: true}) 隔离 Statement，
// 底层连接/事务与当前 callback 共享，无并发问题。
//
// 当前从 Dest 反射拿主键——依赖调用方用 Save/Delete(&entity) 模式，entity 需有完整 PK。
// 若未来需要支持 Model().Where().Updates() 等 Dest 不含 PK 的模式，可回退解析
// db.Statement.Clauses["WHERE"] 提取条件（递归遍历 clause.Where{Exprs} 中的 Eq/IN/AND 等表达式）。
func fetchOldRow(db *gorm.DB) any {
	if db.Statement.Schema == nil || db.Statement.Dest == nil {
		return nil
	}

	pkMap := getPKValues(db)
	if !hasNonZeroPK(pkMap) {
		return nil
	}

	modelType := db.Statement.Schema.ModelType
	oldRow := reflect.New(modelType).Interface()

	err := db.Session(&gorm.Session{NewDB: true, SkipHooks: true}).
		Table(db.Statement.Table).
		Where(pkMap).
		Take(oldRow).Error
	if err != nil {
		return nil
	}
	return oldRow
}

// getPKValues 从 Dest 反射出主键名到值的映射。
func getPKValues(db *gorm.DB) map[string]any {
	pkFields := db.Statement.Schema.PrimaryFields
	if len(pkFields) == 0 {
		return nil
	}

	destValue := reflect.ValueOf(db.Statement.Dest)
	if destValue.Kind() == reflect.Pointer {
		destValue = destValue.Elem()
	}
	if destValue.Kind() != reflect.Struct {
		return nil
	}

	result := make(map[string]any, len(pkFields))
	for _, f := range pkFields {
		fv := destValue.FieldByName(f.Name)
		if fv.IsValid() {
			result[f.DBName] = fv.Interface()
		}
	}
	return result
}

// hasNonZeroPK 检查是否所有主键值都非零。任一为零时表示自增 ID 尚未分配，无需查旧行。
func hasNonZeroPK(pkMap map[string]any) bool {
	if len(pkMap) == 0 {
		return false
	}
	for _, v := range pkMap {
		if reflect.ValueOf(v).IsZero() {
			return false
		}
	}
	return true
}

// buildEntityID 用当前行的主键值构建 JSON 化的 WHERE 条件。
// 示例：{"id":5} 或 {"novel_id":1,"scope":"next"}
func buildEntityID(db *gorm.DB) string {
	pkMap := getPKValues(db)
	if len(pkMap) == 0 {
		return "{}"
	}
	b, _ := json.Marshal(pkMap)
	return string(b)
}

// serializeDest 将 db.Statement.Dest 序列化为 JSON 字符串。
func serializeDest(db *gorm.DB) string {
	if db.Statement.Dest == nil {
		return ""
	}
	return toJSON(db.Statement.Dest)
}

// skipOperLog 跳过对操作日志表、消息表和全局配置表的操作。
// 操作日志永不被追踪（防递归）；消息通过 turn_id 列直接 DELETE 回退，不走日志；
// 应用配置与 turn 无关，无需回退；turn_commits 由 rollback 包自行清理。
func skipOperLog(db *gorm.DB) bool {
	if db.Statement.Schema == nil {
		return true // Schema 不存在则无法判断表名，安全做法是跳过
	}
	t := db.Statement.Schema.Table
	return t == "operation_log" || t == "messages" || t == "app_config" || t == "turn_commits" ||
		t == "sessions" || t == "novels" || t == "writing_log" || t == "style_samples"
}

// toJSON 将任意值序列化为 JSON 字符串。
func toJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

// ── 回退 ────────────────────────────────────────────────────

// RollbackTo 回退到 targetTurn 开始之前的状态。
// 即撤销 [ targetTurn, last_turn_id ] 区间内所有 turn 的变更。
// 传入 15：撤销 turn 15 及之后（≥15）所有变更，回到 turn 15 还没发出时的状态。
//
// 回退内容：
//   - 领域实体：按 operation_log 逐条逆向执行（create→delete, update→恢复旧值, delete→insert）
//   - 消息：直接 DELETE WHERE turn_id >= targetTurn
//   - 回退自身的日志记录一并清理
//
// 整个操作在单个事务内完成。调用方若已持有事务，应使用 RollbackInTx 传入 tx。
func RollbackTo(ctx context.Context, db *gorm.DB, sessionID string, targetTurn, lastTurn int) error {
	return db.WithContext(withoutTurn(ctx)).Transaction(func(tx *gorm.DB) error {
		return RollbackInTx(ctx, tx, sessionID, targetTurn, lastTurn)
	})
}

// RollbackInTx 是 RollbackTo 的事务内版本，供外部已有事务时复用（如 rollback.RollbackBeforeTurn）。
// 回退 [targetTurn, lastTurn] 区间内领域实体、messages、operation_log 自身的记录。
func RollbackInTx(ctx context.Context, tx *gorm.DB, sessionID string, targetTurn, lastTurn int) error {
	// 1. 取出待回退的日志（倒序：从最新到最早）
	var records []OperationLogRecord
	if err := tx.Where("session_id = ? AND turn_id >= ? AND turn_id <= ?",
		sessionID, targetTurn, lastTurn).
		Order("id DESC").
		Find(&records).Error; err != nil {
		return fmt.Errorf("oplog: query operation_log: %w", err)
	}

	// 2. 逐条逆向执行领域实体变更
	for _, r := range records {
		if err := reverseOne(tx, r); err != nil {
			return fmt.Errorf("oplog: reverse failed (id=%d, op=%s, table=%s): %w",
				r.ID, r.Operation, r.Table, err)
		}
	}

	// 3. 删除该区间的 messages（通过 turn_id 列直接定位）
	if err := tx.Table("messages").
		Where("session_id = ? AND turn_id >= ? AND turn_id <= ?", sessionID, targetTurn, lastTurn).
		Delete(nil).Error; err != nil {
		return fmt.Errorf("oplog: delete messages: %w", err)
	}

	// 4. 清理已回退的操作日志
	if err := tx.Where("session_id = ? AND turn_id >= ? AND turn_id <= ?",
		sessionID, targetTurn, lastTurn).
		Delete(&OperationLogRecord{}).Error; err != nil {
		return fmt.Errorf("oplog: cleanup operation_log: %w", err)
	}

	// 5. 将 active_version 回退到剩余消息的最大版本号
	if err := tx.Exec(`
		UPDATE sessions
		SET active_version = (SELECT COALESCE(MAX(version), 1) FROM messages WHERE session_id = ? AND to_api = 1)
		WHERE session_id = ?
	`, sessionID, sessionID).Error; err != nil {
		return fmt.Errorf("oplog: rollback active_version: %w", err)
	}

	return nil
}

// reverseOne 对单条操作日志执行逆向操作。
func reverseOne(tx *gorm.DB, r OperationLogRecord) error {
	var pkMap map[string]any
	if err := json.Unmarshal([]byte(r.EntityID), &pkMap); err != nil {
		return fmt.Errorf("parse entity_id: %w", err)
	}

	switch r.Operation {
	case "create":
		// INSERT 的逆向：DELETE
		return tx.Table(r.Table).Where(pkMap).Delete(nil).Error

	case "update":
		// UPDATE 的逆向：用 old_values 覆盖整行
		var old map[string]any
		if err := json.Unmarshal([]byte(r.OldValues), &old); err != nil {
			return fmt.Errorf("parse old_values: %w", err)
		}
		return tx.Table(r.Table).Where(pkMap).Updates(old).Error

	case "delete":
		// DELETE 的逆向：INSERT 回旧值
		var old map[string]any
		if err := json.Unmarshal([]byte(r.OldValues), &old); err != nil {
			return fmt.Errorf("parse old_values: %w", err)
		}
		return tx.Table(r.Table).Create(old).Error

	default:
		return fmt.Errorf("unknown operation type: %s", r.Operation)
	}
}
