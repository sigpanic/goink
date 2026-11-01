//go:build cgo && upgrade

package upgrade_test

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHistoricalWritingUpgradeWithoutBackup(t *testing.T) {
	gitBin := requireGit(t)
	for _, sample := range historicalFixtures(t) {
		if sample.Tag != "v1.6.0" {
			continue
		}
		t.Run(sample.Origin, func(t *testing.T) {
			dir := prepare(t, sample)
			blocker := filepath.Join(dir, "backups")
			if err := os.WriteFile(blocker, []byte("backup unavailable"), 0644); err != nil {
				t.Fatal(err)
			}
			if err := runWorker(t, dir, gitBin, "migrate", "writing-without-backup"); err != nil {
				t.Fatal(err)
			}
			assertContract(t, dir, sample, gitBin)
			if data, err := os.ReadFile(blocker); err != nil || string(data) != "backup unavailable" {
				t.Fatalf("writing aggregation changed the backup path: data=%q error=%v", data, err)
			}
		})
	}
}
