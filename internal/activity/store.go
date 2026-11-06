package activity

import (
	"context"
	"log/slog"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/sigpanic/goink/internal/config"
)

// Store 负责每日统计的尽力记录；写入失败只告警，不向业务调用方返回错误。
type Store struct {
	db     *gorm.DB
	logger *slog.Logger
	now    func() time.Time
}

func NewStore(db *gorm.DB, logger *slog.Logger) *Store {
	return &Store{db: db, logger: logger, now: time.Now}
}

// InitTracking 在表就绪后、加载运行时 AppSettings 前调用，保留已有统计起点。
// 起点写入 app_config；先初始化再加载设置，避免后续 SaveSettings 写回旧快照。
func (s *Store) InitTracking(ctx context.Context) {
	now := s.now()
	startedAt := now.UTC()
	settings := config.AppSettings{
		ID:                1,
		TrackingStartedAt: &startedAt,
		TrackingStartDate: now.Format(time.DateOnly),
	}
	err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"tracking_started_at": startedAt,
			"tracking_start_date": settings.TrackingStartDate,
		}),
		Where: clause.Where{Exprs: []clause.Expression{
			clause.Eq{Column: clause.Column{Name: "tracking_started_at"}, Value: nil},
		}},
	}).Create(&settings).Error
	if err != nil {
		s.logger.Warn("初始化创作统计起点失败", "err", err)
	}
}

// AddActivity 将非负增量原子累加到当天；全零增量不创建空行。
// 调用位置应在业务事务外，统计失败无需补偿或重试。
func (s *Store) AddActivity(ctx context.Context, delta ActivityDelta) {
	if delta == (ActivityDelta{}) {
		return
	}
	if !nonNegative(delta.NovelsCreated, delta.VolumesCreated, delta.ChaptersCreated,
		delta.CharactersCreated, delta.CharacterGroupsCreated, delta.CharacterRelationsCreated,
		delta.LocationsCreated, delta.LocationRelationsCreated, delta.StoryArcsCreated,
		delta.ArcNodesCreated, delta.TimelineEntriesCreated, delta.ReaderPerspectivesCreated,
		delta.SettingsCreated, delta.PreferencesCreated, delta.StyleSamplesCreated,
		delta.SkillsCreated, delta.ImagesImported, delta.SessionsCreated,
		delta.CreativeSeconds, delta.ConversationTurns, delta.ToolCalls) {
		s.logger.Warn("忽略负数创作统计增量")
		return
	}
	record := DailyActivity{Date: s.now().Format(time.DateOnly), ActivityDelta: delta}
	err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "date"}},
		DoUpdates: clause.Assignments(map[string]any{
			"novels_created":              gorm.Expr("novels_created + excluded.novels_created"),
			"volumes_created":             gorm.Expr("volumes_created + excluded.volumes_created"),
			"chapters_created":            gorm.Expr("chapters_created + excluded.chapters_created"),
			"characters_created":          gorm.Expr("characters_created + excluded.characters_created"),
			"character_groups_created":    gorm.Expr("character_groups_created + excluded.character_groups_created"),
			"character_relations_created": gorm.Expr("character_relations_created + excluded.character_relations_created"),
			"locations_created":           gorm.Expr("locations_created + excluded.locations_created"),
			"location_relations_created":  gorm.Expr("location_relations_created + excluded.location_relations_created"),
			"story_arcs_created":          gorm.Expr("story_arcs_created + excluded.story_arcs_created"),
			"arc_nodes_created":           gorm.Expr("arc_nodes_created + excluded.arc_nodes_created"),
			"timeline_entries_created":    gorm.Expr("timeline_entries_created + excluded.timeline_entries_created"),
			"reader_perspectives_created": gorm.Expr("reader_perspectives_created + excluded.reader_perspectives_created"),
			"settings_created":            gorm.Expr("settings_created + excluded.settings_created"),
			"preferences_created":         gorm.Expr("preferences_created + excluded.preferences_created"),
			"style_samples_created":       gorm.Expr("style_samples_created + excluded.style_samples_created"),
			"skills_created":              gorm.Expr("skills_created + excluded.skills_created"),
			"images_imported":             gorm.Expr("images_imported + excluded.images_imported"),
			"sessions_created":            gorm.Expr("sessions_created + excluded.sessions_created"),
			"creative_seconds":            gorm.Expr("creative_seconds + excluded.creative_seconds"),
			"conversation_turns":          gorm.Expr("conversation_turns + excluded.conversation_turns"),
			"tool_calls":                  gorm.Expr("tool_calls + excluded.tool_calls"),
		}),
	}).Create(&record).Error
	if err != nil {
		s.logger.Warn("记录每日创作统计失败", "date", record.Date, "err", err)
	}
}

// AddLLMUsage 累计一次已取得的最终 usage，包括服务商明确返回全零的用量。
// 调用方跳过缺失 usage 的请求，并负责从原始响应中提取单次用量。
func (s *Store) AddLLMUsage(ctx context.Context, provider, model, purpose string, usage TokenUsage) {
	if !nonNegative(usage.InputTokens, usage.OutputTokens, usage.TotalTokens,
		usage.CacheHitTokens, usage.CacheMissTokens, usage.ReasoningTokens) {
		s.logger.Warn("忽略负数 Token 统计增量", "provider", provider, "model", model)
		return
	}
	record := DailyLLMUsage{
		Date:       s.now().Format(time.DateOnly),
		Provider:   provider,
		Model:      model,
		Purpose:    purpose,
		UsageCount: 1,
		TokenUsage: usage,
	}
	err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "date"}, {Name: "provider"}, {Name: "model"}, {Name: "purpose"}},
		DoUpdates: clause.Assignments(map[string]any{
			"usage_count":       gorm.Expr("usage_count + excluded.usage_count"),
			"input_tokens":      gorm.Expr("input_tokens + excluded.input_tokens"),
			"output_tokens":     gorm.Expr("output_tokens + excluded.output_tokens"),
			"total_tokens":      gorm.Expr("total_tokens + excluded.total_tokens"),
			"cache_hit_tokens":  gorm.Expr("cache_hit_tokens + excluded.cache_hit_tokens"),
			"cache_miss_tokens": gorm.Expr("cache_miss_tokens + excluded.cache_miss_tokens"),
			"reasoning_tokens":  gorm.Expr("reasoning_tokens + excluded.reasoning_tokens"),
		}),
	}).Create(&record).Error
	if err != nil {
		s.logger.Warn("记录每日 Token 用量失败", "date", record.Date, "provider", provider, "model", model, "err", err)
	}
}

func nonNegative(values ...int64) bool {
	for _, value := range values {
		if value < 0 {
			return false
		}
	}
	return true
}
