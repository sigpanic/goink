package activity

// ActivityDelta 是一次操作对全局每日统计的非负增量。
type ActivityDelta struct {
	NovelsCreated             int64 `gorm:"column:novels_created;not null;default:0"`
	VolumesCreated            int64 `gorm:"column:volumes_created;not null;default:0"`
	ChaptersCreated           int64 `gorm:"column:chapters_created;not null;default:0"`
	CharactersCreated         int64 `gorm:"column:characters_created;not null;default:0"`
	CharacterGroupsCreated    int64 `gorm:"column:character_groups_created;not null;default:0"`
	CharacterRelationsCreated int64 `gorm:"column:character_relations_created;not null;default:0"`
	LocationsCreated          int64 `gorm:"column:locations_created;not null;default:0"`
	LocationRelationsCreated  int64 `gorm:"column:location_relations_created;not null;default:0"`
	StoryArcsCreated          int64 `gorm:"column:story_arcs_created;not null;default:0"`
	ArcNodesCreated           int64 `gorm:"column:arc_nodes_created;not null;default:0"`
	TimelineEntriesCreated    int64 `gorm:"column:timeline_entries_created;not null;default:0"`
	ReaderPerspectivesCreated int64 `gorm:"column:reader_perspectives_created;not null;default:0"`
	SettingsCreated           int64 `gorm:"column:settings_created;not null;default:0"`
	PreferencesCreated        int64 `gorm:"column:preferences_created;not null;default:0"`
	StyleSamplesCreated       int64 `gorm:"column:style_samples_created;not null;default:0"`
	SkillsCreated             int64 `gorm:"column:skills_created;not null;default:0"`
	ImagesImported            int64 `gorm:"column:images_imported;not null;default:0"`
	SessionsCreated           int64 `gorm:"column:sessions_created;not null;default:0"`
	CreativeSeconds           int64 `gorm:"column:creative_seconds;not null;default:0"`
	ConversationTurns         int64 `gorm:"column:conversation_turns;not null;default:0"`
	ToolCalls                 int64 `gorm:"column:tool_calls;not null;default:0"`
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
