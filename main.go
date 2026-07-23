package main

import (
	"embed"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	ort "github.com/yalue/onnxruntime_go"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"github.com/sigpanic/goink/app"
	"github.com/sigpanic/goink/internal/config"
	"github.com/sigpanic/goink/internal/logger"
	"github.com/sigpanic/goink/internal/platform"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	log := logger.Default()

	if lib, err := platform.ResolveOnnxLib(); err == nil {
		ort.SetSharedLibraryPath(lib)
		log.Info("ONNX 运行库已设置", "path", lib)
	} else {
		log.Warn("未找到 ONNX Runtime 库，向量检索将不可用", "err", err)
	}

	wapp := app.New(log)

	err := wails.Run(&options.App{
		Title:     "Goink",
		Width:     1400,
		Height:    900,
		MinWidth:  900,
		MinHeight: 600,
		Frameless: runtime.GOOS != "darwin", // macOS 用原生标题栏
		AssetServer: &assetserver.Options{
			Assets: assets,
			Middleware: func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if idStr, ok := strings.CutPrefix(r.URL.Path, "/covers/"); ok {
						novelID, err := strconv.ParseInt(idStr, 10, 64)
						if err != nil || novelID <= 0 {
							http.NotFound(w, r)
							return
						}
						coverPath := filepath.Join(config.DataDirPath(), "novels",
							strconv.FormatInt(novelID, 10), "cover.jpg")
						http.ServeFile(w, r, coverPath)
						return
					}
					if r.URL.Path == "/avatar" {
						avatarPath := filepath.Join(config.DataDirPath(), "user", "avatar.jpg")
						http.ServeFile(w, r, avatarPath)
						return
					}
					next.ServeHTTP(w, r)
				})
			},
		},
		OnStartup:  wapp.OnStartup,
		OnShutdown: wapp.OnShutdown,
		Bind: []any{
			wapp,
		},
	})
	if err != nil {
		slog.Error("应用退出", "err", err)
		os.Exit(1)
	}
}
