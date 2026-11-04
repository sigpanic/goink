package attachment

import "github.com/sigpanic/goink/internal/apperr"

const (
	CodeInvalidNovelID apperr.Code = "attachment.invalid_novel_id"
	// CodeFileTooLarge 原图或处理后的图片超过文件体积限制。
	CodeFileTooLarge apperr.Code = "attachment.file_too_large"
	// CodeTooManyPixels 原图像素数超过解码限制。
	CodeTooManyPixels apperr.Code = "attachment.too_many_pixels"
	// CodeInvalidImage 图片格式不受支持、数据损坏或编码无效。
	CodeInvalidImage apperr.Code = "attachment.invalid_image"
)
