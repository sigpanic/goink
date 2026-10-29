package v180

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strings"

	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/config"
	"github.com/sigpanic/goink/internal/migrate/engine"
	"github.com/sigpanic/goink/internal/utils"
)

const (
	llmConfigMigration = "v1.8.0-llm-config"
	llmConfigBackup    = ".pre-v1.8.0.bak"
)

// 以下类型和名称映射冻结 v1.8.0 的磁盘格式，不依赖后续版本的 llm 类型或内置列表。
// 旧 Name 是内置索引或用户实例名；旧文件只保存用户配置，不包含内置默认模型。
type legacyLLMConfig struct {
	Providers []legacyProvider `json:"providers"`
}

type legacyProvider struct {
	Name        string      `json:"name"`
	ChatURL     string      `json:"chat_url"`
	APIKey      string      `json:"api_key,omitempty"`
	Models      []modelInfo `json:"models"`
	Temperature *float64    `json:"temperature,omitempty"`
}

type modelInfo struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	ContextWindow    int      `json:"context_window"`
	MaxOutputTokens  int      `json:"max_output_tokens"`
	SupportsThinking bool     `json:"supports_thinking"`
	ReasoningLevels  []string `json:"reasoning_levels,omitempty"`
	SupportsVision   bool     `json:"supports_vision"`
}

type llmConfigV2 struct {
	Version  int                     `json:"version"`
	Adapters map[string]llmAdapterV2 `json:"adapters"`
}

type llmAdapterV2 struct {
	Providers []providerV2 `json:"providers"`
}

type providerV2 struct {
	legacyProvider
	ProviderType string `json:"provider_type,omitempty"`
}

var builtinNames = map[string]string{
	"deepseek":    "DeepSeek",
	"opencode":    "OpenCode Zen",
	"opencode-go": "OpenCode Go",
	"doubao":      "Doubao",
	"qwen":        "Qwen",
	"zhipu":       "GLM",
	"minimax":     "MiniMax",
	"mimo":        "MiMo",
	"moonshot":    "Kimi",
}

type llmConfigStep struct{}

func (llmConfigStep) Key() string { return "1-convert-llm-config-to-protocols" }

func (llmConfigStep) Run(_ *gorm.DB, log *slog.Logger) error {
	path := config.LLMConfigPath()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read legacy LLM config: %w", err)
	}
	plain, err := config.Decrypt(data)
	if err != nil {
		return fmt.Errorf("decrypt legacy LLM config: %w", err)
	}
	converted, err := convertLLMConfig(plain)
	if err != nil {
		return err
	}
	if converted == nil {
		return nil
	}
	enc, err := config.Encrypt(converted)
	if err != nil {
		return fmt.Errorf("encrypt migrated LLM config: %w", err)
	}
	// 先保留原始密文；重试复用同一备份，绝不覆盖不一致的已有备份。
	backupPath := path + llmConfigBackup
	backup, err := os.ReadFile(backupPath)
	if errors.Is(err, os.ErrNotExist) {
		if err := utils.AtomicWrite(backupPath, data); err != nil {
			return fmt.Errorf("backup legacy LLM config: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("read legacy LLM config backup: %w", err)
	} else if !bytes.Equal(backup, data) {
		return fmt.Errorf("legacy LLM config backup differs from source; original file was not changed")
	}
	if err := utils.AtomicWrite(path, enc); err != nil {
		return fmt.Errorf("replace legacy LLM config: %w", err)
	}
	log.Info("LLM 配置已迁移为协议分组格式", "backup", backupPath)
	return nil
}

// convertLLMConfig 只转换无版本的旧 providers 格式；nil 表示已是目标或更新版本。
// 已版本化的配置不重写、不降级，其业务合法性仍由对应版本的运行时加载器校验。
func convertLLMConfig(plain []byte) ([]byte, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(plain, &fields); err != nil {
		return nil, fmt.Errorf("parse LLM config: %w", err)
	}
	_, legacy := fields["providers"]
	if rawVersion, versioned := fields["version"]; versioned {
		var version int
		if err := json.Unmarshal(rawVersion, &version); err != nil || version < 2 || legacy {
			return nil, fmt.Errorf("unrecognized LLM config version or mixed legacy format")
		}
		if version == 2 {
			var adapters map[string]json.RawMessage
			if err := json.Unmarshal(fields["adapters"], &adapters); err != nil || adapters == nil {
				return nil, fmt.Errorf("LLM config version 2 requires an adapters object")
			}
		}
		return nil, nil
	}
	if !legacy {
		return nil, fmt.Errorf("unrecognized legacy LLM config format")
	}
	var old legacyLLMConfig
	decoder := json.NewDecoder(bytes.NewReader(plain))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&old); err != nil {
		return nil, fmt.Errorf("parse legacy LLM config: %w", err)
	}
	// 预留全部内置名和原始自定义名，防止自动改名占用后面实例的名字。
	used := make(map[string]bool, len(builtinNames)+len(old.Providers))
	for _, name := range builtinNames {
		used[name] = true
	}
	seen := make(map[string]bool, len(old.Providers))
	for i, p := range old.Providers {
		if err := validateLegacyProvider(p); err != nil {
			return nil, fmt.Errorf("legacy LLM provider %d: %w", i, err)
		}
		if seen[p.Name] {
			return nil, fmt.Errorf("legacy LLM provider %d has a duplicate name", i)
		}
		seen[p.Name] = true
		used[p.Name] = true
	}
	group := llmAdapterV2{Providers: make([]providerV2, 0, len(old.Providers))}
	for _, p := range old.Providers {
		next := providerV2{legacyProvider: p}
		if builtin, ok := builtinNames[p.Name]; ok {
			next.Name = builtin
		} else {
			next.ProviderType = "openai"
			for _, builtin := range builtinNames {
				if p.Name == builtin {
					next.Name = p.Name + " (custom)"
					for suffix := 2; used[next.Name]; suffix++ {
						next.Name = fmt.Sprintf("%s (custom %d)", p.Name, suffix)
					}
					used[next.Name] = true
					break
				}
			}
		}
		group.Providers = append(group.Providers, next)
	}
	result := llmConfigV2{Version: 2, Adapters: map[string]llmAdapterV2{"openai_chat": group}}
	return json.MarshalIndent(result, "", "  ")
}

func validateLegacyProvider(p legacyProvider) error {
	if p.Name == "" || strings.TrimSpace(p.Name) != p.Name {
		return fmt.Errorf("name must not be empty or have surrounding whitespace")
	}
	_, builtin := builtinNames[p.Name]
	if p.ChatURL != "" || !builtin {
		u, err := url.Parse(p.ChatURL)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			return fmt.Errorf("chat_url requires an absolute HTTP(S) URL")
		}
	}
	seen := make(map[string]bool, len(p.Models))
	for i, model := range p.Models {
		if strings.TrimSpace(model.ID) == "" || seen[model.ID] {
			return fmt.Errorf("model %d has an empty or duplicate id", i)
		}
		seen[model.ID] = true
	}
	return nil
}

// Register 注册独立于数据库历史 schema 的配置文件迁移。
func Register() {
	registerModelSelection()
	engine.Register(engine.Migration{
		Name:                 llmConfigMigration,
		Description:          "将旧 LLM 服务商配置转换为协议分组格式，保留原始加密文件备份",
		PostDBMigrationSteps: []engine.Step{llmConfigStep{}},
	})
}
