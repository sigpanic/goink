package attachment_test

import (
	"bytes"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sigpanic/goink/internal/attachment"
	"github.com/stretchr/testify/require"
)

func TestImagePreviewReadsScopedVariants(t *testing.T) {
	store := attachment.NewStore(t.TempDir())
	handler := attachment.Handler(store)
	var data bytes.Buffer
	require.NoError(t, png.Encode(&data, image.NewGray(image.Rect(0, 0, 32, 32))))
	meta, err := store.Import(7, "picture.png", &data)
	require.NoError(t, err)
	for _, variant := range []attachment.Variant{attachment.Original, attachment.Standard} {
		path := "/attachments/7/" + meta.ID + "/" + string(variant)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusOK, response.Code)
		require.Equal(t, "image/png", response.Header().Get("Content-Type"))
		expected, _, err := store.Read(7, attachment.Ref{AttachmentID: meta.ID, Variant: variant})
		require.NoError(t, err)
		require.Equal(t, expected, response.Body.Bytes())
		request := httptest.NewRequest(http.MethodHead, path, nil)
		response = httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		require.Equal(t, http.StatusOK, response.Code)
		require.Empty(t, response.Body.Bytes())
	}
	for _, path := range []string{
		"/attachments/8/" + meta.ID + "/standard-v1",
		"/attachments/7/" + meta.ID + "/../../original",
		"/attachments/7/" + meta.ID + "/manifest.json",
		"/attachments/0/" + meta.ID + "/original",
		"/attachments/7/../original",
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusNotFound, response.Code, path)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/attachments/7/"+meta.ID+"/original", nil))
	require.Equal(t, http.StatusMethodNotAllowed, response.Code)
}
