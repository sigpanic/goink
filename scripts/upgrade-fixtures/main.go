// Generate frozen upgrade inputs from released source, without checking out or
// changing the project repository. Run from the repository root:
// go run ./scripts/upgrade-fixtures -tags v1.0.0,v1.1.0
// go run ./scripts/upgrade-fixtures -check -target v1.8.0
package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	fixturedata "github.com/sigpanic/goink/internal/migrate/upgrade/fixture"
)

//go:embed runner.go.tmpl
var templates embed.FS

const fixtureDir = "internal/migrate/upgrade/testdata"

type fixture = fixturedata.Fixture
type manifest = fixturedata.Manifest

func main() {
	tags := flag.String("tags", "", "comma-separated release tags; empty selects all stable releases since v1.0.0")
	check := flag.Bool("check", false, "check archive hashes and coverage of historical release tags")
	target := flag.String("target", "", "candidate tag excluded from historical coverage")
	rebuild := flag.Bool("rebuild", false, "explicitly regenerate frozen fixtures after verifying their existing checksums and source commits")
	flag.Parse()
	if err := run(*tags, *check, *target, *rebuild); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(selected string, check bool, target string, rebuild bool) error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	gitRoot, err := git(root, "rev-parse", "--show-toplevel")
	if err != nil {
		return err
	}
	if filepath.Clean(gitRoot) != filepath.Clean(root) {
		return fmt.Errorf("run from the worktree root")
	}
	path := filepath.Join(root, fixtureDir, "manifest.json")
	m := manifest{Format: 1, Generator: 1}
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &m); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if m.Format != 1 || m.Generator != 1 {
		return fmt.Errorf("unsupported fixture format/generator")
	}
	all, err := releaseTags(root, target)
	if err != nil {
		return err
	}
	if check {
		return checkManifest(root, m, all)
	}
	if runtime.GOOS != "linux" {
		return fmt.Errorf("generate historical fixtures on Linux; consumption is cross-platform")
	}
	if selected != "" {
		all = strings.Split(selected, ",")
	}
	if len(all) == 0 {
		return fmt.Errorf("no release tags selected")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	work, err := os.MkdirTemp(root, ".upgrade-fixtures-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	baseRunner, err := buildRunner(root, work, "v1.0.0")
	if err != nil {
		return err
	}
	for _, tag := range all {
		if !fixturedata.SupportedTag(tag) {
			return fmt.Errorf("invalid supported release tag %q", tag)
		}
		commit, err := git(root, "rev-parse", tag+"^{commit}")
		if err != nil {
			return err
		}
		runner := baseRunner
		if tag != "v1.0.0" {
			runner, err = buildRunner(root, work, tag)
			if err != nil {
				return err
			}
		}
		for _, origin := range []string{"fresh", "from-v1.0.0"} {
			if tag == "v1.0.0" && origin != "fresh" {
				continue
			}
			name := tag + "-" + origin
			frozen := false
			for _, existing := range m.Fixtures {
				if existing.Archive == name+".zip" {
					hash, err := fileHash(filepath.Join(root, fixtureDir, existing.Archive))
					if err != nil {
						return err
					}
					if existing.Commit != commit || hash != existing.SHA256 {
						return fmt.Errorf("frozen fixture %s changed; refusing to overwrite", name)
					}
					frozen = true
				}
			}
			if frozen && !rebuild {
				continue
			}
			fmt.Println("Generate", name, commit)
			home := filepath.Join(work, name)
			seedRunner := runner
			if origin != "fresh" {
				seedRunner = baseRunner
			}
			if err := invoke(root, seedRunner, home, "seed"); err != nil {
				return fmt.Errorf("seed %s: %w", name, err)
			}
			if origin != "fresh" {
				if err := invoke(root, runner, home, "upgrade"); err != nil {
					return fmt.Errorf("historical upgrade %s: %w", name, err)
				}
			}
			var f fixture
			meta, err := os.ReadFile(filepath.Join(home, "metadata.json"))
			if err != nil {
				return err
			}
			if err := json.Unmarshal(meta, &f); err != nil {
				return err
			}
			f.Tag, f.Commit, f.Origin, f.Archive = tag, commit, origin, name+".zip"
			// Persist each completed sample, so an interrupted generation can be
			// reviewed and resumed without overwriting already frozen archives.
			if err := saveFixture(filepath.Join(home, "Goink"), path, &m, f, rebuild); err != nil {
				return err
			}
		}
	}
	return nil
}

func git(root string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %v: %w: %s", args, err, out)
	}
	return strings.TrimSpace(string(out)), nil
}

func releaseTags(root, target string) ([]string, error) {
	out, err := git(root, "tag", "--list", "v*", "--sort=version:refname")
	if err != nil {
		return nil, err
	}
	var tags []string
	for _, tag := range strings.Fields(out) {
		if fixturedata.BeforeTarget(tag, target) {
			tags = append(tags, tag)
		}
	}
	return tags, nil
}

func checkManifest(root string, m manifest, tags []string) error {
	if _, err := fixturedata.Load(filepath.Join(root, fixtureDir)); err != nil {
		return err
	}
	seen := make(map[string]bool)
	for _, f := range m.Fixtures {
		key := f.Tag + "/" + f.Origin
		if seen[key] || !fixturedata.SupportedTag(f.Tag) || filepath.Base(f.Archive) != f.Archive {
			return fmt.Errorf("invalid/duplicate fixture %s", key)
		}
		seen[key] = true
		commit, err := git(root, "rev-parse", f.Tag+"^{commit}")
		if err != nil {
			return err
		}
		if commit != f.Commit {
			return fmt.Errorf("tag %s no longer matches frozen commit %s", f.Tag, f.Commit)
		}
	}
	for _, tag := range tags {
		for _, origin := range []string{"fresh", "from-v1.0.0"} {
			if tag == "v1.0.0" && origin != "fresh" {
				continue
			}
			if !seen[tag+"/"+origin] {
				return fmt.Errorf("missing historical fixture %s/%s; generate it before the next release", tag, origin)
			}
		}
	}
	if len(tags) == 0 || len(m.Fixtures) == 0 {
		return fmt.Errorf("historical coverage is empty; checkout must include release tags")
	}
	fmt.Printf("Verified %d frozen samples and %d historical releases\n", len(m.Fixtures), len(tags))
	return nil
}

func buildRunner(root, work, tag string) (string, error) {
	dir := filepath.Join(work, "source-"+tag)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	cmd := exec.Command("git", "archive", "--format=tar", tag)
	cmd.Dir = root
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return "", err
	}
	extractErr := extractSource(pipe, dir)
	if extractErr != nil {
		_ = cmd.Process.Kill()
	}
	if err := cmd.Wait(); err != nil {
		return "", fmt.Errorf("archive %s: %w: %s", tag, err, stderr.String())
	}
	if extractErr != nil {
		return "", extractErr
	}
	mod, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return "", err
	}
	fields := strings.Fields(string(mod))
	if len(fields) < 2 || fields[0] != "module" {
		return "", fmt.Errorf("invalid historical go.mod")
	}
	source, err := templates.ReadFile("runner.go.tmpl")
	if err != nil {
		return "", err
	}
	source = bytes.ReplaceAll(source, []byte("{{.}}"), []byte(fields[1]))
	if err := os.WriteFile(filepath.Join(dir, "upgrade_fixture_main.go"), source, 0644); err != nil {
		return "", err
	}
	runner := filepath.Join(work, "runner-"+tag)
	build := exec.Command("go", "build", "-o", runner, "upgrade_fixture_main.go")
	build.Dir, build.Stdout, build.Stderr = dir, os.Stdout, os.Stderr
	if err := build.Run(); err != nil {
		return "", fmt.Errorf("build historical runner %s: %w", tag, err)
	}
	return runner, nil
}

func extractSource(reader io.Reader, dir string) error {
	tr := tar.NewReader(reader)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if h.Name != "go.mod" && h.Name != "go.sum" && !strings.HasPrefix(h.Name, "internal/") && !strings.HasPrefix(h.Name, "third_party/") {
			continue
		}
		name := filepath.Clean(filepath.FromSlash(h.Name))
		if filepath.IsAbs(name) || name == ".." || strings.HasPrefix(name, ".."+string(filepath.Separator)) {
			return fmt.Errorf("invalid archive path %s", h.Name)
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_EXCL, os.FileMode(h.Mode))
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(f, tr)
		closeErr := f.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
}

func invoke(root, runner, home, mode string) error {
	if err := os.MkdirAll(home, 0755); err != nil {
		return err
	}
	gitBin, err := exec.LookPath("git")
	if err != nil {
		return err
	}
	cmd := exec.Command(runner, mode, filepath.Join(home, "metadata.json"))
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "HOME="+home, "GOINK_DATA_DIR="+filepath.Join(home, "Goink"), "GOINK_TESTING=1", "GOINK_GIT_BIN="+gitBin,
		"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_AUTHOR_DATE=2026-01-01T00:00:00Z", "GIT_COMMITTER_DATE=2026-01-01T00:00:00Z")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}

func zipData(dir, target string) error {
	f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	complete := false
	defer func() {
		if !complete {
			_ = os.Remove(target)
		}
	}()
	zw := zip.NewWriter(f)
	walk := func(root, prefix string) error {
		return filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			name := filepath.ToSlash(filepath.Join(prefix, rel))
			if entry.IsDir() {
				if name == "backups" || strings.HasSuffix(name, "/.git/hooks") {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasSuffix(name, "-wal") || strings.HasSuffix(name, "-shm") {
				return fmt.Errorf("database must be closed before freezing: %s", name)
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("unsupported fixture file %s", name)
			}
			header, err := zip.FileInfoHeader(info)
			if err != nil {
				return err
			}
			header.Name, header.Method = name, zip.Deflate
			header.Modified = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			writer, err := zw.CreateHeader(header)
			if err != nil {
				return err
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			_, err = writer.Write(data)
			return err
		})
	}
	err = walk(dir, "")
	if err == nil {
		err = walk(filepath.Join(filepath.Dir(dir), ".goink"), ".goink")
	}
	closeZipErr, closeFileErr := zw.Close(), f.Close()
	if err != nil {
		return err
	}
	if closeZipErr != nil {
		return closeZipErr
	}
	if closeFileErr != nil {
		return closeFileErr
	}
	complete = true
	return nil
}

func fileHash(path string) (string, error) {
	return fixturedata.Hash(path)
}
