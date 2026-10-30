package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func saveFixture(source, manifestPath string, m *manifest, sample fixture, rebuild bool) error {
	if filepath.Base(sample.Archive) != sample.Archive || sample.Archive != sample.Tag+"-"+sample.Origin+".zip" {
		return fmt.Errorf("invalid fixture archive %q", sample.Archive)
	}
	replacing := -1
	for i, frozen := range m.Fixtures {
		if frozen.Archive == sample.Archive || (frozen.Tag == sample.Tag && frozen.Origin == sample.Origin) {
			if !rebuild {
				return fmt.Errorf("fixture %s is already frozen; refusing to overwrite", sample.Archive)
			}
			hash, err := fileHash(filepath.Join(filepath.Dir(manifestPath), frozen.Archive))
			if err != nil {
				return err
			}
			if frozen.Archive != sample.Archive || frozen.Commit != sample.Commit || hash != frozen.SHA256 {
				return fmt.Errorf("frozen fixture %s changed; refusing to rebuild", sample.Archive)
			}
			replacing = i
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
	next.Fixtures = append([]fixture(nil), m.Fixtures...)
	if replacing >= 0 {
		next.Fixtures[replacing] = sample
	} else {
		next.Fixtures = append(next.Fixtures, sample)
	}
	data, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return err
	}
	// The manifest is the commit point. After an interruption, an archive absent
	// from it is uncommitted and may be replaced by a newly generated sample.
	target := filepath.Join(dir, sample.Archive)
	previous := filepath.Join(staging, "previous.zip")
	if replacing >= 0 {
		if err := os.Link(target, previous); err != nil {
			return err
		}
	}
	if err := os.Rename(archive, target); err != nil {
		return err
	}
	if err := writeManifest(manifestPath, append(data, '\n')); err != nil {
		if replacing >= 0 {
			if rollbackErr := os.Rename(previous, target); rollbackErr != nil {
				return fmt.Errorf("publish manifest: %w; restore archive: %v", err, rollbackErr)
			}
		}
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
