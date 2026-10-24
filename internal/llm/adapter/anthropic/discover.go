package anthropic

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

// DiscoverModels 只从明确的 Messages 路由推导 Models 路由，不探测其他协议或主机。
func (c *Adapter) DiscoverModels(ctx context.Context, providerName string) ([]adapter.ModelInfo, error) {
	p, err := c.provider(providerName)
	if err != nil {
		return nil, err
	}
	endpoint, err := adapter.ParseChatURL(p.ChatURL)
	if err != nil {
		return nil, err
	}
	base, ok := strings.CutSuffix(strings.TrimRight(endpoint.EscapedPath(), "/"), "/messages")
	if !ok {
		return nil, fmt.Errorf("cannot infer Models endpoint from chat_url; use a /messages endpoint or add models manually")
	}
	endpoint.RawPath = base + "/models"
	endpoint.Path, _ = url.PathUnescape(endpoint.RawPath)
	urlOption := modelsURLOption(endpoint)
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	models := make([]adapter.ModelInfo, 0)
	seenIDs, seenCursors := map[string]bool{}, map[string]bool{}
	params := sdk.ModelListParams{Limit: sdk.Int(1000)}
	for {
		page, err := p.client.Models.List(ctx, params, urlOption)
		if err != nil {
			return nil, convertSDKError(err)
		}
		if page == nil || !page.JSON.Data.Valid() || !strings.HasPrefix(page.JSON.Data.Raw(), "[") {
			return nil, fmt.Errorf("invalid Anthropic Models response: missing data array")
		}
		for _, model := range page.Data {
			if !stringField(model.JSON.ID) || strings.TrimSpace(model.ID) == "" {
				return nil, fmt.Errorf("invalid Anthropic Models response: missing model ID")
			}
			if !seenIDs[model.ID] {
				seenIDs[model.ID] = true
				models = append(models, p.modelInfo(model))
			}
		}
		if !page.HasMore {
			return models, nil
		}
		if !stringField(page.JSON.LastID) || page.LastID == "" || seenCursors[page.LastID] || len(page.Data) == 0 {
			return nil, fmt.Errorf("invalid Anthropic Models pagination cursor")
		}
		seenCursors[page.LastID] = true
		params.AfterID = sdk.String(page.LastID)
	}
}

// Models 的分页参数由 SDK 生成；路由查询参数保留，分页参数以本次 SDK 参数为准。
// 不能使用 messagesURLOption，否则后续页的 after_id 会被完整 URL 覆盖掉。
func modelsURLOption(endpoint *url.URL) option.RequestOption {
	return option.WithMiddleware(func(req *http.Request, next option.MiddlewareNext) (*http.Response, error) {
		request := req.Clone(req.Context())
		target := *endpoint
		query := target.Query()
		query.Del("after_id")
		query.Del("before_id")
		for key, values := range req.URL.Query() {
			query[key] = values
		}
		target.RawQuery = query.Encode()
		request.URL, request.Host = &target, target.Host
		return next(request)
	})
}

func (p Provider) modelInfo(model sdk.ModelInfo) adapter.ModelInfo {
	info := adapter.ModelInfo{ID: model.ID, Name: model.ID}
	// 缺少能力字段的中转只采用精确 ID 的配置或已验证预设，不按名称猜测。
	for _, candidates := range [][]adapter.ModelInfo{p.Models, builtin["Anthropic"].Models} {
		found := false
		for _, preset := range candidates {
			if preset.ID == model.ID {
				info, found = preset.Clone(), true
				break
			}
		}
		if found {
			break
		}
	}
	if stringField(model.JSON.DisplayName) && model.DisplayName != "" {
		info.Name = model.DisplayName
	}
	if model.JSON.MaxInputTokens.Valid() && model.MaxInputTokens > 0 {
		info.ContextWindow = int(model.MaxInputTokens)
	}
	if model.JSON.MaxTokens.Valid() && model.MaxTokens > 0 {
		info.MaxOutputTokens = int(model.MaxTokens)
	}
	caps := model.Capabilities
	if caps.ImageInput.JSON.Supported.Valid() {
		info.SupportsVision = caps.ImageInput.Supported
	}
	if caps.Thinking.JSON.Supported.Valid() {
		info.SupportsThinking = caps.Thinking.Supported
	}
	if caps.JSON.Effort.Valid() {
		preferred := ""
		if len(info.ReasoningLevels) > 0 {
			preferred = info.ReasoningLevels[0]
		}
		info.ReasoningLevels = nil
		if caps.Effort.Supported {
			for _, level := range []struct {
				name string
				cap  sdk.CapabilitySupport
			}{
				{"low", caps.Effort.Low}, {"medium", caps.Effort.Medium}, {"high", caps.Effort.High},
				{"xhigh", caps.Effort.Xhigh}, {"max", caps.Effort.Max},
			} {
				if level.cap.Supported {
					if level.name == preferred {
						info.ReasoningLevels = append([]string{preferred}, info.ReasoningLevels...)
					} else {
						info.ReasoningLevels = append(info.ReasoningLevels, level.name)
					}
				}
			}
		}
	}
	if !info.SupportsThinking {
		info.ReasoningLevels = nil
	}
	return info
}
