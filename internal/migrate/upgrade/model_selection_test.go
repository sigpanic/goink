//go:build cgo && upgrade

package upgrade_test

import (
	"database/sql"
	"path/filepath"
	"reflect"
	"testing"
)

func TestHistoricalModelSelectionUpgrade(t *testing.T) {
	samples := historicalFixtures(t)
	sample := samples[len(samples)-1]
	gitBin := requireGit(t)
	for _, tt := range []struct{ name, key, provider, model string }{
		{"builtin", "deepseek/fixture-deepseek", "DeepSeek", "fixture-deepseek"},
		{"relay", "升级中转站/vendor/model", "升级中转站", "vendor/model"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := prepare(t, sample)
			// 在真实历史 schema 中模拟用户保存选择，仍只通过公开 Run 执行升级。
			mutate(t, dir, "UPDATE app_config SET selected_model_key='"+tt.key+"'")
			mutate(t, dir, "UPDATE sessions SET model='vendor/model'")
			oldSchema := readSchema(t, filepath.Join(dir, "novel-agent.db"))
			effort := ""
			if _, exists := oldSchema["app_config"].Columns["reasoning_effort"]; exists {
				mutate(t, dir, "UPDATE app_config SET reasoning_effort='high'")
				effort = "high"
			}
			if _, exists := oldSchema["sessions"].Columns["reasoning_effort"]; exists {
				mutate(t, dir, "UPDATE sessions SET reasoning_effort='high'")
			}
			if err := runWorker(t, dir, gitBin, "migrate", "model-selection"); err != nil {
				t.Fatal(err)
			}
			assertLLMConfig(t, dir)
			withDB(t, filepath.Join(dir, "novel-agent.db"), func(db *sql.DB) {
				assertCount(t, db, "migrated default selection", `SELECT COUNT(*) FROM app_config WHERE id=1 AND default_adapter_name='openai_chat' AND default_provider_name=? AND default_model_id=? AND default_reasoning_effort=?`, 1, tt.provider, tt.model, effort)
				assertCount(t, db, "reset session selection", `SELECT COUNT(*) FROM sessions WHERE session_id='upgrade-session' AND title='历史对话' AND active_version=2 AND adapter_name='' AND provider_name='' AND model_id='' AND reasoning_effort=''`, 1)
				assertCount(t, db, "retained messages", `SELECT COUNT(*) FROM messages WHERE session_id='upgrade-session'`, 3)
				assertCount(t, db, "completed model selection migration", `SELECT COUNT(*) FROM migrate_state WHERE migration='v1.8.0-model-selection' AND status='done'`, 2)
			})
			before := dumpDatabase(t, filepath.Join(dir, "novel-agent.db"), true)
			if err := runWorker(t, dir, gitBin, "migrate", "model-selection-restart"); err != nil {
				t.Fatal(err)
			}
			if after := dumpDatabase(t, filepath.Join(dir, "novel-agent.db"), true); !reflect.DeepEqual(before, after) {
				t.Fatal("restart changed the migrated selections or migration state")
			}
		})
	}
}
