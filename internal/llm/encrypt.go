package llm

import (
	"encoding/json"
	"errors"
	"os"

	"github.com/sigpanic/goink/internal/config"
)

// LoadUserConfig 从加密文件读取并解密用户 LLM 配置。
// 文件不存在时返回空的 UserLLMConfig 和 nil error。
func LoadUserConfig(path string) (*UserLLMConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &UserLLMConfig{}, nil
		}
		return nil, err
	}

	plain, err := config.Decrypt(data)
	if err != nil {
		return nil, errors.New("decrypt config failed: " + err.Error())
	}

	var cfg UserLLMConfig
	if err := json.Unmarshal(plain, &cfg); err != nil {
		return nil, errors.New("parse config failed: " + err.Error())
	}
	return &cfg, nil
}

// SaveUserConfig 加密并写入用户 LLM 配置到文件。
func SaveUserConfig(path string, cfg *UserLLMConfig) error {
	plain, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	enc, err := config.Encrypt(plain)
	if err != nil {
		return err
	}

	return os.WriteFile(path, enc, 0600)
}
