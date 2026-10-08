package config

import (
	"path/filepath"
	"testing"

	"github.com/sigpanic/goink/internal/testsupport"
	"github.com/stretchr/testify/require"
)

func TestExpandTilde_WithHome(t *testing.T) {
	home := testsupport.RequireEnv(t, "HOME")
	result := expandTilde("~/data")
	if result != home+"/data" {
		t.Errorf("expected %s, got %s", home+"/data", result)
	}
}

func TestExpandTilde_NoTilde(t *testing.T) {
	result := expandTilde("/absolute/path")
	if result != "/absolute/path" {
		t.Errorf("expected /absolute/path, got %s", result)
	}
}

func TestExpandTilde_Empty(t *testing.T) {
	home := testsupport.RequireEnv(t, "HOME")
	result := expandTilde("")
	// 空字符串等价于 "~"，直接返回 home 目录
	if result != home {
		t.Errorf("expected %s, got %s", home, result)
	}
}

func TestExpandTilde_OnlyTilde(t *testing.T) {
	home := testsupport.RequireEnv(t, "HOME")
	result := expandTilde("~")
	if result != home {
		t.Errorf("expected %s, got %s", home, result)
	}
}

func TestMCPConfigPaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	testsupport.Isolate(t)
	require.Equal(t, filepath.Join(home, ".goink", "mcp_config.enc"), MCPConfigPath())
	require.Equal(t, filepath.Join(home, ".goink", "llm_config.enc"), LLMConfigPath())
}
