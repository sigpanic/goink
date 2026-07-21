package logger

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"gopkg.in/natefinch/lumberjack.v2"

	"novel/internal/config"
)

// New 创建结构化日志器。
//
//	format: "text"（开发环境）或 "json"（生产环境）
//	level:  slog.LevelDebug / Info / Warn / Error
//	out:    os.Stdout、os.Stderr 或文件
func New(level slog.Level, format string, out io.Writer) *slog.Logger {
	var handler slog.Handler

	opts := &slog.HandlerOptions{
		Level:     level,
		AddSource: true,
	}

	switch format {
	case "json":
		handler = slog.NewJSONHandler(out, opts)
	default:
		handler = slog.NewTextHandler(out, opts)
	}

	return slog.New(handler)
}

// Default 返回开发环境默认日志器：文本格式、Debug 级别，同时写到 stderr 和数据目录下的 goink.log。
// 文件日志自动轮转：单文件 10MB，保留 3 个备份，超过 30 天自动删除。
// stderr 写入失败（如 Windows GUI 无控制台）不影响文件日志。
func Default() *slog.Logger {
	logPath := filepath.Join(config.DataDirPath(), "goink.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0700); err != nil {
		return New(slog.LevelDebug, "text", os.Stderr)
	}

	return New(slog.LevelDebug, "text", &fanWriter{
		writers: []io.Writer{
			os.Stderr,
			&lumberjack.Logger{
				Filename:   logPath,
				MaxSize:    10, // MB
				MaxBackups: 3,
				MaxAge:     30, // days
				Compress:   true,
			},
		},
	})
}

// fanWriter 向多个 writer 同时写入。各 writer 的写入错误互相独立，不影响其他 writer。
type fanWriter struct{ writers []io.Writer }

func (fw *fanWriter) Write(p []byte) (int, error) {
	for _, w := range fw.writers {
		_, _ = w.Write(p)
	}
	return len(p), nil
}
