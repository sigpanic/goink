package attachment

import (
	"bytes"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func Handler(store *Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/attachments/"), "/")
		if len(parts) != 3 {
			http.NotFound(w, r)
			return
		}
		novelID, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil || novelID <= 0 {
			http.NotFound(w, r)
			return
		}
		data, info, err := store.Read(novelID, Ref{AttachmentID: parts[1], Variant: Variant(parts[2])})
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", info.MIMEType)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "private, max-age=3600")
		http.ServeContent(w, r, parts[1], time.Time{}, bytes.NewReader(data))
	})
}
