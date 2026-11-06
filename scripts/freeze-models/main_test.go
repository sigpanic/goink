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

func TestResolveSource(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	head, err := git(root, "rev-parse", "HEAD^{commit}")
	if err != nil {
		t.Fatal(err)
	}
	type sourceTest struct{ ref, source, commit string }
	tests := []sourceTest{
		{"HEAD", head, head},
		{"HEAD^{commit}", head, head},
		{head, head, head},
		{head[:8], head[:8], head},
	}
	if parent, err := git(root, "rev-parse", "--verify", "HEAD~1^{commit}"); err == nil {
		tests = append(tests, sourceTest{"HEAD~1", parent, parent})
	}
	if tagCommit, err := git(root, "rev-parse", "--verify", "refs/tags/v1.6.0^{commit}"); err == nil {
		tests = append(tests,
			sourceTest{"v1.6.0", "v1.6.0", tagCommit},
			sourceTest{"refs/tags/v1.6.0", "refs/tags/v1.6.0", tagCommit},
			sourceTest{"v1.6.0^{commit}", tagCommit, tagCommit},
		)
	}
	branch, err := git(root, "rev-parse", "--symbolic-full-name", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(branch, "refs/heads/") {
		tests = append(tests,
			sourceTest{branch, head, head},
			sourceTest{strings.TrimPrefix(branch, "refs/heads/"), head, head},
		)
	}
	for _, test := range tests {
		t.Run(test.ref, func(t *testing.T) {
			source, commit, err := resolveSource(root, test.ref)
			if err != nil {
				t.Fatal(err)
			}
			if source != test.source || commit != test.commit {
				t.Fatalf("resolveSource(%q) = (%q, %q), want (%q, %q)", test.ref, source, commit, test.source, test.commit)
			}
		})
	}
	t.Run("invalid revision", func(t *testing.T) {
		if _, _, err := resolveSource(root, "refs/heads/freeze-models-missing-ref"); err == nil {
			t.Fatal("expected an error for a missing revision")
		}
	})
}

func TestFreezeRevisionSnapshotCheck(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	parent, err := git(root, "rev-parse", "--verify", "HEAD~1^{commit}")
	if err != nil {
		t.Skipf("snapshot regression requires the parent commit: %v", err)
	}
	dir, err := os.MkdirTemp(root, ".freeze-models-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	out := filepath.Join(dir, "snapshot", "models.gen.go")
	if err := run("HEAD~1", out, false); err != nil {
		t.Fatal(err)
	}
	snapshot, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var source string
	for _, line := range strings.Split(string(snapshot), "\n") {
		if strings.HasPrefix(line, "// Source: ") {
			source = strings.TrimPrefix(line, "// Source: ")
			break
		}
	}
	if source != parent {
		t.Fatalf("snapshot Source = %q, want pinned commit %q", source, parent)
	}
	if err := run(source, out, true); err != nil {
		t.Fatalf("checking a snapshot from an earlier commit: %v", err)
	}
	after, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(snapshot) {
		t.Fatal("check modified the frozen snapshot")
	}
}
