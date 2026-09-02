package character

import "time"

// Character 是角色元数据。
// personality 和 abilities 为 JSON 自由格式，由 LLM 填写和读取，不做结构化约束。
type Character struct {
	ID          int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	NovelID     int64     `gorm:"column:novel_id;not null;index"    json:"novel_id"`
	Name        string    `gorm:"column:name;not null;index"        json:"name"`
	Description string    `gorm:"column:description"                json:"description"` // 自然语言描述，LLM 填写
	Personality string    `gorm:"column:personality"                json:"personality"` // JSON 自由格式，LLM 填写。如 {"traits":["勇敢","冲动"],"brief":"一个热血青年"}
	Abilities   string    `gorm:"column:abilities"                  json:"abilities"`   // JSON 数组，如 ["剑术","隐身"]
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime"  json:"created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at;autoUpdateTime"  json:"updated_at"`
}

// TableName 指定 GORM 表名。
func (Character) TableName() string { return "characters" }

// CharacterRelation 是角色之间有向关系的当前快照。
//
// 设计原则（与 Python 版本的关键差异）：
//  1. 自由文本 relation_describe — LLM 用自然语言描述关系（如 "朋友、高中同学、暗中嫉妒"），
//     不再受枚举 18 种类型的限制。单行可表达多面关系。
//  2. 追加不可变 (append-only) — 关系变更时不修改旧行，而是 INSERT 新行并将旧行 is_current 置 false。
//     同一对 (source_id, target_id) 的全部记录按 created_at 排序即为完整演变历史。
//  3. 不需要 evolved_from_id 指针链 — 用 (source,target) 配对 + 时间戳隐式排序替代显式链表，
//     消除断链风险和并发分叉问题。Python 因为枚举 type 导致一对角色需要多行并行记录，
//     每行各自演化，才被迫引入自引用链；自由文本 relation_describe 从根上消除了这个需求。
//  4. 不需要 status 字段 — is_current 布尔替代 active/dormant/resolved/severed 四状态枚举，
//     关系状态只需区分"当前"和"历史"。
//
// 图查询支持：
//   - 全图：WHERE novel_id = ? AND is_current = true  → 所有节点+边，前端可渲染关系图
//   - 单角色邻域：WHERE (source_id = ? OR target_id = ?) AND is_current = true
//   - 两角色之间：WHERE (source_id = A AND target_id = B) OR (source_id = B AND target_id = A)
//
// MCP 工具实现参考：
//   - update_character_relationship: 变更时开启事务，旧行 SET is_current = false，INSERT 新行，COMMIT
//   - get_character_network: 查当前全图，返回 nodes(Character 列表) + edges(CharacterRelation 列表) 可以加入ids参数只查涉及到的人的关系网
//   - get_character_history: WHERE source_id = X AND target_id = Y ORDER BY created_at，
//     可选通过 chapter_number JOIN chapters 拉取对应章节摘要补充上下文
//
// TODO
// 后续可实现前端的可视化关系图绘制
type CharacterRelation struct {
	ID                int64     `gorm:"column:id;primaryKey;autoIncrement"           json:"id"`
	NovelID           int64     `gorm:"column:novel_id;not null;index"              json:"novel_id"`
	SourceCharacterID int64     `gorm:"column:source_character_id;not null;index"   json:"source_character_id"` // 关系发出方
	TargetCharacterID int64     `gorm:"column:target_character_id;not null;index"   json:"target_character_id"` // 关系接收方
	RelationDescribe  string    `gorm:"column:relation_describe;not null"          json:"relation_describe"`    // 自由文本，LLM 自行描述，如 "亦师亦敌"、"朋友、高中同学"，工具描述的时候需要告诉llm详细描述而不是简单的type
	Description       string    `gorm:"column:description"                          json:"description"`         // 当前关系阶段的详细描述
	ChapterNumber     int       `gorm:"column:chapter_number"                      json:"chapter_number"`       // 此关系在哪个章节确立/变更，可用于拉取章节细节作为 LLM 上下文；v1.5.0 迁移期保留，commit 1.7 删除
	ChapterID         *int64    `gorm:"column:chapter_id;index"                     json:"chapter_id"`           // v1.5.0 新增：chapters.id 外键（nullable）；commit 1.5 由 ChapterNumber 反查填充
	IsCurrent         bool      `gorm:"column:is_current;not null;index"            json:"is_current"`          // true=当前有效关系，false=历史记录
	CreatedAt         time.Time `gorm:"column:created_at;autoCreateTime"            json:"created_at"`
}

// TableName 指定 GORM 表名。
func (CharacterRelation) TableName() string { return "character_relations" }
