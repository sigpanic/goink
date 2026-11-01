package attachment

type Variant string

const (
	Original Variant = "original"
	Standard Variant = "standard-v1"
)

// Ref 是消息中持久化的图片引用，不包含文件路径或图片数据。
type Ref struct {
	AttachmentID string  `json:"attachment_id"`
	Variant      Variant `json:"variant"`
}

type ImageInfo struct {
	MIMEType string `json:"mime_type"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	Size     int    `json:"size"`
}

// Image 同时是导入结果和本地 manifest，两个版本在导入后不再修改。
type Image struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Original ImageInfo `json:"original"`
	Standard ImageInfo `json:"standard"`
}
