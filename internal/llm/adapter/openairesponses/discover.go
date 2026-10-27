package openairesponses

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	sdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

// DiscoverModels 只从 /responses 推导同源 Models 目录；目录记录不代表已验证 Responses 生成能力。
func (c *Adapter) DiscoverModels(ctx context.Context, providerName string) ([]adapter.ModelInfo, error) {
	p, err := c.provider(providerName)
	if err != nil {
		return nil, err
	}
	endpoint, err := adapter.ParseChatURL(p.ChatURL)
	if err != nil {
		return nil, err
	}
	base, ok := strings.CutSuffix(strings.TrimRight(endpoint.EscapedPath(), "/"), "/responses")
	if !ok {
		return nil, fmt.Errorf("cannot infer Models endpoint from chat_url; use a /responses endpoint or add models manually")
	}
	endpoint.RawPath = base + "/models"
	endpoint.Path, _ = url.PathUnescape(endpoint.RawPath)
	// 官方 Models.List 没有分页；单次完整 URL 覆盖可保留版本、代理前缀、转义及查询参数。
	opts, err := responsesURLOptions(endpoint.String())
	if err != nil {
		return nil, err
	}
	var response *http.Response
	opts = append(opts, option.WithHeader("Accept", "application/json"), option.WithResponseInto(&response))
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	models := sdk.NewModelService(p.client.Options...)
	page, err := models.List(ctx, opts...)
	if err != nil {
		return nil, convertSDKError(err, response)
	}
	if page == nil || !page.JSON.Data.Valid() || !strings.HasPrefix(strings.TrimSpace(page.JSON.Data.Raw()), "[") {
		return nil, fmt.Errorf("invalid OpenAI Models response: expected a data array")
	}
	if page.JSON.Object.Raw() != "" && (!wireString(page.JSON.Object) || page.Object != "list") {
		return nil, fmt.Errorf("invalid OpenAI Models response: expected a list object")
	}
	if field, present := page.JSON.ExtraFields["has_more"]; present {
		var more bool
		if err := json.Unmarshal([]byte(field.Raw()), &more); err != nil || more {
			return nil, fmt.Errorf("paginated relay catalogs are not supported by OpenAI Models.List; add models manually")
		}
	}
	result := make([]adapter.ModelInfo, 0, len(page.Data))
	seen := make(map[string]bool)
	for _, model := range page.Data {
		if !wireString(model.JSON.ID) || strings.TrimSpace(model.ID) == "" || strings.TrimSpace(model.ID) != model.ID {
			return nil, fmt.Errorf("invalid OpenAI Models response: invalid model ID")
		}
		if seen[model.ID] {
			continue
		}
		seen[model.ID] = true
		result = append(result, p.modelInfo(model.ID))
	}
	return result, nil
}

func (p Provider) modelInfo(id string) adapter.ModelInfo {
	// 官方目录没有能力字段；只采用精确 ID 的实例配置或已核实预设，不按前缀猜测。
	for _, candidates := range [][]adapter.ModelInfo{p.Models, builtin["ChatGPT"].Models} {
		for _, model := range candidates {
			if model.ID == id {
				return model.Clone()
			}
		}
	}
	return adapter.ModelInfo{ID: id, Name: id}
}
