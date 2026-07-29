package style

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

// StringSlice 是 []string 的 DB 适配类型，存储为 JSON TEXT。
type StringSlice []string

// Scan 实现 sql.Scanner，从 DB 读取 JSON TEXT → []string。
func (s *StringSlice) Scan(value any) error {
	if value == nil {
		*s = nil
		return nil
	}
	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return fmt.Errorf("StringSlice: cannot scan %T", value)
	}
	return json.Unmarshal(bytes, s)
}

// Value 实现 driver.Valuer，从 []string → JSON TEXT 写入 DB。
func (s StringSlice) Value() (driver.Value, error) {
	if s == nil {
		return "[]", nil
	}
	b, err := json.Marshal([]string(s))
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

// Sample 是一条风格素材的 GORM model。
type Sample struct {
	ID        int64       `gorm:"column:id;primaryKey;autoIncrement"    json:"id"`
	NovelID   int64       `gorm:"column:novel_id;not null"               json:"novel_id"`
	IsGlobal  bool        `gorm:"column:is_global;not null"              json:"is_global"`
	Name      string      `gorm:"column:name;not null"                   json:"name"`
	Content   string      `gorm:"column:content;not null"                json:"content"`
	Preview   string      `gorm:"column:preview;not null"                json:"preview"`
	Tags      StringSlice `gorm:"column:tags;type:text;not null"         json:"tags"`
	WordCount int         `gorm:"column:word_count;not null"             json:"word_count"`
	CreatedAt time.Time   `gorm:"column:created_at;autoCreateTime"       json:"created_at"`
	UpdatedAt time.Time   `gorm:"column:updated_at;autoUpdateTime"       json:"updated_at"`
}

func (Sample) TableName() string { return "style_samples" }

// Stats 是代码计算的确定性文本统计，不依赖 LLM。
type Stats struct {
	TotalChars      int     `json:"total_chars"`
	TotalWords      int     `json:"total_words"`
	SentenceCount   int     `json:"sentence_count"`
	ShortSentPct    float64 `json:"short_sent_pct"` // <15 字
	MidSentPct      float64 `json:"mid_sent_pct"`   // 15-30 字
	LongSentPct     float64 `json:"long_sent_pct"`  // >30 字
	AvgSentLen      float64 `json:"avg_sent_len"`
	SentLenStdDev   float64 `json:"sent_len_std_dev"` // 句长标准差
	CommaDensity    float64 `json:"comma_density"`    // 逗号占比
	PeriodDensity   float64 `json:"period_density"`   // 句号占比
	ExclaimDensity  float64 `json:"exclaim_density"`  // 感叹号占比
	QuestionDensity float64 `json:"question_density"` // 问号占比
	QuoteDensity    float64 `json:"quote_density"`    // 引号占比
	ParagraphCount  int     `json:"paragraph_count"`
	AvgParaLen      float64 `json:"avg_para_len"`
}

// ExtractResult 是风格提取的返回值。
type ExtractResult struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	RawContent  string `json:"raw_content"`
	FilePath    string `json:"file_path"`
}

// StyleSkillOutput 是风格提取 toolcall 的结构化输出，由 LLM 调用 output_style_skill 工具填充。
type StyleSkillOutput struct {
	Name        string `json:"name" jsonschema:"required,description=风格名称（中文）"`
	Description string `json:"description" jsonschema:"required,description=一句话描述适合什么写作场景，什么时候调用"`
	Content     string `json:"content" jsonschema:"required,description=风格仿写技能正文 markdown，含 ## 风格概述/句式特征/用词习惯/修辞手法/节奏控制/叙事视角与距离/氛围与语调/仿写要点/原文锚点 等章节，不含 frontmatter"`
}
