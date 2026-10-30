package v180

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/config"
	"github.com/sigpanic/goink/internal/migrate/engine"
)

const modelSelectionMigration = "v1.8.0-model-selection"

func needsModelSelectionMigration(db *gorm.DB) (bool, error) {
	for _, legacy := range [][2]string{{"app_config", "selected_model_key"}, {"app_config", "reasoning_effort"}, {"sessions", "model"}} {
		if db.Migrator().HasColumn(legacy[0], legacy[1]) {
			return true, nil
		}
	}
	return false, nil
}

type modelSelectionStep struct{}

func (modelSelectionStep) Key() string { return "1-convert-model-selections" }

func (modelSelectionStep) Run(db *gorm.DB, log *slog.Logger) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if tx.Migrator().HasColumn("app_config", "selected_model_key") {
			var settings []struct {
				ID                             uint
				FrozenLlmAdapterModelSelection `gorm:"embedded;embeddedPrefix:default_"`
				SelectedModelKey               string
				LegacyEffort                   string `gorm:"column:reasoning_effort"`
			}
			if err := tx.Table("app_config").Find(&settings).Error; err != nil {
				return fmt.Errorf("read legacy model selection: %w", err)
			}
			var names map[string]string
			for _, s := range settings {
				// 任一新定位字段已写入时保留整组新选择，空 effort 也是有效的默认选择。
				if s.AdapterName != "" || s.ProviderName != "" || s.ModelID != "" || s.SelectedModelKey == "" {
					continue
				}
				if names == nil {
					var err error
					names, err = legacyProviderNames()
					if err != nil {
						return err
					}
				}
				selection, ok := selectionFromLegacyKey(s.SelectedModelKey, names)
				if !ok {
					log.Warn("旧默认模型选择无法确定服务商，已重置为未选择")
				}
				values := map[string]any{
					"default_adapter_name": selection.AdapterName, "default_provider_name": selection.ProviderName,
					"default_model_id": selection.ModelID, "default_reasoning_effort": "",
				}
				if ok {
					values["default_reasoning_effort"] = s.LegacyEffort
				}
				if err := tx.Table("app_config").Where("id = ?", s.ID).Updates(values).Error; err != nil {
					return fmt.Errorf("convert default model selection: %w", err)
				}
			}
		}
		if tx.Migrator().HasColumn("sessions", "model") {
			// 旧 Session 只有裸模型 ID，无法可靠还原服务商；重置旧选择交给前端兜底。
			result := tx.Table("sessions").
				Where("COALESCE(adapter_name, '') = '' AND COALESCE(provider_name, '') = '' AND COALESCE(model_id, '') = ''").
				Updates(map[string]any{"adapter_name": "", "provider_name": "", "model_id": "", "reasoning_effort": ""})
			if result.Error != nil {
				return fmt.Errorf("reset legacy session selections: %w", result.Error)
			}
			log.Info("旧会话模型选择已重置", "sessions", result.RowsAffected)
		}
		for _, legacy := range [][2]string{{"app_config", "selected_model_key"}, {"app_config", "reasoning_effort"}, {"sessions", "model"}} {
			if tx.Migrator().HasColumn(legacy[0], legacy[1]) {
				if err := tx.Exec("ALTER TABLE " + legacy[0] + " DROP COLUMN " + legacy[1]).Error; err != nil {
					return fmt.Errorf("drop legacy %s.%s: %w", legacy[0], legacy[1], err)
				}
			}
		}
		return nil
	})
}

// 仅解释冻结的历史配置格式，不依赖当前内置模型或运行时配置合并。
// 数据库阶段在 .enc 转换前执行；也接受已经转换的 v2 配置，兼容开发期间已启动的库。
func legacyProviderNames() (map[string]string, error) {
	names := make(map[string]string, len(builtinNames))
	for old, name := range builtinNames {
		names[old] = name
	}
	data, err := os.ReadFile(config.LLMConfigPath())
	if errors.Is(err, os.ErrNotExist) {
		return names, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read model selection providers: %w", err)
	}
	plain, err := config.Decrypt(data)
	if err != nil {
		return nil, fmt.Errorf("decrypt model selection providers: %w", err)
	}
	converted, err := convertLLMConfig(plain)
	if err != nil {
		return nil, err
	}
	var next llmConfigV2
	if converted != nil {
		var old legacyLLMConfig
		if err := json.Unmarshal(plain, &old); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(converted, &next); err != nil {
			return nil, err
		}
		providers := next.Adapters["openai_chat"].Providers
		for i, p := range old.Providers {
			names[p.Name] = providers[i].Name
		}
	} else {
		if err := json.Unmarshal(plain, &next); err != nil {
			return nil, err
		}
		for _, p := range next.Adapters["openai_chat"].Providers {
			if p.ProviderType != "" {
				if _, builtin := names[p.Name]; !builtin {
					names[p.Name] = p.Name
				}
			}
		}
	}
	return names, nil
}

func selectionFromLegacyKey(key string, names map[string]string) (FrozenLlmAdapterModelSelection, bool) {
	var result FrozenLlmAdapterModelSelection
	for old, name := range names {
		model, found := strings.CutPrefix(key, old+"/")
		if !found || model == "" {
			continue
		}
		if result.ProviderName != "" {
			return FrozenLlmAdapterModelSelection{}, false
		}
		result = FrozenLlmAdapterModelSelection{AdapterName: "openai_chat", ProviderName: name, ModelID: model}
	}
	return result, result.ProviderName != ""
}

func registerModelSelection() {
	engine.Register(engine.Migration{
		Name:        modelSelectionMigration,
		Description: "将全局默认模型转换为结构化选择，重置旧会话选择并移除旧列",
		// 模型选择属于可丢弃的偏好状态，由前端兜底，不值得为此备份全部小说仓库。
		RequiresBackup:  false,
		NeedsMigration:  needsModelSelectionMigration,
		SchemaStep:      engine.AutoMigrateStep{Models: FrozenModels},
		PostSchemaSteps: []engine.Step{modelSelectionStep{}},
	})
}
