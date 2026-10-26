package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func saveFixture(source, manifestPath string, m *manifest, sample fixture) error {
	if filepath.Base(sample.Archive) != sample.Archive || sample.Archive != sample.Tag+"-"+sample.Origin+".zip" {
		return fmt.Errorf("invalid fixture archive %q", sample.Archive)
	}
	for _, frozen := range m.Fixtures {
		if frozen.Archive == sample.Archive || (frozen.Tag == sample.Tag && frozen.Origin == sample.Origin) {
			return fmt.Errorf("fixture %s is already frozen; refusing to overwrite", sample.Archive)
		}
	}
	dir := filepath.Dir(manifestPath)
	staging, err := os.MkdirTemp(dir, ".fixture-pending-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	archive := filepath.Join(staging, sample.Archive)
	if err := zipData(source, archive); err != nil {
		return err
	}
	sample.SHA256, err = fileHash(archive)
	if err != nil {
		return err
	}
	next := *m
	next.Fixtures = append(append([]fixture(nil), m.Fixtures...), sample)
	data, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return err
	}
	// The manifest is the commit point. After an interruption, an archive absent
	// from it is uncommitted and may be replaced by a newly generated sample.
	if err := os.Rename(archive, filepath.Join(dir, sample.Archive)); err != nil {
		return err
	}
	if err := writeManifest(manifestPath, append(data, '\n')); err != nil {
		return err
	}
	*m = next
	return nil
}

func writeManifest(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".manifest-pending-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		return err
	}
	if err := f.Chmod(0644); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
