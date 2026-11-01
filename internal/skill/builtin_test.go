package skill

import (
	"log/slog"
	"os"
	"testing"
)

func TestLoadBuiltinSkills(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	skills, err := LoadBuiltinSkills(logger)
	if err != nil {
		t.Fatalf("LoadBuiltinSkills failed: %v", err)
	}
	if len(skills) == 0 {
		t.Fatal("expected at least 1 builtin skill, got 0")
	}
	t.Logf("Loaded %d builtin skills:", len(skills))
	for _, s := range skills {
		t.Logf("  - %s (mode=%s)", s.Name, s.Mode)
	}
}

func TestLoadBuiltinSkillCreator(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	skills, err := LoadBuiltinSkills(logger)
	if err != nil {
		t.Fatalf("LoadBuiltinSkills failed: %v", err)
	}

	creator := findByName(skills, "skill-creator")
	if creator == nil {
		t.Fatal("expected skill-creator to be embedded and loaded")
	}
	if creator.Mode != ModeAuto {
		t.Errorf("skill-creator mode = %q, want %q for on-demand discovery", creator.Mode, ModeAuto)
	}
	if creator.Description == "" || creator.Category == "" || creator.Content == "" {
		t.Error("skill-creator must include discovery metadata and authoring instructions")
	}
}
