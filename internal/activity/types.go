package activity

// ActivityDelta 是一次操作对全局每日统计的非负增量。
type ActivityDelta struct {
	NovelsCreated     int64 `gorm:"column:novels_created;not null;default:0"`
	ChaptersCreated   int64 `gorm:"column:chapters_created;not null;default:0"`
	CharactersCreated int64 `gorm:"column:characters_created;not null;default:0"`
	LocationsCreated  int64 `gorm:"column:locations_created;not null;default:0"`
	CreativeSeconds   int64 `gorm:"column:creative_seconds;not null;default:0"`
	ConversationTurns int64 `gorm:"column:conversation_turns;not null;default:0"`
	ToolCalls         int64 `gorm:"column:tool_calls;not null;default:0"`
}

// DailyActivity 按记录时的本地日期累计，不与小说或会话建立级联关系。
type DailyActivity struct {
	Date          string `gorm:"column:date;primaryKey;size:10"`
	ActivityDelta `gorm:"embedded"`
}

func (DailyActivity) TableName() string { return "activity_daily" }

// TokenUsage 是调用方已取得的单次用量；缺失字段不贡献增量。
// TotalTokens 由调用方按服务商口径确定，缓存和推理明细不再加进总量。
type TokenUsage struct {
	InputTokens     int64 `gorm:"column:input_tokens;not null;default:0"`
	OutputTokens    int64 `gorm:"column:output_tokens;not null;default:0"`
	TotalTokens     int64 `gorm:"column:total_tokens;not null;default:0"`
	CacheHitTokens  int64 `gorm:"column:cache_hit_tokens;not null;default:0"`
	CacheMissTokens int64 `gorm:"column:cache_miss_tokens;not null;default:0"`
	ReasoningTokens int64 `gorm:"column:reasoning_tokens;not null;default:0"`
}

// DailyLLMUsage 按日期、服务商、模型和用途累计已取得的用量。
type DailyLLMUsage struct {
	Date       string `gorm:"column:date;primaryKey;size:10"`
	Provider   string `gorm:"column:provider;primaryKey"`
	Model      string `gorm:"column:model;primaryKey"`
	Purpose    string `gorm:"column:purpose;primaryKey"`
	UsageCount int64  `gorm:"column:usage_count;not null;default:0"`
	TokenUsage `gorm:"embedded"`
}

func (DailyLLMUsage) TableName() string { return "llm_usage_daily" }
