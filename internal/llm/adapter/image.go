package adapter

import (
	"fmt"

	"github.com/sigpanic/goink/internal/attachment"
)

// ImageInput 是出站请求中的图片数据，不参与消息持久化或 JSON 审计序列化。
type ImageInput struct {
	attachment.ImageInfo
	Base64 string
}

func (i ImageInput) DataURL() string {
	return "data:" + i.MIMEType + ";base64," + i.Base64
}

func (m Message) ValidateImages() error {
	if len(m.Images) != len(m.ImageInputs) {
		return fmt.Errorf("图片引用尚未解析")
	}
	if len(m.Images) == 0 {
		return nil
	}
	if m.Role != "user" && m.Role != "tool" {
		return fmt.Errorf("%s 消息不支持图片输入", m.Role)
	}
	for _, input := range m.ImageInputs {
		if input.Base64 == "" || input.Width <= 0 || input.Height <= 0 ||
			(input.MIMEType != "image/png" && input.MIMEType != "image/jpeg") {
			return fmt.Errorf("图片输入缺少有效数据、类型或尺寸")
		}
	}
	return nil
}
