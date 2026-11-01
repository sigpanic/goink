package migrate

import (
	"github.com/sigpanic/goink/internal/activity"
	"github.com/sigpanic/goink/internal/chapter"
	"github.com/sigpanic/goink/internal/character"
	"github.com/sigpanic/goink/internal/config"
	"github.com/sigpanic/goink/internal/location"
	"github.com/sigpanic/goink/internal/migrate/engine"
	"github.com/sigpanic/goink/internal/novel"
	"github.com/sigpanic/goink/internal/preference"
	"github.com/sigpanic/goink/internal/reader"
	"github.com/sigpanic/goink/internal/rollback"
	"github.com/sigpanic/goink/internal/session"
	"github.com/sigpanic/goink/internal/setting"
	"github.com/sigpanic/goink/internal/storage"
	"github.com/sigpanic/goink/internal/storyarc"
	"github.com/sigpanic/goink/internal/style"
	"github.com/sigpanic/goink/internal/timeline"
	"github.com/sigpanic/goink/internal/volume"
	"github.com/sigpanic/goink/internal/writing"
)

// Models 返回当前全部模型，用于新库初始化、最终 schema 对齐及生成冻结快照。
// 快照生成器会排除 MigrateState。
func Models() []any {
	return []any{
		&config.AppSettings{},
		&novel.Novel{},
		&preference.PreferenceItem{},
		&setting.SettingItem{},
		&chapter.Chapter{},
		&character.Character{},
		&character.CharacterRelation{},
		&character.Group{},
		&character.GroupMember{},
		&timeline.TimelineEntry{},
		&storyarc.StoryArc{},
		&storyarc.ArcNode{},
		&location.Location{},
		&location.LocationRelation{},
		&reader.ReaderPerspective{},
		&session.Session{},
		&session.Message{},
		&storage.OperationLogRecord{},
		&rollback.TurnCommit{},
		&style.Sample{},
		&writing.WritingLog{},
		&activity.DailyActivity{},
		&activity.DailyLLMUsage{},
		&volume.Volume{},
		&engine.MigrateState{},
	}
}
