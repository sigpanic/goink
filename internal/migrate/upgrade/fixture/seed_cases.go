package fixture

import (
	"github.com/sigpanic/goink/internal/migrate/upgrade/fixture/cases/v100"
	"github.com/sigpanic/goink/internal/migrate/upgrade/fixture/cases/v110"
	"github.com/sigpanic/goink/internal/migrate/upgrade/fixture/cases/v120"
	"github.com/sigpanic/goink/internal/migrate/upgrade/fixture/cases/v130"
	"github.com/sigpanic/goink/internal/migrate/upgrade/fixture/cases/v160"
	"github.com/sigpanic/goink/internal/migrate/upgrade/fixture/cases/v161"
	"github.com/sigpanic/goink/internal/migrate/upgrade/fixture/cases/v180"
	"github.com/sigpanic/goink/internal/migrate/upgrade/fixture/seeddata"
)

func seedCases() map[string]seeddata.SeedCase {
	return map[string]seeddata.SeedCase{
		"novels":                v100.Novels(),
		"chapters-numbered":     v100.Chapters(),
		"characters":            v100.Characters(),
		"references-v100":       v100.References(),
		"preferences":           v100.Preferences(),
		"settings-legacy":       v100.Settings(),
		"sessions-legacy":       v100.Sessions(),
		"messages":              v100.Messages(),
		"writing-v100":          v100.Writing(),
		"llm-v1":                v100.LLM(),
		"locations":             v100.Locations(),
		"styles":                v110.Styles(),
		"references-v120":       v120.References(),
		"writing-v120":          v120.Writing(),
		"settings-legacy-relay": v130.Settings(),
		"setting-items":         v130.SettingItems(),
		"chapters-id":           v160.Chapters(),
		"references-id":         v160.References(),
		"writing-id":            v160.Writing(),
		"volumes":               v160.Volumes(),
		"writing-daily":         v161.Writing(),
		"groups":                v161.Groups(),
		"activity":              v161.Activity(),
		"settings-native":       v180.Settings(),
		"sessions-native":       v180.Sessions(),
		"llm-v2":                v180.LLM(),
	}
}
