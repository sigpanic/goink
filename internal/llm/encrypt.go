package llm

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/sigpanic/goink/internal/config"
	"github.com/sigpanic/goink/internal/utils"
)

// LoadUserConfig 从加密文件读取并解密用户 LLM 配置。
// 文件不存在时返回空的 UserLLMConfig 和 nil error。
func LoadUserConfig(path string) (*UserLLMConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return emptyUserConfig(), nil
		}
		return nil, err
	}

	plain, err := config.Decrypt(data)
	if err != nil {
		return nil, fmt.Errorf("decrypt config failed: %w", err)
	}

	var raw json.RawMessage
	if err := json.Unmarshal(plain, &raw); err != nil {
		return nil, fmt.Errorf("parse config failed: %w", err)
	}
	var cfg UserLLMConfig
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parse config failed: %w", err)
	}
	if err := ValidateUserConfig(&cfg); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	return &cfg, nil
}

// SaveUserConfig 加密并写入用户 LLM 配置到文件。
func SaveUserConfig(path string, cfg *UserLLMConfig) error {
	if err := ValidateUserConfig(cfg); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	plain, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	enc, err := config.Encrypt(plain)
	if err != nil {
		return err
	}

	return utils.AtomicWrite(path, enc)
}
