package version

import (
	"os/exec"
	"path/filepath"
	"testing"
)

func TestBuildHashWithoutVCS(t *testing.T) {
	for _, hash := range []string{"", "01234567", "01234567-dirty"} {
		name := hash
		if name == "" {
			name = "missing"
		}
		t.Run(name, func(t *testing.T) {
			binary := filepath.Join(t.TempDir(), "buildhash.exe")
			args := []string{"build", "-buildvcs=false", "-o", binary}
			if hash != "" {
				args = append(args, "-ldflags", "-X github.com/sigpanic/goink/internal/version.CommitHash="+hash)
			}
			args = append(args, "./internal/version/testdata/buildhash")
			cmd := exec.Command("go", args...)
			cmd.Dir = filepath.Join("..", "..")
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("build probe: %v\n%s", err, output)
			}
			output, err := exec.Command(binary).CombinedOutput()
			if err != nil {
				t.Fatalf("run probe: %v\n%s", err, output)
			}
			if string(output) != hash {
				t.Fatalf("BuildHash() = %q, want %q", output, hash)
			}
		})
	}
}
