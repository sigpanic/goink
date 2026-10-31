package attachment

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/disintegration/imaging"
	"github.com/google/uuid"
)

const (
	MaxFileSize  = 20 << 20
	MaxPixels    = 40_000_000
	standardEdge = 2048
)

type Store struct {
	dir string
}

func NewStore(dir string) *Store {
	return &Store{dir: dir}
}

// Import 复制原图并生成标准版本；文件名仅用于展示，存储路径由后端生成。
func (s *Store) Import(novelID int64, name string, src io.Reader) (Image, error) {
	dir, err := s.novelDir(novelID)
	if err != nil {
		return Image{}, err
	}
	data, err := io.ReadAll(io.LimitReader(src, MaxFileSize+1))
	if err != nil {
		return Image{}, fmt.Errorf("attachment: read image: %w", err)
	}
	if len(data) > MaxFileSize {
		return Image{}, fmt.Errorf("attachment: 图片不能超过 20 MiB")
	}
	info, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return Image{}, fmt.Errorf("attachment: decode image header: %w", err)
	}
	if format != "png" && format != "jpeg" {
		return Image{}, fmt.Errorf("attachment: 仅支持 PNG 和 JPEG 图片")
	}
	if info.Width <= 0 || info.Height <= 0 || int64(info.Width)*int64(info.Height) > MaxPixels {
		return Image{}, fmt.Errorf("attachment: 图片不能超过 4000 万像素")
	}
	decoded, err := imaging.Decode(bytes.NewReader(data), imaging.AutoOrientation(true))
	if err != nil {
		return Image{}, fmt.Errorf("attachment: decode image: %w", err)
	}
	if decoded.Bounds().Dx() > standardEdge || decoded.Bounds().Dy() > standardEdge {
		decoded = imaging.Fit(decoded, standardEdge, standardEdge, imaging.Lanczos)
	}
	var standard bytes.Buffer
	if format == "jpeg" {
		err = jpeg.Encode(&standard, decoded, &jpeg.Options{Quality: 85})
	} else {
		err = png.Encode(&standard, decoded)
	}
	if err != nil {
		return Image{}, fmt.Errorf("attachment: encode standard image: %w", err)
	}
	if standard.Len() > MaxFileSize {
		return Image{}, fmt.Errorf("attachment: 处理后的图片不能超过 20 MiB")
	}
	meta := Image{
		ID: uuid.NewString(), Name: name,
		Original: ImageInfo{MIMEType: "image/" + format, Width: info.Width, Height: info.Height, Size: len(data)},
		Standard: ImageInfo{MIMEType: "image/" + format, Width: decoded.Bounds().Dx(), Height: decoded.Bounds().Dy(), Size: standard.Len()},
	}
	manifest, err := json.Marshal(meta)
	if err != nil {
		return Image{}, fmt.Errorf("attachment: encode manifest: %w", err)
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return Image{}, fmt.Errorf("attachment: create novel directory: %w", err)
	}
	tmp, err := os.MkdirTemp(dir, ".import-")
	if err != nil {
		return Image{}, fmt.Errorf("attachment: create import directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	files := []struct {
		name string
		data []byte
	}{
		{"original." + format, data},
		{"standard-v1." + format, standard.Bytes()},
		{"manifest.json", manifest},
	}
	for _, file := range files {
		if err := os.WriteFile(filepath.Join(tmp, file.name), file.data, 0600); err != nil {
			return Image{}, fmt.Errorf("attachment: write %s: %w", file.name, err)
		}
	}
	if err := os.Rename(tmp, filepath.Join(dir, meta.ID)); err != nil {
		return Image{}, fmt.Errorf("attachment: publish image: %w", err)
	}
	return meta, nil
}

func (s *Store) Get(novelID int64, id string) (Image, error) {
	root, err := s.open(novelID, id)
	if err != nil {
		return Image{}, err
	}
	defer root.Close()
	return readManifest(root, id)
}

// Read 返回指定版本的文件字节，由调用方在构造模型请求时编码为 Base64。
func (s *Store) Read(novelID int64, ref Ref) ([]byte, ImageInfo, error) {
	if ref.Variant != Original && ref.Variant != Standard {
		return nil, ImageInfo{}, fmt.Errorf("attachment: 无效图片版本 %q", ref.Variant)
	}
	root, err := s.open(novelID, ref.AttachmentID)
	if err != nil {
		return nil, ImageInfo{}, err
	}
	defer root.Close()
	meta, err := readManifest(root, ref.AttachmentID)
	if err != nil {
		return nil, ImageInfo{}, err
	}
	info := meta.Standard
	if ref.Variant == Original {
		info = meta.Original
	}
	var ext string
	switch info.MIMEType {
	case "image/png":
		ext = "png"
	case "image/jpeg":
		ext = "jpeg"
	default:
		return nil, ImageInfo{}, fmt.Errorf("attachment: 无效图片类型 %q", info.MIMEType)
	}
	data, err := root.ReadFile(string(ref.Variant) + "." + ext)
	if err != nil {
		return nil, ImageInfo{}, fmt.Errorf("attachment: read %s image: %w", ref.Variant, err)
	}
	if len(data) != info.Size {
		return nil, ImageInfo{}, fmt.Errorf("attachment: 图片大小与记录不符")
	}
	return data, info, nil
}

func (s *Store) novelDir(novelID int64) (string, error) {
	if novelID <= 0 {
		return "", fmt.Errorf("attachment: 无效小说 ID")
	}
	return filepath.Join(s.dir, strconv.FormatInt(novelID, 10)), nil
}

func (s *Store) open(novelID int64, id string) (*os.Root, error) {
	if parsed, err := uuid.Parse(id); err != nil || parsed.String() != id {
		return nil, fmt.Errorf("attachment: 无效附件 ID")
	}
	dir, err := s.novelDir(novelID)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("attachment: open novel directory: %w", err)
	}
	defer root.Close()
	attachment, err := root.OpenRoot(id)
	if err != nil {
		return nil, fmt.Errorf("attachment: open image: %w", err)
	}
	return attachment, nil
}

func readManifest(root *os.Root, id string) (Image, error) {
	data, err := root.ReadFile("manifest.json")
	if err != nil {
		return Image{}, fmt.Errorf("attachment: read manifest: %w", err)
	}
	var meta Image
	if err := json.Unmarshal(data, &meta); err != nil {
		return Image{}, fmt.Errorf("attachment: decode manifest: %w", err)
	}
	if meta.ID != id {
		return Image{}, fmt.Errorf("attachment: 附件 ID 与记录不符")
	}
	return meta, nil
}
