//go:build cgo && upgrade

package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/sigpanic/goink/internal/config"
	fixturedata "github.com/sigpanic/goink/internal/migrate/upgrade/fixture"
	"github.com/sigpanic/goink/internal/migrate/upgrade/fixture/seeddata"
)

func TestVersionedRunnerSeedAndUpgrade(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("historical fixture generation requires Linux")
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	runner, err := buildRunner(root, work, "v1.8.0")
	if err != nil {
		t.Fatal(err)
	}
	base, err := buildRunner(root, work, "v1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct{ origin, seedRunner, seedTag string }{
		{"fresh", runner, "v1.8.0"},
		{"from-v1.0.0", base, "v1.0.0"},
	} {
		t.Run(tt.origin, func(t *testing.T) {
			home := t.TempDir()
			if err := invoke(root, tt.seedRunner, home, "seed"); err != nil {
				t.Fatal(err)
			}
			assertGeneratedSeedInput(t, home, tt.seedTag)
			input := readFile(t, filepath.Join(home, "seed.json"))
			if err := invoke(root, runner, home, "upgrade"); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(input, readFile(t, filepath.Join(home, "seed.json"))) {
				t.Fatal("historical upgrade replaced the original seed input")
			}
			metadataPath := filepath.Join(home, "metadata.json")
			metadata := readFile(t, metadataPath)
			var sample fixture
			if err := json.Unmarshal(metadata, &sample); err != nil {
				t.Fatal(err)
			}
			sample.Tag, sample.Origin = "v1.8.0", tt.origin
			if err := fixturedata.ValidateSeed(sample); err != nil {
				t.Fatal(err)
			}
			configPath := filepath.Join(home, ".goink", "llm_config.enc")
			ciphertext := readFile(t, configPath)
			dbPath := filepath.Join(home, "Goink", "novel-agent.db")
			dsn := (&url.URL{Scheme: "file", OmitHost: true, Path: filepath.ToSlash(dbPath), RawQuery: "mode=ro"}).String()
			db, err := sql.Open("sqlite3", dsn)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			var integrity string
			if err := db.QueryRow("PRAGMA integrity_check").Scan(&integrity); err != nil || integrity != "ok" {
				t.Fatalf("historical database integrity: %q, error=%v", integrity, err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			if err := invoke(root, runner, home, "upgrade"); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(metadata, readFile(t, metadataPath)) || !bytes.Equal(ciphertext, readFile(t, configPath)) {
				t.Fatal("restart changed seed identity, Git heads, or encrypted config")
			}
			if _, err := os.Stat(filepath.Join(home, "Goink", "novel-agent.db-wal")); err == nil {
				t.Fatal("runner left a live WAL database")
			}
		})
	}
}

func assertGeneratedSeedInput(t *testing.T, home, tag string) {
	t.Helper()
	var sample fixture
	if err := json.Unmarshal(readFile(t, filepath.Join(home, "metadata.json")), &sample); err != nil {
		t.Fatal(err)
	}
	sample.Tag, sample.Origin = tag, "fresh"
	if err := fixturedata.ValidateSeed(sample); err != nil {
		t.Fatal(err)
	}
	input, err := fixturedata.BuildSeed(sample.CaseIDs)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := config.Decrypt(readFile(t, filepath.Join(home, ".goink", "llm_config.enc")))
	if err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(home, "Goink", "novel-agent.db")
	dsn := (&url.URL{Scheme: "file", OmitHost: true, Path: filepath.ToSlash(dbPath), RawQuery: "mode=ro"}).String()
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	observed := &seeddata.UpgradeResult{DB: db, DataDir: filepath.Join(home, "Goink"), LLMConfig: plain}
	if err := seeddata.AssertRows(observed, input.Rows); err != nil {
		t.Fatalf("fresh %s database differs from Before: %v", tag, err)
	}
	if err := seeddata.AssertFiles(observed, input.Files); err != nil {
		t.Fatalf("fresh %s files differ from Before: %v", tag, err)
	}
	if err := seeddata.AssertConfig(observed, input.LLMConfig); err != nil {
		t.Fatalf("fresh %s LLM config differs from Before: %v", tag, err)
	}
}
