package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/sigpanic/goink/internal/platform"
)

// ErrNotInitialized 表示指针文件不存在，应用尚未完成首次初始化。没初始化弹出来初始化界面，如果初始化了但是还是出错就谈配置错误恢复
var ErrNotInitialized = errors.New("指针文件不存在，应用未初始化")

var (
	globalCfg *AppConfig
	cfgMu     sync.RWMutex
)

// Set 设置全局配置单例，InitWithConfig 成功后调用。
func Set(cfg *AppConfig) {
	cfgMu.Lock()
	defer cfgMu.Unlock()
	globalCfg = cfg
}

// Get 返回全局配置单例，未初始化时返回 nil。
func Get() *AppConfig {
	cfgMu.RLock()
	defer cfgMu.RUnlock()
	return globalCfg
}

// AppConfig 是启动指针文件 ~/.goink/config.json 的内容。
// DataDir 字段保留用于未来扩展，当前数据目录由 platform.DataDir() 确定。
type AppConfig struct {
	DataDir string `json:"data_dir"` // 用户选择的数据根目录（保留字段）
}

// DataDirPath 返回数据根目录（绝对路径）。
func DataDirPath() string {
	return platform.DataDir()
}

// GlobalDBPath 返回全局数据库路径。
func GlobalDBPath() string {
	return filepath.Join(platform.DataDir(), "novel-agent.db")
}

// NovelDirPath 返回指定小说的 Git 仓库根目录。
func NovelDirPath(novelID int64) string {
	return filepath.Join(platform.DataDir(), "novels", fmt.Sprintf("%d", novelID))
}

// LLMConfigPath 返回 LLM 加密配置文件的固定路径 ~/.goink/llm_config.enc。
func LLMConfigPath() string {
	dir, _ := configDir()
	return filepath.Join(dir, "llm_config.enc")
}

// UserSkillsDir 返回用户级 skill 目录 ~/.goink/skills/。
func UserSkillsDir() string {
	dir, _ := configDir()
	return filepath.Join(dir, "skills")
}

// NovelSkillsDir 返回指定小说的 skill 目录。
func NovelSkillsDir(novelID int64) string {
	return filepath.Join(NovelDirPath(novelID), "skills")
}

// StyleSamplesDir 返回全局风格素材目录 ~/.goink/style_samples/。
func StyleSamplesDir() string {
	dir, _ := configDir()
	return filepath.Join(dir, "style_samples")
}

// ModelsDir 返回 ONNX 模型目录路径。
// 优先查安装包自带的 runtime/models/，找不到再 fallback 到用户数据目录。
func ModelsDir() string {
	appDir, err := platform.AppDir()
	if err == nil {
		bundled := platform.BundledModelsDir(appDir)
		if _, err := os.Stat(filepath.Join(bundled, "model.onnx")); err == nil {
			return bundled
		}
	}
	return filepath.Join(platform.DataDir(), "models")
}

// configDir 返回指针文件所在的目录 ~/.goink。
func configDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("获取用户目录失败: %w", err)
	}
	return filepath.Join(home, ".goink"), nil
}

// configPath 返回指针文件的完整路径。
func configPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// Load 读取启动指针文件，返回 AppConfig。
// 文件不存在时返回错误，调用方应引导用户完成初始化。
func Load() (*AppConfig, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s", ErrNotInitialized, path)
		}
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	cfg := &AppConfig{}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	// 确保平台数据目录存在
	dataDir := platform.DataDir()
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return nil, fmt.Errorf("创建数据目录 %s 失败: %w", dataDir, err)
	}
	return cfg, nil
}

// expandTilde 将路径开头的 ~ 替换为当前用户主目录。
func expandTilde(path string) string {
	if path == "" || path == "~" {
		home, _ := os.UserHomeDir()
		return home
	}
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Clean(filepath.Join(home, path[2:]))
	}
	return path
}

// Save 将数据目录路径写入指针文件。自动展开 ~ 并转为绝对路径。
// 如果 ~/.goink/ 目录不存在则自动创建。
func Save(dataDir string) error {
	dataDir = expandTilde(dataDir)
	var err error
	dataDir, err = filepath.Abs(dataDir)
	if err != nil {
		return fmt.Errorf("解析数据目录绝对路径失败: %w", err)
	}

	dir, err := configDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("创建配置目录失败: %w", err)
	}

	path := filepath.Join(dir, "config.json")
	cfg := AppConfig{DataDir: dataDir}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}
	if err := os.WriteFile(path, raw, 0600); err != nil {
		return fmt.Errorf("写入配置文件失败: %w", err)
	}
	return nil
}
