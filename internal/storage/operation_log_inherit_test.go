package storage

import (
	"context"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	glog "gorm.io/gorm/logger"
)

// TestEntity 用于测试 operation_log 回调的测试实体
type TestEntity struct {
	ID    int64  `gorm:"column:id;primaryKey;autoIncrement"`
	Name  string `gorm:"column:name"`
	Value string `gorm:"column:value"`
}

func (TestEntity) TableName() string { return "test_entities" }

// setupOplogTestDB 创建内存 SQLite，注册 operation_log 回调
func setupOplogTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: glog.Default.LogMode(glog.Silent),
	})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := db.AutoMigrate(&TestEntity{}, &OperationLogRecord{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := RegisterOplogHooks(db); err != nil {
		t.Fatalf("register oplog hooks: %v", err)
	}
	return db
}

// TestResolveTurnForEntity_NoHistory 验证无历史记录时 fallback 为 turn_id=0
func TestResolveTurnForEntity_NoHistory(t *testing.T) {
	db := setupOplogTestDB(t)

	// 直接创建（无 TurnInfo）→ 回调触发 resolveTurnForEntity
	// 此时 operation_log 为空 → fallback turn_id=0
	e := TestEntity{Name: "test", Value: "v1"}
	if err := db.Create(&e).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	// 验证 operation_log 记录了 turn_id=0
	var rec OperationLogRecord
	if err := db.Where("table_name = ?", "test_entities").First(&rec).Error; err != nil {
		t.Fatalf("should have log record: %v", err)
	}
	if rec.TurnID != 0 || rec.SessionID != "" {
		t.Errorf("expected turn_id=0, session_id='', got turn_id=%d, session_id=%q", rec.TurnID, rec.SessionID)
	}
}

// TestResolveTurnForEntity_InheritFromAI 验证用户编辑继承 AI 的 session/turn
func TestResolveTurnForEntity_InheritFromAI(t *testing.T) {
	db := setupOplogTestDB(t)

	// 1. AI 在 turn 5 创建实体
	ctx := WithTurn(context.Background(), "sess_A", 5)
	e := TestEntity{Name: "alice", Value: "ai_created"}
	if err := db.WithContext(ctx).Create(&e).Error; err != nil {
		t.Fatalf("ai create: %v", err)
	}

	// 验证 AI 操作记录
	var aiRec OperationLogRecord
	if err := db.Where("table_name = ? AND turn_id = ?", "test_entities", 5).First(&aiRec).Error; err != nil {
		t.Fatalf("should have AI log: %v", err)
	}
	if aiRec.SessionID != "sess_A" || aiRec.TurnID != 5 {
		t.Fatalf("AI record wrong: session=%s, turn=%d", aiRec.SessionID, aiRec.TurnID)
	}

	// 2. 用户编辑（无 TurnInfo）→ 应继承 turn 5
	e.Value = "user_edited"
	if err := db.Save(&e).Error; err != nil {
		t.Fatalf("user edit: %v", err)
	}

	// 验证用户编辑记录继承了 turn 5
	var userRec OperationLogRecord
	if err := db.Where("table_name = ? AND operation = ?", "test_entities", "update").First(&userRec).Error; err != nil {
		t.Fatalf("should have user update log: %v", err)
	}
	if userRec.TurnID != 5 || userRec.SessionID != "sess_A" {
		t.Errorf("expected inherit turn_id=5, session_id='sess_A', got turn_id=%d, session_id=%q",
			userRec.TurnID, userRec.SessionID)
	}
}

// TestResolveTurnForEntity_ChainedInheritance 验证链式继承
// AI编辑 → 用户编辑 → 用户再编辑，三次都应绑定到同一 turn
func TestResolveTurnForEntity_ChainedInheritance(t *testing.T) {
	db := setupOplogTestDB(t)

	// 1. 用户先创建实体（无 AI 参与）→ turn_id=0
	e := TestEntity{Name: "bob", Value: "user_created"}
	if err := db.Create(&e).Error; err != nil {
		t.Fatalf("user create: %v", err)
	}

	// 2. AI 在 turn 5 编辑
	ctx := WithTurn(context.Background(), "sess_A", 5)
	e.Value = "ai_edited"
	if err := db.WithContext(ctx).Save(&e).Error; err != nil {
		t.Fatalf("ai edit: %v", err)
	}

	// 3. 用户编辑（无 TurnInfo）→ 应继承 turn 5
	e.Value = "user_edited_1"
	if err := db.Save(&e).Error; err != nil {
		t.Fatalf("user edit 1: %v", err)
	}

	// 4. 用户再编辑（无 TurnInfo）→ 应继承上一条的 turn 5
	e.Value = "user_edited_2"
	if err := db.Save(&e).Error; err != nil {
		t.Fatalf("user edit 2: %v", err)
	}

	// 验证所有 update 记录都是 turn 5
	var updates []OperationLogRecord
	if err := db.Where("table_name = ? AND operation = ?", "test_entities", "update").
		Order("id ASC").Find(&updates).Error; err != nil {
		t.Fatalf("query updates: %v", err)
	}

	if len(updates) != 3 { // AI edit + 2 user edits
		t.Fatalf("expected 3 update records, got %d", len(updates))
	}

	for i, rec := range updates {
		if rec.TurnID != 5 || rec.SessionID != "sess_A" {
			t.Errorf("update[%d]: expected turn_id=5, session_id='sess_A', got turn_id=%d, session_id=%q",
				i, rec.TurnID, rec.SessionID)
		}
	}
}

// TestResolveTurnForEntity_PureUserEntity 验证纯用户实体不被回滚
func TestResolveTurnForEntity_PureUserEntity(t *testing.T) {
	db := setupOplogTestDB(t)

	// 用户创建 + 编辑（无 AI 参与）
	e := TestEntity{Name: "charlie", Value: "v1"}
	if err := db.Create(&e).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	e.Value = "v2"
	if err := db.Save(&e).Error; err != nil {
		t.Fatalf("edit: %v", err)
	}

	// 所有记录都应是 turn_id=0
	var recs []OperationLogRecord
	if err := db.Where("table_name = ?", "test_entities").Find(&recs).Error; err != nil {
		t.Fatalf("query: %v", err)
	}

	for i, rec := range recs {
		if rec.TurnID != 0 || rec.SessionID != "" {
			t.Errorf("record[%d]: expected turn_id=0, got turn_id=%d, session_id=%q",
				i, rec.TurnID, rec.SessionID)
		}
	}
}
