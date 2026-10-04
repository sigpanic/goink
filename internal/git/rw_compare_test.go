package git

import (
	"errors"
	"testing"

	"github.com/sigpanic/goink/internal/config"
)

func TestWriteFileIfUnchanged(t *testing.T) {
	config.Set(&config.AppConfig{DataDir: t.TempDir()})
	const path = "goink.md"

	if err := WriteFileIfUnchanged(1, path, "", "first"); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileIfUnchanged(1, path, "", "stale"); !errors.Is(err, ErrFileChanged) {
		t.Fatalf("stale write error = %v, want ErrFileChanged", err)
	}
	if err := WriteFileIfUnchanged(1, path, "first", "second"); err != nil {
		t.Fatal(err)
	}
	got, err := ReadFile(1, path)
	if err != nil || got != "second" {
		t.Fatalf("content = %q, err = %v, want second", got, err)
	}
}
