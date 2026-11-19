//go:build cgo && upgrade

package upgrade_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sigpanic/goink/internal/config"
	"github.com/sigpanic/goink/internal/migrate/upgrade/fixture"
	"github.com/sigpanic/goink/internal/migrate/upgrade/fixture/seeddata"
)

// Faults affect only isolated migrated copies, never production migration code.
func TestCaseAssertionsDetectCorruption(t *testing.T) {
	samples := historicalFixtures(t)
	gitBin := requireGit(t)
	for _, tt := range []struct {
		name, archive, caseID, query string
	}{
		{"builtin provider lost", "v1.7.0-from-v1.0.0.zip", "settings-legacy", "UPDATE app_config SET default_provider_name=''"},
		{"relay model slash lost", "v1.7.0-fresh.zip", "settings-legacy-relay", "UPDATE app_config SET default_model_id='model'"},
		{"native selection reset", "v1.8.0-fresh.zip", "settings-native", "UPDATE app_config SET default_model_id=''"},
		{"legacy session not reset", "v1.7.0-from-v1.0.0.zip", "sessions-legacy", "UPDATE sessions SET model_id='vendor/model'"},
		{"native session lost", "v1.8.0-fresh.zip", "sessions-native", "UPDATE sessions SET model_id=''"},
		{"message lost", "v1.7.0-from-v1.0.0.zip", "messages", "DELETE FROM messages WHERE id=3"},
		{"writing count lost", "v1.7.0-fresh.zip", "writing-daily", "UPDATE writing_log SET words_deleted=50"},
		{"cross novel reference", "v1.7.0-from-v1.0.0.zip", "references-v100", "UPDATE time_entries SET source_chapter_id=21 WHERE id=1"},
		{"volume association lost", "v1.7.0-fresh.zip", "chapters-id", "UPDATE chapters SET volume_id=NULL WHERE id=12"},
		{"group member lost", "v1.7.0-fresh.zip", "groups", "DELETE FROM character_group_members WHERE id=2"},
		{"unexpected message", "v1.7.0-fresh.zip", "messages", "INSERT INTO messages (id,session_id,role,content) VALUES (99,'upgrade-session','user','unexpected')"},
		{"config changed", "v1.7.0-fresh.zip", "llm-v1", ""},
		{"chapter file changed", "v1.7.0-fresh.zip", "chapters-id", ""},
	} {
		var sample fixture.Fixture
		for _, candidate := range samples {
			if candidate.Archive == tt.archive {
				sample = candidate
				break
			}
		}
		// A release-target run excludes its own fresh archive from historical inputs.
		if sample.Archive == "" {
			continue
		}
		t.Run(tt.name, func(t *testing.T) {
			dir := prepare(t, sample)
			if err := runWorker(t, dir, gitBin, "migrate", "assertion-baseline"); err != nil {
				t.Fatal(err)
			}
			cases, err := fixture.ResolveCases(sample.CaseIDs)
			if err != nil {
				t.Fatal(err)
			}
			var selected seeddata.SeedCase
			for _, c := range cases {
				if c.ID == tt.caseID {
					selected = c
					break
				}
			}
			if selected.AssertAfter == nil {
				t.Fatalf("fixture %s does not exercise seed case %s", sample.Archive, tt.caseID)
			}
			ciphertext, err := os.ReadFile(filepath.Join(dir, ".goink", "llm_config.enc"))
			if err != nil {
				t.Fatal(err)
			}
			plain, err := config.Decrypt(ciphertext)
			if err != nil {
				t.Fatal(err)
			}
			withDB(t, filepath.Join(dir, "novel-agent.db"), func(db *sql.DB) {
				result := &seeddata.UpgradeResult{DB: db, DataDir: dir, LLMConfig: plain}
				if err := selected.AssertAfter(result); err != nil {
					t.Fatalf("valid upgraded input rejected: %v", err)
				}
				switch {
				case tt.query != "":
					mutate(t, dir, tt.query)
				case tt.caseID == "llm-v1":
					result.LLMConfig = []byte(strings.ReplaceAll(string(plain), "fixture-relay-key", "lost-key"))
				default:
					if err := os.WriteFile(filepath.Join(dir, "novels", "1", "chapters", "id_11.md"), []byte("lost chapter"), 0644); err != nil {
						t.Fatal(err)
					}
				}
				if err := selected.AssertAfter(result); err == nil {
					t.Fatal("corrupted upgrade result passed its seed case assertion")
				}
			})
		})
	}
}
