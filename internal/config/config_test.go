package config

import (
	"testing"

	"github.com/sigpanic/goink/internal/testsupport"
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
