package search

// Result 是统一搜索结果的单条记录。
type Result struct {
	Type          string  `json:"type"`           // character / location / timeline / storyarc / chapter / content / rag
	ID            int64   `json:"id"`             // 实体 ID（content/rag 为 0）
	Title         string  `json:"title"`          // 主显示名
	Subtitle      string  `json:"subtitle"`       // 副标题（类型标签等）
	ChapterNum    int     `json:"chapter_num"`    // 关联章节号
	FilePath      string  `json:"file_path"`      // 章节文件路径
	MatchPrefix   string  `json:"match_prefix"`   // 命中前上下文（纯文本）
	MatchHit      string  `json:"match_hit"`      // 命中的文本（纯文本）
	MatchSuffix   string  `json:"match_suffix"`   // 命中后上下文（纯文本）
	MatchPosition int     `json:"match_position"` // 命中 rune 偏移，用于编辑器定位
	MatchLen      int     `json:"match_len"`      // 命中长度（rune），精确搜索=关键词长，RAG=chunk长
	Relevance     float64 `json:"relevance"`      // 相关度 0-1（精确搜索为 1）
	PanelID       string  `json:"panel_id"`       // 跳转目标面板 ID
}

// 搜索参数常量
const (
	EntityLimit   = 5  // 每类实体最多返回数
	RagTopK       = 8  // RAG 返回数
	ContentLimit  = 10 // 正文精确匹配最多返回数
	ContextRadius = 15 // 命中上下文半径（中文字符），mark 居中简短
)
