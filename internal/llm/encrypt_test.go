package llm

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/sigpanic/goink/internal/config"
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
	state := &UserLLMConfig{Providers: []Provider{{Name: "test", APIKey: "private-test-key", ChatURL: "https://example.com"}}}
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
	require.NotContains(t, err.Error(), "private-test-key")
	_, err = config.Decrypt([]byte("short"))
	require.Error(t, err)
}
