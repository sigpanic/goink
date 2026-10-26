package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	fixturedata "github.com/sigpanic/goink/internal/migrate/upgrade/fixture"
)

func TestSaveFixtureRecoversUnregisteredArchive(t *testing.T) {
	dir, source, path, m, sample := fixtureOutput(t)
	archive := filepath.Join(dir, sample.Archive)
	if err := os.WriteFile(archive, []byte("interrupted archive"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := saveFixture(source, path, &m, sample); err != nil {
		t.Fatal(err)
	}
	loaded, err := fixturedata.Load(dir)
	if err != nil || len(loaded.Fixtures) != 1 {
		t.Fatalf("recovered manifest: %+v, %v", loaded, err)
	}
	output := t.TempDir()
	if err := fixturedata.Extract(archive, output); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(output, "novel-agent.db")); string(got) != "synthetic data" {
		t.Fatalf("recovered archive has wrong content: %q", got)
	}
	frozenManifest, frozenArchive := readFile(t, path), readFile(t, archive)
	if err := saveFixture(source, path, &m, sample); err == nil {
		t.Fatal("overwrote a registered frozen fixture")
	}
	if !bytes.Equal(frozenManifest, readFile(t, path)) || !bytes.Equal(frozenArchive, readFile(t, archive)) {
		t.Fatal("refused overwrite modified the frozen fixture")
	}
}

func TestSaveFixtureRecoversManifestPublicationFailure(t *testing.T) {
	dir, source, path, m, sample := fixtureOutput(t)
	before := readFile(t, path)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0755); err != nil {
		t.Fatal(err)
	}
	if err := saveFixture(source, path, &m, sample); err == nil {
		t.Fatal("expected manifest publication to fail")
	}
	if len(m.Fixtures) != 0 {
		t.Fatal("failed publication changed the committed manifest")
	}
	if _, err := os.Stat(filepath.Join(dir, sample.Archive)); err != nil {
		t.Fatal("expected unregistered archive at the interruption point:", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := writeManifest(path, before); err != nil {
		t.Fatal(err)
	}
	if err := saveFixture(source, path, &m, sample); err != nil {
		t.Fatal("retry after publication failure:", err)
	}
	if _, err := fixturedata.Load(dir); err != nil {
		t.Fatal(err)
	}
}

func TestSaveFixtureLeavesManifestIntactOnArchiveFailure(t *testing.T) {
	dir, source, path, m, sample := fixtureOutput(t)
	before := readFile(t, path)
	if err := os.WriteFile(filepath.Join(source, "novel-agent.db-wal"), []byte("uncheckpointed"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := saveFixture(source, path, &m, sample); err == nil {
		t.Fatal("expected archive creation to fail with a live WAL")
	}
	if !bytes.Equal(before, readFile(t, path)) || len(m.Fixtures) != 0 {
		t.Fatal("archive failure changed the existing manifest")
	}
	if _, err := os.Stat(filepath.Join(dir, sample.Archive)); !os.IsNotExist(err) {
		t.Fatalf("incomplete archive was published: %v", err)
	}
}

func fixtureOutput(t *testing.T) (string, string, string, manifest, fixture) {
	t.Helper()
	dir, source := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "novel-agent.db"), []byte("synthetic data"), 0644); err != nil {
		t.Fatal(err)
	}
	m := manifest{Format: 1, Generator: 1}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "manifest.json")
	if err := writeManifest(path, data); err != nil {
		t.Fatal(err)
	}
	sample := fixture{
		Tag: "v1.0.0", Origin: "fresh", Archive: "v1.0.0-fresh.zip",
		Commit:   strings.Repeat("a", 40),
		GitHeads: map[string]string{"1": strings.Repeat("b", 40), "2": strings.Repeat("c", 40)},
	}
	return dir, source, path, m, sample
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
