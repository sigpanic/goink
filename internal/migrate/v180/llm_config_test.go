package v180

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/config"
	"github.com/sigpanic/goink/internal/llm"
	"github.com/sigpanic/goink/internal/llm/adapter"
	"github.com/sigpanic/goink/internal/migrate/engine"
	"github.com/stretchr/testify/require"
)

func TestLLMConfigMigrationPreservesOverrides(t *testing.T) {
	plain, err := os.ReadFile("testdata/llm-config-v1.json")
	require.NoError(t, err)
	path, original := writeLegacyConfig(t, plain)
	// 模拟备份已完成、源文件尚未替换就中断，重试应复用原备份。
	require.NoError(t, os.WriteFile(path+llmConfigBackup, original, 0600))
	require.NoError(t, (llmConfigStep{}).Run(nil, slog.Default()))

	loaded, err := llm.LoadUserConfig(path)
	require.NoError(t, err, "迁移结果必须能被当前严格加载器直接读取")
	require.Equal(t, 2, loaded.Version)
	require.Len(t, loaded.Adapters, 1)
	providers := loaded.Adapters[adapter.ProtocolOpenAIChat].Providers
	wantNames := []string{
		"DeepSeek", "OpenCode Zen", "OpenCode Go", "Doubao", "Qwen", "GLM", "MiniMax", "MiMo", "Kimi",
		"my/relay", "DeepSeek (custom 2)", "DeepSeek (custom)",
	}
	var old struct {
		Providers []adapter.Provider `json:"providers"`
	}
	require.NoError(t, json.Unmarshal(plain, &old))
	require.Len(t, providers, len(wantNames))
	for i, name := range wantNames {
		want := old.Providers[i]
		want.Name = name
		if i >= 9 {
			want.ProviderType = "openai"
		}
		require.Equal(t, want, providers[i], "除名称和类型外，密钥、完整 URL、温度及模型字段应逐项保留")
	}
	resolved, err := llm.Merge(loaded)
	require.NoError(t, err)
	for name, providerType := range map[string]string{"Doubao": "bytedance", "Qwen": "alibaba", "MiMo": "xiaomi"} {
		require.Equal(t, providerType, resolved[adapter.ProtocolOpenAIChat][name].ProviderType,
			"历史实例索引照常迁移，运行时从内置配置补齐公司类型")
	}
	backup, err := os.ReadFile(path + llmConfigBackup)
	require.NoError(t, err)
	require.Equal(t, original, backup, "备份保留原始密文，不能重新加密或输出明文")
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Zero(t, info.Mode().Perm()&0077)
}

func TestLLMConfigMigrationLeavesVersionedFilesUntouched(t *testing.T) {
	for _, plain := range []string{
		`{"version":2,"adapters":{}}`,
		`{"version":3,"future_format":{}}`,
	} {
		t.Run(plain, func(t *testing.T) {
			path, original := writeLegacyConfig(t, []byte(plain))
			require.NoError(t, (llmConfigStep{}).Run(nil, slog.Default()))
			after, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, original, after)
			_, err = os.Stat(path + llmConfigBackup)
			require.ErrorIs(t, err, os.ErrNotExist)
		})
	}
	t.Run("missing file", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		require.NoError(t, (llmConfigStep{}).Run(nil, slog.Default()))
		_, err := os.Stat(config.LLMConfigPath())
		require.ErrorIs(t, err, os.ErrNotExist)
	})
}

func TestLLMConfigMigrationRejectsUnsafeInputWithoutWriting(t *testing.T) {
	tests := []struct{ name, plain string }{
		{"invalid json", `{`},
		{"unknown format", `{}`},
		{"null document", `null`},
		{"mixed format", `{"version":2,"adapters":{},"providers":[]}`},
		{"unrecognized version", `{"version":1,"providers":[]}`},
		{"missing adapters", `{"version":2}`},
		{"unknown field", `{"providers":[],"unexpected":true}`},
		{"duplicate provider", `{"providers":[{"name":"mimo"},{"name":"mimo"}]}`},
		{"invalid custom url", `{"providers":[{"name":"relay","api_key":"private-test-key"}]}`},
		{"duplicate model", `{"providers":[{"name":"mimo","models":[{"id":"a"},{"id":"a"}]}]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, original := writeLegacyConfig(t, []byte(tt.plain))
			err := (llmConfigStep{}).Run(nil, slog.Default())
			require.Error(t, err)
			require.NotContains(t, err.Error(), "private-test-key")
			after, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, original, after)
			_, err = os.Stat(path + llmConfigBackup)
			require.ErrorIs(t, err, os.ErrNotExist)
		})
	}
	t.Run("invalid ciphertext", func(t *testing.T) {
		path, _ := writeLegacyConfig(t, []byte(`{"providers":[]}`))
		original := []byte("invalid-ciphertext")
		require.NoError(t, os.WriteFile(path, original, 0600))
		require.Error(t, (llmConfigStep{}).Run(nil, slog.Default()))
		after, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, original, after)
	})
}

func TestLLMConfigBackupFailurePreservesSource(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		t.Run(map[bool]string{false: "backup blocked", true: "backup differs"}[conflict], func(t *testing.T) {
			path, original := writeLegacyConfig(t, []byte(`{"providers":[{"name":"mimo","api_key":"private-test-key"}]}`))
			backupPath := path + llmConfigBackup
			if conflict {
				require.NoError(t, os.WriteFile(backupPath, []byte("keep-existing-backup"), 0600))
			} else {
				require.NoError(t, os.Mkdir(backupPath, 0700))
			}
			require.Error(t, (llmConfigStep{}).Run(nil, slog.Default()))
			after, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, original, after)
			if conflict {
				backup, err := os.ReadFile(backupPath)
				require.NoError(t, err)
				require.Equal(t, "keep-existing-backup", string(backup))
			}
		})
	}
}

func TestLLMConfigResumesAfterFileReplacedBeforeDone(t *testing.T) {
	path, original := writeLegacyConfig(t, []byte(`{"providers":[{"name":"moonshot","api_key":"private-test-key"}]}`))
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&engine.MigrateState{}))
	require.NoError(t, db.Exec(`CREATE TRIGGER block_done BEFORE INSERT ON migrate_state
		WHEN NEW.status = 'done' BEGIN SELECT RAISE(ABORT, 'done blocked'); END`).Error)
	steps := []engine.Step{llmConfigStep{}}
	require.ErrorContains(t, engine.RunSteps(db, slog.Default(), llmConfigMigration, steps), "done blocked")
	migrated, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NotEqual(t, original, migrated)
	_, err = llm.LoadUserConfig(path)
	require.NoError(t, err)
	require.NoError(t, db.Exec("DROP TRIGGER block_done").Error)
	require.NoError(t, engine.RunSteps(db, slog.Default(), llmConfigMigration, steps))
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, migrated, after, "重试不能重加密已经转换的文件")
	backup, err := os.ReadFile(path + llmConfigBackup)
	require.NoError(t, err)
	require.Equal(t, original, backup)
	var state engine.MigrateState
	require.NoError(t, db.Where("migration = ? AND step = ?", llmConfigMigration, steps[0].Key()).First(&state).Error)
	require.Equal(t, "done", state.Status)

	// 完成后故意破坏测试文件；若再次调用步骤会解密失败，成功说明由状态表直接跳过。
	require.NoError(t, os.WriteFile(path, []byte("broken-after-completion"), 0600))
	require.NoError(t, engine.RunSteps(db, slog.Default(), llmConfigMigration, steps))
}

func writeLegacyConfig(t *testing.T, plain []byte) (string, []byte) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	path := config.LLMConfigPath()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
	enc, err := config.Encrypt(plain)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, enc, 0600))
	return path, enc
}
