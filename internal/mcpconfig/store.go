package mcpconfig

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/sigpanic/goink/internal/config"
)

type Connection struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	Transport    string          `json:"transport"`
	Command      string          `json:"command"`
	Args         []string        `json:"args"`
	Endpoint     string          `json:"endpoint"`
	Enabled      bool            `json:"enabled"`
	AllowedTools map[string]bool `json:"allowed_tools"`
}

// Secrets 不用于设置查询；环境变量也按凭据处理，只返回已配置的变量名。
type Secrets struct {
	Env         map[string]string `json:"env"`
	BearerToken string            `json:"bearer_token"`
}

type Local struct {
	Enabled      bool     `json:"enabled"`
	AllowedTools []string `json:"allowed_tools"`
}

type State struct {
	Connections []Connection       `json:"connections"`
	Credentials map[string]Secrets `json:"credentials"`
	Local       Local              `json:"local"`
	LocalToken  string             `json:"local_token"`
}

// Store 将 MCP 配置与凭据作为一个加密快照提交，避免跨文件更新留下半份授权。
// 与 LLM 配置共用应用固定密钥；调用方负责串行化 Load/Save。
type Store struct {
	path string
}

func NewStore(path string) *Store { return &Store{path: path} }

func (s *Store) Load() (State, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return State{Credentials: map[string]Secrets{}}, nil
	}
	if err != nil {
		return State{}, errors.New("读取 MCP 配置失败")
	}
	plain, err := config.Decrypt(data)
	if err != nil {
		return State{}, errors.New("MCP 配置解密失败")
	}
	return decodeState(plain)
}

func decodeState(plain []byte) (State, error) {
	var state State
	if err := json.Unmarshal(plain, &state); err != nil {
		return State{}, errors.New("解析 MCP 配置失败")
	}
	if state.Credentials == nil {
		state.Credentials = map[string]Secrets{}
	}
	return state, nil
}

func (s *Store) Save(state State) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return errors.New("创建 MCP 配置目录失败")
	}
	plain, err := json.Marshal(state)
	if err != nil {
		return errors.New("编码 MCP 配置失败")
	}
	data, err := config.Encrypt(plain)
	if err != nil {
		return errors.New("加密 MCP 配置失败")
	}
	if err := atomicWrite(s.path, data); err != nil {
		return errors.New("保存 MCP 配置失败")
	}
	return nil
}

func atomicWrite(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".mcp-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
