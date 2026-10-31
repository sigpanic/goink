package llm

import (
	"encoding/base64"
	"fmt"
	"slices"

	"github.com/sigpanic/goink/internal/attachment"
	"github.com/sigpanic/goink/internal/config"
	"github.com/sigpanic/goink/internal/llm/adapter"
)

const maxRequestImageBytes = 8 << 20

// PrepareImageMessages 在当前小说目录解析图片引用，返回请求专用副本。
// 图片预算包括全部历史和工具结果；不修改原始消息，也不保存 Base64。
func PrepareImageMessages(novelID int64, model ModelInfo, messages []Message) ([]Message, error) {
	if !slices.ContainsFunc(messages, func(m Message) bool { return len(m.Images) > 0 }) {
		return messages, nil
	}
	if !model.SupportsVision {
		return nil, fmt.Errorf("模型 %s 不支持图片输入，请切换支持视觉的模型", model.ID)
	}
	if novelID <= 0 {
		return nil, fmt.Errorf("图片输入需要有效的小说 ID")
	}
	store := attachment.NewStore(config.AttachmentsDirPath())
	prepared := slices.Clone(messages)
	total := 0
	for index, message := range messages {
		if len(message.Images) == 0 {
			continue
		}
		message = message.Clone()
		if len(message.ImageInputs) == 0 {
			message.ImageInputs = make([]adapter.ImageInput, 0, len(message.Images))
			for _, ref := range message.Images {
				data, info, err := store.Read(novelID, ref)
				if err != nil {
					return nil, fmt.Errorf("message %d: 读取图片 %s: %w", index, ref.AttachmentID, err)
				}
				total += len(data)
				if total > maxRequestImageBytes {
					return nil, fmt.Errorf("当前上下文图片合计超过 8 MiB，请减少图片或使用标准清晰度")
				}
				message.ImageInputs = append(message.ImageInputs, adapter.ImageInput{
					ImageInfo: info, Base64: base64.StdEncoding.EncodeToString(data),
				})
			}
		} else {
			for _, input := range message.ImageInputs {
				total += input.Size
			}
			if total > maxRequestImageBytes {
				return nil, fmt.Errorf("当前上下文图片合计超过 8 MiB，请减少图片或使用标准清晰度")
			}
		}
		if err := message.ValidateImages(); err != nil {
			return nil, fmt.Errorf("message %d: %w", index, err)
		}
		prepared[index] = message
	}
	return prepared, nil
}

func prepareRequestImages(backend adapter.ProtocolAdapter, providerName, modelID string, messages []Message, opts *CallOptions) ([]Message, error) {
	if !slices.ContainsFunc(messages, func(m Message) bool { return len(m.Images) > 0 }) {
		return messages, nil
	}
	provider, ok := backend.Providers()[providerName]
	if !ok {
		return nil, fmt.Errorf("unknown provider: %s", providerName)
	}
	for _, model := range provider.Models {
		if model.ID == modelID {
			var novelID int64
			if opts != nil {
				novelID = opts.NovelID
			}
			return PrepareImageMessages(novelID, model, messages)
		}
	}
	return nil, fmt.Errorf("模型未找到: %s/%s", providerName, modelID)
}
