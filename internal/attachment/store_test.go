package attachment_test

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/sigpanic/goink/internal/attachment"
)

func TestImportPersistsOriginalAndStandard(t *testing.T) {
	dir := t.TempDir()
	store := attachment.NewStore(dir)
	source := image.NewNRGBA(image.Rect(0, 0, 4096, 1024))
	var encoded bytes.Buffer
	require.NoError(t, png.Encode(&encoded, source))
	path := filepath.Join(t.TempDir(), "source.png")
	require.NoError(t, os.WriteFile(path, encoded.Bytes(), 0600))
	file, err := os.Open(path)
	require.NoError(t, err)
	meta, err := store.Import(7, "source.png", file)
	require.NoError(t, file.Close())
	require.NoError(t, err)
	require.NoError(t, os.Remove(path))
	require.Equal(t, 4096, meta.Original.Width)
	require.Equal(t, 1024, meta.Original.Height)
	require.Equal(t, 2048, meta.Standard.Width)
	require.Equal(t, 512, meta.Standard.Height)

	restarted := attachment.NewStore(dir)
	loaded, err := restarted.Get(7, meta.ID)
	require.NoError(t, err)
	require.Equal(t, meta, loaded)
	original, info, err := restarted.Read(7, attachment.Ref{AttachmentID: meta.ID, Variant: attachment.Original})
	require.NoError(t, err)
	require.Equal(t, encoded.Bytes(), original)
	require.Equal(t, meta.Original, info)
	standard, info, err := restarted.Read(7, attachment.Ref{AttachmentID: meta.ID, Variant: attachment.Standard})
	require.NoError(t, err)
	require.Equal(t, meta.Standard, info)
	decoded, err := png.Decode(bytes.NewReader(standard))
	require.NoError(t, err)
	require.Equal(t, image.Rect(0, 0, 2048, 512), decoded.Bounds())
	entries, err := os.ReadDir(filepath.Join(dir, "7"))
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, meta.ID, entries[0].Name())
	_, err = restarted.Get(8, meta.ID)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestSmallPNGIsNotEnlargedAndKeepsTransparency(t *testing.T) {
	source := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	source.SetNRGBA(1, 1, color.NRGBA{R: 200, G: 100, B: 50, A: 127})
	var encoded bytes.Buffer
	require.NoError(t, png.Encode(&encoded, source))
	store := attachment.NewStore(t.TempDir())
	meta, err := store.Import(1, "picture.jpg", &encoded)
	require.NoError(t, err)
	require.Equal(t, "image/png", meta.Standard.MIMEType)
	data, info, err := store.Read(1, attachment.Ref{AttachmentID: meta.ID, Variant: attachment.Standard})
	require.NoError(t, err)
	require.Equal(t, 3, info.Width)
	require.Equal(t, 2, info.Height)
	decoded, err := png.Decode(bytes.NewReader(data))
	require.NoError(t, err)
	require.Equal(t, source.NRGBAAt(1, 1), color.NRGBAModel.Convert(decoded.At(1, 1)))
}

func TestJPEGStandardCorrectsOrientation(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 80, 40))
	for y := 0; y < 40; y++ {
		for x := 0; x < 80; x++ {
			pixel := color.RGBA{R: 255, A: 255}
			if x >= 40 {
				pixel = color.RGBA{B: 255, A: 255}
			}
			source.SetRGBA(x, y, pixel)
		}
	}
	var encoded bytes.Buffer
	require.NoError(t, jpeg.Encode(&encoded, source, &jpeg.Options{Quality: 100}))
	// EXIF orientation 6: 顺时针旋转 90 度，使用最小 little-endian TIFF IFD。
	exif := []byte{
		0xff, 0xe1, 0x00, 0x22, 'E', 'x', 'i', 'f', 0, 0,
		'I', 'I', 0x2a, 0, 8, 0, 0, 0, 1, 0,
		0x12, 0x01, 3, 0, 1, 0, 0, 0, 6, 0, 0, 0, 0, 0, 0, 0,
	}
	data := append(bytes.Clone(encoded.Bytes()[:2]), exif...)
	data = append(data, encoded.Bytes()[2:]...)
	store := attachment.NewStore(t.TempDir())
	meta, err := store.Import(1, "rotated.jpeg", bytes.NewReader(data))
	require.NoError(t, err)
	require.Equal(t, 80, meta.Original.Width)
	require.Equal(t, 40, meta.Original.Height)
	standard, info, err := store.Read(1, attachment.Ref{AttachmentID: meta.ID, Variant: attachment.Standard})
	require.NoError(t, err)
	require.Equal(t, "image/jpeg", info.MIMEType)
	require.Equal(t, 40, info.Width)
	require.Equal(t, 80, info.Height)
	decoded, err := jpeg.Decode(bytes.NewReader(standard))
	require.NoError(t, err)
	r, _, b, _ := decoded.At(20, 10).RGBA()
	require.Greater(t, r, b)
	r, _, b, _ = decoded.At(20, 70).RGBA()
	require.Greater(t, b, r)
	original, _, err := store.Read(1, attachment.Ref{AttachmentID: meta.ID, Variant: attachment.Original})
	require.NoError(t, err)
	require.Equal(t, data, original)
}

func TestInvalidImportsDoNotPublishAttachments(t *testing.T) {
	var unsupported, pngImage bytes.Buffer
	require.NoError(t, gif.Encode(&unsupported, image.NewGray(image.Rect(0, 0, 2, 2)), nil))
	require.NoError(t, png.Encode(&pngImage, image.NewGray(image.Rect(0, 0, 1, 1))))
	oversized := bytes.Clone(pngImage.Bytes())
	binary.BigEndian.PutUint32(oversized[16:20], 100_000)
	binary.BigEndian.PutUint32(oversized[20:24], 100_000)
	binary.BigEndian.PutUint32(oversized[29:33], crc32.ChecksumIEEE(oversized[12:29]))
	tests := []struct {
		name   string
		novel  int64
		reader io.Reader
		want   string
	}{
		{"invalid novel", 0, bytes.NewReader(nil), "无效小说 ID"},
		{"unsupported format", 1, bytes.NewReader(unsupported.Bytes()), "仅支持 PNG 和 JPEG"},
		{"empty image", 1, bytes.NewReader(nil), "decode image header"},
		{"corrupt image", 1, bytes.NewReader(pngImage.Bytes()[:33]), "decode image:"},
		{"oversized dimensions", 1, bytes.NewReader(oversized), "4000 万像素"},
		{"oversized file", 1, io.LimitReader(zeroReader{}, attachment.MaxFileSize+1), "20 MiB"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			_, err := attachment.NewStore(dir).Import(tt.novel, "test.png", tt.reader)
			require.ErrorContains(t, err, tt.want)
			entries, err := os.ReadDir(dir)
			require.NoError(t, err)
			require.Empty(t, entries)
		})
	}
}

func TestReadRejectsInvalidReferencesAndEscapingLinks(t *testing.T) {
	dir := t.TempDir()
	store := attachment.NewStore(dir)
	for _, id := range []string{"", "../outside", uuid.NewString() + "/../outside"} {
		_, err := store.Get(1, id)
		require.Error(t, err)
	}
	_, _, err := store.Read(1, attachment.Ref{AttachmentID: uuid.NewString(), Variant: "../original"})
	require.Error(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "1"), 0700))
	id := uuid.NewString()
	if err := os.Symlink(t.TempDir(), filepath.Join(dir, "1", id)); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	_, err = store.Get(1, id)
	require.Error(t, err)
}

func TestMissingAndTruncatedImageAreReported(t *testing.T) {
	dir := t.TempDir()
	store := attachment.NewStore(dir)
	var encoded bytes.Buffer
	require.NoError(t, png.Encode(&encoded, image.NewGray(image.Rect(0, 0, 1, 1))))
	meta, err := store.Import(1, "picture.png", &encoded)
	require.NoError(t, err)
	path := filepath.Join(dir, "1", meta.ID, "standard-v1.png")
	require.NoError(t, os.WriteFile(path, nil, 0600))
	ref := attachment.Ref{AttachmentID: meta.ID, Variant: attachment.Standard}
	_, _, err = store.Read(1, ref)
	require.ErrorContains(t, err, "大小与记录不符")
	require.NoError(t, os.Remove(path))
	_, _, err = store.Read(1, ref)
	require.ErrorIs(t, err, os.ErrNotExist)
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}
