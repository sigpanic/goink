package v100

import (
	"github.com/sigpanic/goink/internal/migrate/upgrade/fixture/seeddata"
)

func Messages() seeddata.SeedCase {
	before := seeddata.SeedData{
		Rows: []seeddata.SeedRow{
			seeddata.Row("messages", map[string]any{"id": 1, "session_id": "upgrade-session", "role": "user", "content": "消息1", "version": 2, "to_api": true, "to_frontend": false}),
			seeddata.Row("messages", map[string]any{"id": 2, "session_id": "upgrade-session", "role": "user", "content": "消息2", "version": 1, "to_api": false, "to_frontend": true}),
			seeddata.Row("messages", map[string]any{"id": 3, "session_id": "upgrade-session", "role": "user", "content": "消息3", "version": 2, "to_api": false, "to_frontend": false}),
		},
	}
	return seeddata.SeedCase{
		ID:     "messages",
		Before: before,
		AssertAfter: func(result *seeddata.UpgradeResult) error {
			return seeddata.AssertRows(result, before.Rows)
		},
	}
}
