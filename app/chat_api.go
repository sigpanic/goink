package app

import (
	"encoding/json"
	"fmt"

	"github.com/sigpanic/goink/internal/config"
	"github.com/sigpanic/goink/internal/llm"
	"github.com/sigpanic/goink/internal/session"
	"github.com/sigpanic/goink/internal/storage"
)

// SessionMeta 是前端会话列表的轻量视图。
type SessionMeta struct {
	SessionID string `json:"session_id"`
	Title     string `json:"title"`
	UpdatedAt string `json:"updated_at"` // ISO 8601
}

// SessionDetail 是单个会话的完整元数据视图，含用量、推理程度等。
type SessionDetail struct {
	SessionID       string          `json:"session_id"`
	NovelID         int64           `json:"novel_id"`
	Title           string          `json:"title"`
	Model           string          `json:"model"`
	ReasoningEffort string          `json:"reasoning_effort"`
	ActiveVersion   int             `json:"active_version"`
	LastTurnID      int             `json:"last_turn_id"`
	Usage           json.RawMessage `json:"usage,omitempty"`
	CreatedAt       string          `json:"created_at"`
	UpdatedAt       string          `json:"updated_at"`
}

// GetModels 返回所有可用模型列表，由后端决定能力和推理程度。
func (a *App) GetModels() []llm.AvailableModel {
	if a.llmClient == nil {
		return nil
	}
	return llm.Models(a.llmClient.Providers())
}

// GetSessionsInput 是 GetSessions 的入参。
type GetSessionsInput struct {
	NovelID int64  `json:"novel_id"`
	Page    int    `json:"page"`
	Size    int    `json:"size"`
	Search  string `json:"search"`
}

// GetSessions 分页查询当前小说的对话历史。search 非空时搜索消息内容。
func (a *App) GetSessions(input GetSessionsInput) (*storage.PageResult[SessionMeta], error) {
	if a.session == nil {
		return nil, nil
	}
	result, err := a.session.ListSessions(a.ctx, input.NovelID, session.ListSessionsOptions{
		PageParams: storage.PageParams{Page: input.Page, Size: input.Size},
		Search:     input.Search,
	})
	if err != nil {
		a.logger.Error("failed to list sessions", "novel_id", input.NovelID, "search", input.Search, "err", err)
		return nil, fmt.Errorf("app: list sessions: %w", err)
	}

	metas := make([]SessionMeta, 0, len(result.Items))
	for _, s := range result.Items {
		metas = append(metas, SessionMeta{
			SessionID: s.SessionID,
			Title:     s.Title,
			UpdatedAt: s.UpdatedAt.Format("2006-01-02T15:04:05"),
		})
	}
	return storage.NewPageResult(metas, result.Total, input.Page, input.Size), nil
}

// GetSession 加载单个会话的完整元数据，含用量、推理程度等。
func (a *App) GetSession(sessionID string) (*SessionDetail, error) {
	if a.session == nil {
		return nil, nil
	}
	s, err := a.session.GetSession(a.ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("app: get session: %w", err)
	}
	return &SessionDetail{
		SessionID:       s.SessionID,
		NovelID:         s.NovelID,
		Title:           s.Title,
		Model:           s.Model,
		ReasoningEffort: s.ReasoningEffort,
		ActiveVersion:   s.ActiveVersion,
		LastTurnID:      s.LastTurnID,
		Usage:           json.RawMessage(s.Usage),
		CreatedAt:       s.CreatedAt.Format("2006-01-02T15:04:05"),
		UpdatedAt:       s.UpdatedAt.Format("2006-01-02T15:04:05"),
	}, nil
}

// DeleteSession 删除指定会话及其全部消息、关联的操作日志。不可恢复。
func (a *App) DeleteSession(sessionID string) error {
	if a.session == nil {
		return fmt.Errorf("app: session store not initialized")
	}
	if a.agent.IsRunning(sessionID) {
		return fmt.Errorf("对话进行中，无法删除该会话，请等待对话完成或先停止")
	}
	if err := a.session.Delete(a.ctx, sessionID); err != nil {
		a.logger.Error("failed to delete session", "session_id", sessionID, "err", err)
		return fmt.Errorf("app: delete session: %w", err)
	}
	a.logger.Info("deleted session", "session_id", sessionID)
	return nil
}

// GetSessionMessages 加载指定 session 的全部前端可见消息。
func (a *App) GetSessionMessages(sessionID string) ([]session.Message, error) {
	if a.session == nil {
		return nil, nil
	}
	msgs, err := a.session.GetMessagesForFrontend(a.ctx, sessionID)
	if err != nil {
		a.logger.Error("failed to get messages", "session_id", sessionID, "err", err)
		return nil, fmt.Errorf("app: get messages: %w", err)
	}
	if msgs == nil {
		return []session.Message{}, nil
	}
	return msgs, nil
}

// GetLLMConfig 返回合并内置模板和用户配置的完整视图。
func (a *App) GetLLMConfig() (*llm.LLMConfigView, error) {
	user, err := llm.LoadUserConfig(config.LLMConfigPath())
	if err != nil {
		a.logger.Error("failed to load LLM config", "err", err)
		return nil, fmt.Errorf("app: load llm config: %w", err)
	}
	return llm.BuildConfigView(user), nil
}

// SaveLLMConfig 保存用户 LLM 配置并热重载 Client。
func (a *App) SaveLLMConfig(input llm.LLMConfigView) error {
	cfg := input.ToUserConfig()
	if err := llm.SaveUserConfig(config.LLMConfigPath(), cfg); err != nil {
		a.logger.Error("failed to save LLM config", "err", err)
		return fmt.Errorf("app: save llm config: %w", err)
	}

	if a.llmClient != nil {
		providers := llm.Merge(llm.Builtin, cfg)
		a.llmClient.Reload(providers)
	}

	a.logger.Info("LLM config saved and hot-reloaded")
	return nil
}

// DiscoverModels 调用 /models 端点自动发现可用模型列表。
func (a *App) DiscoverModels(chatURL, apiKey string) ([]llm.ModelInfo, error) {
	return llm.DiscoverModels(a.ctx, chatURL, apiKey)
}

// TestConnectionInput 是连通性测试的入参。
type TestConnectionInput struct {
	ProviderName string `json:"provider_name"`
	ChatURL      string `json:"chat_url"`
	APIKey       string `json:"api_key"`
	ModelID      string `json:"model_id"`
}

// TestConnection 发送最小化请求验证 provider 连通性。
// 多层 fallback 真测，返回验证通过的实际 URL（可能和入参不同，是探测到的正确端点）。
// 成功返回 (url, nil)，前端应将 url 回写到 provider.chat_url 再保存，确保保存的 URL 和测试时一致。
// 失败返回 ("", error)。
func (a *App) TestConnection(input TestConnectionInput) (string, error) {
	return llm.TestConnection(a.ctx, llm.Builtin, llm.TestConnectionInput{
		ProviderName: input.ProviderName,
		ChatURL:      input.ChatURL,
		APIKey:       input.APIKey,
		ModelID:      input.ModelID,
	})
}
