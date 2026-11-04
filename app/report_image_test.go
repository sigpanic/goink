package app

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/sigpanic/goink/internal/apperr"
	"github.com/sigpanic/goink/internal/report"
)

func TestSaveAnnualReportImage(t *testing.T) {
	var data bytes.Buffer
	require.NoError(t, png.Encode(&data, image.NewNRGBA(image.Rect(0, 0, 8, 12))))
	input := SaveAnnualReportImageInput{Year: time.Now().Year(), PNGBase64: base64.StdEncoding.EncodeToString(data.Bytes())}
	t.Run("save selected path with PNG extension", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "yearbook")
		result := saveAnnualReportImage(input, func(options runtime.SaveDialogOptions) (string, error) {
			require.Contains(t, options.DefaultFilename, ".png")
			require.Equal(t, "*.png", options.Filters[0].Pattern)
			return path, nil
		})
		require.Equal(t, apperr.CodeOK, result.ErrCode)
		require.True(t, result.Data)
		written, err := os.ReadFile(path + ".png")
		require.NoError(t, err)
		require.Equal(t, data.Bytes(), written)
	})
	t.Run("canceled dialog is not an error", func(t *testing.T) {
		result := saveAnnualReportImage(input, func(runtime.SaveDialogOptions) (string, error) { return "", nil })
		require.Equal(t, apperr.CodeOK, result.ErrCode)
		require.False(t, result.Data)
	})
	t.Run("invalid input never opens a dialog", func(t *testing.T) {
		for _, tc := range []struct {
			input SaveAnnualReportImageInput
			code  apperr.Code
		}{
			{SaveAnnualReportImageInput{Year: 0, PNGBase64: input.PNGBase64}, report.CodeInvalidYear},
			{SaveAnnualReportImageInput{Year: input.Year + 1, PNGBase64: input.PNGBase64}, report.CodeInvalidYear},
			{SaveAnnualReportImageInput{Year: input.Year, PNGBase64: "invalid base64"}, report.CodeInvalidImage},
			{SaveAnnualReportImageInput{Year: input.Year, PNGBase64: base64.StdEncoding.EncodeToString([]byte("not a PNG"))}, report.CodeInvalidImage},
		} {
			result := saveAnnualReportImage(tc.input, func(runtime.SaveDialogOptions) (string, error) { t.Fatal("unexpected save dialog"); return "", nil })
			require.Equal(t, tc.code, result.ErrCode)
			require.False(t, result.Data)
		}
	})
	t.Run("dialog error", func(t *testing.T) {
		result := saveAnnualReportImage(input, func(runtime.SaveDialogOptions) (string, error) { return "", errors.New("dialog unavailable") })
		require.Equal(t, apperr.CodeInternal, result.ErrCode)
		require.Contains(t, result.ErrMsg, "dialog unavailable")
	})
	t.Run("write error", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "missing", "report.png")
		result := saveAnnualReportImage(input, func(runtime.SaveDialogOptions) (string, error) { return path, nil })
		require.Equal(t, apperr.CodeInternal, result.ErrCode)
		require.False(t, result.Data)
		require.Contains(t, result.ErrMsg, "save image")
	})
}
