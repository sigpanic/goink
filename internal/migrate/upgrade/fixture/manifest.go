// Package fixture defines the frozen input format shared by the historical
// generator and the upgrade tests. It has no dependency on application models.
package fixture

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type Fixture struct {
	Tag            string            `json:"tag"`
	Commit         string            `json:"commit"`
	Origin         string            `json:"origin"`
	Archive        string            `json:"archive"`
	SHA256         string            `json:"sha256"`
	WritingAdded   int               `json:"writing_added"`
	WritingDeleted int               `json:"writing_deleted"`
	LegacyChapters bool              `json:"legacy_chapters"`
	GitHeads       map[string]string `json:"git_heads"`
}

type Manifest struct {
	Format    int       `json:"format"`
	Generator int       `json:"generator"`
	Fixtures  []Fixture `json:"fixtures"`
}

func Load(dir string) (*Manifest, error) {
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	if m.Format != 1 || m.Generator != 1 || len(m.Fixtures) == 0 {
		return nil, fmt.Errorf("unsupported or empty historical manifest")
	}
	seen := map[string]bool{}
	for _, f := range m.Fixtures {
		if seen[f.Archive] || filepath.Base(f.Archive) != f.Archive || !strings.HasSuffix(f.Archive, ".zip") ||
			!SupportedTag(f.Tag) ||
			(f.Origin != "fresh" && f.Origin != "from-v1.0.0") ||
			!regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(f.Commit) ||
			f.WritingAdded < 0 || f.WritingDeleted < 0 {
			return nil, fmt.Errorf("invalid fixture metadata: %s", f.Archive)
		}
		seen[f.Archive] = true
		for _, novel := range []string{"1", "2"} {
			if !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(f.GitHeads[novel]) {
				return nil, fmt.Errorf("missing historical Git head: %s/%s", f.Archive, novel)
			}
		}
		hash, err := Hash(filepath.Join(dir, f.Archive))
		if err != nil {
			return nil, err
		}
		if hash != f.SHA256 {
			return nil, fmt.Errorf("fixture checksum mismatch: %s", f.Archive)
		}
	}
	return &m, nil
}

func Hash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func Extract(archive, dir string) error {
	zr, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer zr.Close()
	for _, entry := range zr.File {
		name := filepath.Clean(filepath.FromSlash(entry.Name))
		if filepath.IsAbs(name) || name == "." || name == ".." || strings.HasPrefix(name, ".."+string(filepath.Separator)) || strings.Contains(name, ":") || !entry.Mode().IsRegular() {
			return fmt.Errorf("invalid fixture archive entry %s", entry.Name)
		}
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		if err := extractFile(entry, path); err != nil {
			return err
		}
	}
	return nil
}

func extractFile(entry *zip.File, path string) error {
	reader, err := entry.Open()
	if err != nil {
		return err
	}
	defer reader.Close()
	writer, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, entry.Mode().Perm())
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(writer, reader)
	closeErr := writer.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}
