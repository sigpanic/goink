package llm

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/sigpanic/goink/internal/config"
	"github.com/sigpanic/goink/internal/llm/adapter"
	"github.com/stretchr/testify/require"
)

func TestUserConfigEncryptionCompatibility(t *testing.T) {
	// 固定原 LLM 密钥和编码方式，独立验证重构前后的磁盘兼容性。
	legacyKey := [32]byte{
		0x7a, 0x3f, 0x71, 0xe2, 0x5c, 0x9d, 0x0b, 0x46,
		0x1a, 0x5f, 0x33, 0xc8, 0x6e, 0x22, 0x4d, 0x0f,
		0x85, 0xce, 0x1c, 0x29, 0x3f, 0xa7, 0x80, 0xf4,
		0x2e, 0x9c, 0x17, 0xd5, 0x4a, 0x8e, 0xd2, 0x06,
	}
	block, err := aes.NewCipher(legacyKey[:])
	require.NoError(t, err)
	legacyCipher, err := cipher.NewGCM(block)
	require.NoError(t, err)
	state := testUserConfig(adapter.Provider{Name: "test", ProviderType: "openai", APIKey: "private-test-key", ChatURL: "https://example.com"})
	plain, err := json.MarshalIndent(state, "", "  ")
	require.NoError(t, err)
	nonce := make([]byte, legacyCipher.NonceSize())
	legacyData := legacyCipher.Seal(nonce, nonce, plain, nil)
	path := filepath.Join(t.TempDir(), "llm_config.enc")
	require.NoError(t, os.WriteFile(path, legacyData, 0600))
	loaded, err := LoadUserConfig(path)
	require.NoError(t, err)
	require.Equal(t, state, loaded)

	require.NoError(t, SaveUserConfig(path, state))
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NotContains(t, string(data), "private-test-key")
	size := legacyCipher.NonceSize()
	decoded, err := legacyCipher.Open(nil, data[:size], data[size:], nil)
	require.NoError(t, err, "新文件仍能使用原 LLM 的密钥和编码方式读取")
	require.JSONEq(t, string(plain), string(decoded))
	sharedPlain, err := config.Decrypt(data)
	require.NoError(t, err)
	require.Equal(t, decoded, sharedPlain)
	loaded, err = LoadUserConfig(path)
	require.NoError(t, err)
	require.Equal(t, state, loaded)

	data[len(data)-1] ^= 1
	require.NoError(t, os.WriteFile(path, data, 0600))
	_, err = LoadUserConfig(path)
	require.Error(t, err)
	require.NotNil(t, errors.Unwrap(err))
	require.NotContains(t, err.Error(), "private-test-key")
	_, err = config.Decrypt([]byte("short"))
	require.Error(t, err)
}

func TestLoadUserConfigPreservesJSONError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "llm_config.enc")
	data, err := config.Encrypt([]byte("{"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0600))
	_, err = LoadUserConfig(path)
	var parseErr *json.SyntaxError
	require.ErrorAs(t, err, &parseErr)
	require.ErrorContains(t, err, "parse config failed")
}

func TestLoadUserConfigMissingFileReturnsNewFormat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.enc")
	loaded, err := LoadUserConfig(path)
	require.NoError(t, err)
	require.Equal(t, ConfigVersion, loaded.Version)
	require.NotNil(t, loaded.Adapters)
	require.Empty(t, loaded.Adapters)
	_, err = os.Stat(path)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestLoadUserConfigRejectsLegacyAndInvalidSchema(t *testing.T) {
	tests := []struct {
		name string
		json string
		want string
	}{
		{"legacy top level", `{"providers":[{"name":"mimo","api_key":"private-test-key"}]}`, `unknown field "providers"`},
		{"legacy provider name", `{"version":2,"adapters":{"openai_chat":{"providers":[{"name":"relay","provider_name":"legacy","provider_type":"mimo","chat_url":"https://relay.example"}]}}}`, `unknown field "provider_name"`},
		{"legacy adapter name", `{"version":2,"adapters":{"openai_chat":{"providers":[{"name":"relay","adapter_name":"openai_chat","provider_type":"mimo","chat_url":"https://relay.example"}]}}}`, `unknown field "adapter_name"`},
		{"missing version", `{"adapters":{}}`, "version"},
		{"unknown version", `{"version":3,"adapters":{}}`, "version"},
		{"missing adapters", `{"version":2}`, "adapters"},
		{"null adapters", `{"version":2,"adapters":null}`, "adapters"},
		{"missing type", `{"version":2,"adapters":{"openai_chat":{"providers":[{"name":"MiMo","api_key":"private-test-key"}]}}}`, "provider type"},
		{"duplicate name", `{"version":2,"adapters":{"openai_chat":{"providers":[{"name":"MiMo","provider_type":"mimo"},{"name":"MiMo","provider_type":"mimo"}]}}}`, "duplicate provider"},
		{"unknown nested field", `{"version":2,"adapters":{"openai_chat":{"providers":[{"name":"MiMo","provider_type":"mimo","extra":"value"}]}}}`, `unknown field "extra"`},
		{"multiple documents", `{"version":2,"adapters":{}} {"version":2,"adapters":{}}`, "parse config failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.enc")
			data, err := config.Encrypt([]byte(tt.json))
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(path, data, 0600))
			_, err = LoadUserConfig(path)
			require.ErrorContains(t, err, tt.want)
			require.NotContains(t, err.Error(), "private-test-key")
			after, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, data, after, "加载不应隐式迁移或重写配置")
		})
	}
}

func TestSaveUserConfigRejectsInvalidWithoutChangingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.enc")
	valid := testUserConfig(adapter.Provider{Name: "MiMo", ProviderType: "mimo", APIKey: "key", Temperature: floatPtr(0)})
	require.NoError(t, SaveUserConfig(path, valid))
	before, err := os.ReadFile(path)
	require.NoError(t, err)
	invalid := testUserConfig(adapter.Provider{Name: "MiMo", ProviderType: "mimo"}, adapter.Provider{Name: "MiMo", ProviderType: "mimo"})
	require.ErrorContains(t, SaveUserConfig(path, invalid), "duplicate provider")
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, before, after)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1, "无效配置不应留下临时文件")
	loaded, err := LoadUserConfig(path)
	require.NoError(t, err)
	require.Equal(t, valid, loaded)
}

func TestSaveUserConfigPersistsExplicitRecordsWithoutKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.enc")
	state := testUserConfig(adapter.Provider{Name: "relay", ProviderType: "mimo", ChatURL: "https://relay.example", Temperature: floatPtr(0)})
	require.NoError(t, SaveUserConfig(path, state))
	loaded, err := LoadUserConfig(path)
	require.NoError(t, err)
	require.Equal(t, state, loaded, "底层IO不使用设置页的无Key过滤规则")
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0600), info.Mode().Perm())
}
