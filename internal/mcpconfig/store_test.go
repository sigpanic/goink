package mcpconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/sigpanic/goink/internal/config"
	"github.com/stretchr/testify/require"
)

func TestStoreEncryptedRoundTripAndCorruption(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".goink", "mcp_config.enc")
	store := NewStore(path)
	empty, err := store.Load()
	require.NoError(t, err)
	require.False(t, empty.Local.Enabled)
	require.NoFileExists(t, path, "读取默认设置不能创建配置文件")
	state := testState()
	require.NoError(t, store.Save(state))
	loaded, err := NewStore(path).Load()
	require.NoError(t, err)
	require.Equal(t, state, loaded)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	for _, secret := range []string{"env-secret", "http-secret", "local-secret"} {
		require.NotContains(t, string(data), secret)
	}
	plain, err := config.Decrypt(data)
	require.NoError(t, err, "MCP 必须使用共用加密格式和固定密钥")
	var decoded State
	require.NoError(t, json.Unmarshal(plain, &decoded))
	require.Equal(t, state, decoded)
	entries, err := os.ReadDir(filepath.Dir(path))
	require.NoError(t, err)
	require.Len(t, entries, 1, "新存储不能产生独立密钥或残留临时文件")
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0600), info.Mode().Perm())
		info, err = os.Stat(filepath.Dir(path))
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0700), info.Mode().Perm())
	}
	require.NoError(t, store.Save(state))
	next, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NotEqual(t, data, next, "重复保存应使用新 nonce")
	data[len(data)-1] ^= 1
	require.NoError(t, os.WriteFile(path, data, 0600))
	_, err = store.Load()
	require.Error(t, err)
	remaining, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, data, remaining)
}

func testState() State {
	return State{
		Connections: []Connection{{ID: "stable-id", Name: "server", Enabled: true, AllowedTools: map[string]bool{"echo": true}}},
		Credentials: map[string]Secrets{"stable-id": {Env: map[string]string{"KEY": "env-secret"}, BearerToken: "http-secret"}},
		Local:       Local{Enabled: true, AllowedTools: []string{"get_chapter_list"}}, LocalToken: "local-secret",
	}
}

func TestStoreFailedAtomicReplacePreservesDestination(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp_config.enc")
	require.NoError(t, os.Mkdir(path, 0700))
	marker := filepath.Join(path, "existing")
	require.NoError(t, os.WriteFile(marker, []byte("preserve"), 0600))
	require.Error(t, NewStore(path).Save(testState()))
	data, err := os.ReadFile(marker)
	require.NoError(t, err)
	require.Equal(t, "preserve", string(data))
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1, "替换失败也要清理临时文件")
}
