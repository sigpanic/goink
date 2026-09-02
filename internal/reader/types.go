package reader

import "time"

// PerspectiveType 定义读者认知条目的类型。
const (
	TypeKnown         = "known"         // 读者已知的信息
	TypeSuspense      = "suspense"      // 读者等待解答的悬念
	TypeMisconception = "misconception" // 读者误以为的情况（用于反转）
)

// ReaderPerspective 是读者认知条目，追踪"读者知道什么、在等什么答案、误以为是什么"。
//
// 三种类型驱动不同的查询和格式化行为：
//   - known：全量返回，不过滤 revealed_chapter
//   - suspense/misconception：只返回 revealed_chapter=0（未回收）的条目
//   - 格式化时分三段输出：已知信息 / 活跃悬念 / 读者误知
//
// 字段精简：Python 9 字段直接平移，无删减。
//
// related_truth 是作者全知视角——不仅 misconception，所有类型都可记录真相。
// "谁杀了村长"是悬念，答案作者心里应该有数。
type ReaderPerspective struct {
	ID                int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	NovelID           int64     `gorm:"column:novel_id;not null;index"      json:"novel_id"`
	Type              string    `gorm:"column:type;not null;index"          json:"type"`               // "known" | "suspense" | "misconception"
	Content           string    `gorm:"column:content;not null"             json:"content"`            // 条目内容：读者知道/想知道/误以为的事情
	RelatedTruth      string    `gorm:"column:related_truth"                json:"related_truth"`      // 作者视角：真实情况。所有类型可选，不只是 misconception
	PlantedChapter    int       `gorm:"column:planted_chapter;not null"     json:"planted_chapter"`    // 在哪章种下
	RevealedChapter   int       `gorm:"column:revealed_chapter;default:0"   json:"revealed_chapter"`   // 在哪章回收，0=未回收
	PlantedChapterID  *int64    `gorm:"column:planted_chapter_id;index"      json:"planted_chapter_id"` // v1.5.0 新增：chapters.id 外键，commit 1.5 由 PlantedChapter 反查填充
	RevealedChapterID *int64    `gorm:"column:revealed_chapter_id;index"     json:"revealed_chapter_id"` // v1.5.0 新增：chapters.id 外键，commit 1.5 由 RevealedChapter 反查填充
	CreatedAt         time.Time `gorm:"column:created_at;autoCreateTime"    json:"created_at"`
}

// TableName 指定 GORM 表名。
func (ReaderPerspective) TableName() string { return "reader_perspectives" }
