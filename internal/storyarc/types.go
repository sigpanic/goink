package storyarc

import "time"

// StoryArc 是跨越多章节的叙事弧线容器，处于 ChapterPlan 和整体大纲之间的战略层。
// 一个小说通常 3-5 条弧线，弧线之间靠自然语言关联，不需要结构化边表。
//
// arc_type 驱动过滤和注入策略，status 驱动活跃窗口筛选，均保持约束枚举。
type StoryArc struct {
	ID           int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	NovelID      int64     `gorm:"column:novel_id;not null;index"    json:"novel_id"`
	Name         string    `gorm:"column:name;not null"              json:"name"`          // 弧线名称，如"复仇之路"
	Description  string    `gorm:"column:description"                json:"description"`   // 弧线整体描述
	ArcType      string    `gorm:"column:arc_type;not null;index"    json:"arc_type"`      // "main" | "sub" | "character" | "background"
	Importance   int       `gorm:"column:importance;default:1"       json:"importance"`    // 1-5，同类型内的排序优先度
	Status       string    `gorm:"column:status;not null;index"      json:"status"`        // "active" | "paused" | "completed" | "abandoned"
	ReactivateAt string    `gorm:"column:reactivate_at"              json:"reactivate_at"` // 自然语言，暂停弧线的恢复条件。LLM 填写，MCP 工具格式化后呈现给 LLM 自行判断
	CreatedAt    time.Time `gorm:"column:created_at;autoCreateTime"  json:"created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at;autoUpdateTime"  json:"updated_at"`
}

func (StoryArc) TableName() string { return "story_arcs" }

// ArcNode 是弧线内的有序链节，承接 Python plot_node 的设计意图。
//
// 排序由 target_chapter ASC, id ASC 替代序列号——创建顺序天然打破同章平局。
// target_chapter 是 LLM 对不确定未来的估算，只排不滤——不准确时 review agent 校准。
// actual_chapter 记录实际发生在哪章，0=未发生。
type ArcNode struct {
	ID              int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	NovelID         int64     `gorm:"column:novel_id;not null;index"         json:"novel_id"`
	StoryArcID      int64     `gorm:"column:story_arc_id;not null;index"     json:"story_arc_id"`
	Title           string    `gorm:"column:title;not null"                  json:"title"`              // "发现仇人身份"
	Description     string    `gorm:"column:description"                     json:"description"`        // 节点详情
	TargetChapter   int       `gorm:"column:target_chapter;default:0"        json:"target_chapter"`     // 预计章节，0=未定
	ActualChapter   int       `gorm:"column:actual_chapter;default:0"        json:"actual_chapter"`     // 实际章节，0=未发生
	TargetChapterID *int64    `gorm:"column:target_chapter_id;index"          json:"target_chapter_id"` // v1.5.0 新增：chapters.id 外键，commit 1.5 由 TargetChapter 反查填充
	ActualChapterID *int64    `gorm:"column:actual_chapter_id;index"          json:"actual_chapter_id"` // v1.5.0 新增：chapters.id 外键，commit 1.5 由 ActualChapter 反查填充
	Status          string    `gorm:"column:status;not null;default:pending" json:"status"`             // "pending" | "completed" | "abandoned"
	CreatedAt       time.Time `gorm:"column:created_at;autoCreateTime"       json:"created_at"`
	UpdatedAt       time.Time `gorm:"column:updated_at;autoUpdateTime"       json:"updated_at"`
}

func (ArcNode) TableName() string { return "arc_nodes" }
