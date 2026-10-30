package main

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFreezeSharedCrossPackageFields(t *testing.T) {
	root := t.TempDir()
	for path, source := range map[string]string{
		"internal/adapter/types.go": `package adapter
type Protocol string
type Selection struct { Protocol Protocol; Effort string }`,
		"internal/settings/types.go": `package settings
import "example.test/app/internal/adapter"
type Settings struct {
 Default adapter.Selection ` + "`gorm:\"embedded;embeddedPrefix:default_\"`" + `
 Current adapter.Selection
}
func (Settings) TableName() string { return "settings" }`,
	} {
		target := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte(source), 0644); err != nil {
			t.Fatal(err)
		}
	}
	f := &freezer{
		root: root, module: "example.test/app", fset: token.NewFileSet(),
		packages: map[string]map[string]*sourceType{}, emitted: map[model]bool{}, imports: map[string]string{},
	}
	m := model{"example.test/app/internal/settings", "Settings"}
	if err := f.emit(m); err != nil {
		t.Fatal(err)
	}
	source, err := f.render("snapshot", "test", "test", "test", []model{m})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "models.go", source, 0); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"type FrozenAdapterSelection struct",
		"Protocol FrozenAdapterProtocol",
		"Default FrozenAdapterSelection `gorm:\"embedded;embeddedPrefix:default_\"`",
		"Current FrozenAdapterSelection",
	} {
		if !strings.Contains(string(source), expected) {
			t.Fatalf("snapshot missing %q:\n%s", expected, source)
		}
	}
	if strings.Count(string(source), "type FrozenAdapterSelection struct") != 1 || strings.Contains(string(source), "example.test/app/internal/adapter") {
		t.Fatalf("shared fields must reference one frozen type without a business import:\n%s", source)
	}
}
