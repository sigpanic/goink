package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sigpanic/goink/internal/apperr"
	"github.com/sigpanic/goink/internal/config"
	"github.com/sigpanic/goink/internal/skill"
)

func moveSkillTestContent(name, description string) string {
	return "---\nname: " + name + "\ndescription: " + description + "\nmode: auto\n---\n\nBody\n"
}

func writeMoveSkillTestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, name+".md"), []byte(content), 0o644))
}

// setupMoveSkillTestApp 隔离 HOME，确保 config.UserSkillsDir() 落在测试临时目录。
func setupMoveSkillTestApp(t *testing.T) *App {
	t.Helper()
	a := setupTestApp(t)
	home := config.DataDirPath()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return a
}

func TestMoveSkill_NovelToUser(t *testing.T) {
	a := setupMoveSkillTestApp(t)
	n := createTestNovel(t, a)
	writeMoveSkillTestFile(t, config.NovelSkillsDir(n.ID), "demo", moveSkillTestContent("demo", "novel desc"))

	res := a.MoveSkill(MoveSkillInput{NovelID: n.ID, Name: "demo", From: "novel", To: "user"})
	require.Equal(t, apperr.CodeOK, res.ErrCode)
	require.Empty(t, res.ErrMsg)
	require.Equal(t, "demo", res.Data.Name)
	require.Equal(t, "user", res.Data.Source)

	require.NoFileExists(t, filepath.Join(config.NovelSkillsDir(n.ID), "demo.md"))
	require.FileExists(t, filepath.Join(config.UserSkillsDir(), "demo.md"))

	sk, ok := a.skill.Get(n.ID, "demo")
	require.True(t, ok)
	require.Equal(t, "Body", sk.Content)
}

func TestMoveSkill_UserToNovel(t *testing.T) {
	a := setupMoveSkillTestApp(t)
	n := createTestNovel(t, a)
	writeMoveSkillTestFile(t, config.UserSkillsDir(), "demo", moveSkillTestContent("demo", "user desc"))

	res := a.MoveSkill(MoveSkillInput{NovelID: n.ID, Name: "demo", From: "user", To: "novel"})
	require.Equal(t, apperr.CodeOK, res.ErrCode)
	require.Equal(t, "novel", res.Data.Source)

	require.NoFileExists(t, filepath.Join(config.UserSkillsDir(), "demo.md"))
	require.FileExists(t, filepath.Join(config.NovelSkillsDir(n.ID), "demo.md"))
}

func TestMoveSkill_TargetExists(t *testing.T) {
	a := setupMoveSkillTestApp(t)
	n := createTestNovel(t, a)
	writeMoveSkillTestFile(t, config.NovelSkillsDir(n.ID), "demo", moveSkillTestContent("demo", "novel desc"))
	writeMoveSkillTestFile(t, config.UserSkillsDir(), "demo", moveSkillTestContent("demo", "user desc"))

	res := a.MoveSkill(MoveSkillInput{NovelID: n.ID, Name: "demo", From: "novel", To: "user"})
	require.Equal(t, skill.CodeExists, res.ErrCode)

	userData, err := os.ReadFile(filepath.Join(config.UserSkillsDir(), "demo.md"))
	require.NoError(t, err)
	require.Equal(t, moveSkillTestContent("demo", "user desc"), string(userData))
	require.FileExists(t, filepath.Join(config.NovelSkillsDir(n.ID), "demo.md"))
}

func TestMoveSkill_SourceMissing(t *testing.T) {
	a := setupMoveSkillTestApp(t)
	n := createTestNovel(t, a)

	res := a.MoveSkill(MoveSkillInput{NovelID: n.ID, Name: "demo", From: "user", To: "novel"})
	require.Equal(t, skill.CodeNotFound, res.ErrCode)
}

func TestMoveSkill_InvalidName(t *testing.T) {
	a := setupMoveSkillTestApp(t)
	n := createTestNovel(t, a)

	res := a.MoveSkill(MoveSkillInput{NovelID: n.ID, Name: "a/b", From: "user", To: "novel"})
	require.Equal(t, skill.CodeInvalidName, res.ErrCode)
}

func TestMoveSkill_InvalidScope(t *testing.T) {
	a := setupMoveSkillTestApp(t)
	n := createTestNovel(t, a)

	cases := []MoveSkillInput{
		{NovelID: n.ID, Name: "demo", From: "builtin", To: "user"},
		{NovelID: n.ID, Name: "demo", From: "user", To: "builtin"},
		{NovelID: n.ID, Name: "demo", From: "user", To: "user"},
		{NovelID: 0, Name: "demo", From: "novel", To: "user"},
		{NovelID: 0, Name: "demo", From: "user", To: "novel"},
	}
	for _, in := range cases {
		res := a.MoveSkill(in)
		require.Equal(t, skill.CodeInvalidScope, res.ErrCode, "%+v", in)
	}
}

func TestMoveSkill_NameMismatch(t *testing.T) {
	a := setupMoveSkillTestApp(t)
	n := createTestNovel(t, a)
	writeMoveSkillTestFile(t, config.UserSkillsDir(), "demo", moveSkillTestContent("other", "desc"))

	res := a.MoveSkill(MoveSkillInput{NovelID: n.ID, Name: "demo", From: "user", To: "novel"})
	require.Equal(t, skill.CodeInvalidContent, res.ErrCode)
	require.FileExists(t, filepath.Join(config.UserSkillsDir(), "demo.md"))
	require.NoFileExists(t, filepath.Join(config.NovelSkillsDir(n.ID), "demo.md"))
}

func TestMoveSkill_InvalidContent(t *testing.T) {
	a := setupMoveSkillTestApp(t)
	n := createTestNovel(t, a)
	writeMoveSkillTestFile(t, config.UserSkillsDir(), "demo", "not a skill")

	res := a.MoveSkill(MoveSkillInput{NovelID: n.ID, Name: "demo", From: "user", To: "novel"})
	require.Equal(t, skill.CodeInvalidContent, res.ErrCode)
}
