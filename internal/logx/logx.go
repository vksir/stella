// Package logx 负责初始化 slog 日志器。
package logx

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/vksir/stella/internal/config"
)

// New 根据配置创建日志器，同时输出到标准错误和日志文件。
func New(cfg config.LogConfig) (*slog.Logger, error) {
	level, err := parseLevel(cfg.Level)
	if err != nil {
		return nil, err
	}

	file, err := openLogFile(cfg.File)
	if err != nil {
		return nil, err
	}

	opts := &slog.HandlerOptions{Level: level}
	if level == slog.LevelDebug {
		opts.AddSource = true
	}
	return slog.New(slog.NewTextHandler(io.MultiWriter(os.Stderr, file), opts)), nil
}

func parseLevel(s string) (slog.Level, error) {
	switch s {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	}
	return 0, fmt.Errorf("unknown log level: %s", s)
}

func openLogFile(path string) (io.Writer, error) {
	if path == "" {
		return io.Discard, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}
	return file, nil
}
