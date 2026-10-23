package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"google.golang.org/genai/interactions/models/apierrors"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

type catalogModel struct {
	Name                       string   `json:"name"`
	DisplayName                string   `json:"displayName"`
	InputTokenLimit            int      `json:"inputTokenLimit"`
	OutputTokenLimit           int      `json:"outputTokenLimit"`
	SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
	Thinking                   *bool    `json:"thinking"`
}

// DiscoverModels 读取独立 Models 目录；目录记录不代表已验证 Interactions 生成能力。
func (c *Adapter) DiscoverModels(ctx context.Context, providerName string) ([]adapter.ModelInfo, error) {
	p, err := c.provider(providerName)
	if err != nil {
		return nil, err
	}
	endpoint, err := parseInteractionsURL(p.ChatURL)
	if err != nil {
		return nil, err
	}
	base, ok := strings.CutSuffix(strings.TrimRight(endpoint.EscapedPath(), "/"), "/interactions")
	if !ok {
		return nil, fmt.Errorf("cannot infer Models endpoint from chat_url; use an /interactions endpoint or add models manually")
	}
	endpoint.RawPath = base + "/models"
	endpoint.Path, _ = url.PathUnescape(endpoint.RawPath)
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	models := make([]adapter.ModelInfo, 0)
	seenIDs, seenTokens := map[string]bool{}, map[string]bool{}
	token := ""
	for {
		page, next, err := p.listModels(ctx, endpoint, token)
		if err != nil {
			return nil, err
		}
		for _, model := range page {
			id := strings.TrimPrefix(model.Name, "models/")
			if strings.TrimSpace(id) == "" || strings.TrimSpace(id) != id || strings.Contains(id, "/") {
				return nil, fmt.Errorf("invalid Gemini Models response: invalid model name %q", model.Name)
			}
			// 排除明确只提供 embedding 等其他方法的条目；generateContent 也不证明支持 Interactions。
			if len(model.SupportedGenerationMethods) > 0 && !slices.Contains(model.SupportedGenerationMethods, "generateContent") {
				continue
			}
			if !seenIDs[id] {
				seenIDs[id] = true
				models = append(models, p.catalogModelInfo(id, model))
			}
		}
		if next == "" {
			return models, nil
		}
		if seenTokens[next] || len(page) == 0 {
			return nil, fmt.Errorf("invalid Gemini Models pagination token")
		}
		seenTokens[next], token = true, next
	}
}

func (p Provider) listModels(ctx context.Context, endpoint *url.URL, token string) ([]catalogModel, string, error) {
	target := *endpoint
	query := target.Query()
	query.Del("pageToken")
	query.Set("pageSize", "1000")
	if token != "" {
		query.Set("pageToken", token)
	}
	target.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("X-Goog-Api-Key", p.APIKey)
	req.Header.Set("Accept", "application/json")
	response, err := p.http.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("gemini Models request failed: %w", err)
	}
	defer response.Body.Close()
	const limit = 8 << 20
	raw, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, "", fmt.Errorf("read Gemini Models response: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		return nil, "", convertSDKError(apierrors.NewAPIError("Models request failed", response.StatusCode, string(raw), response))
	}
	if len(raw) > limit {
		return nil, "", fmt.Errorf("gemini Models response exceeds %d bytes", limit)
	}
	var page struct {
		Models []catalogModel `json:"models"`
		Next   string         `json:"nextPageToken"`
	}
	if json.Unmarshal(raw, &page) != nil || page.Models == nil {
		return nil, "", fmt.Errorf("invalid Gemini Models response: expected a models array")
	}
	return page.Models, page.Next, nil
}

func (p Provider) catalogModelInfo(id string, model catalogModel) adapter.ModelInfo {
	info := adapter.ModelInfo{ID: id, Name: id}
	for _, candidates := range [][]adapter.ModelInfo{p.Models, builtin["Google"].Models} {
		found := false
		for _, preset := range candidates {
			if preset.ID == id {
				info, found = preset.Clone(), true
				break
			}
		}
		if found {
			break
		}
	}
	if model.DisplayName != "" {
		info.Name = model.DisplayName
	}
	if model.InputTokenLimit > 0 {
		info.ContextWindow = model.InputTokenLimit
	}
	if model.OutputTokenLimit > 0 {
		info.MaxOutputTokens = model.OutputTokenLimit
	}
	if model.Thinking != nil {
		info.SupportsThinking = *model.Thinking
	}
	if !info.SupportsThinking {
		info.ReasoningLevels = nil
	}
	return info
}
