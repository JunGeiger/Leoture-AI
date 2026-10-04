package logger

import (
	"fmt"
	"io"
	"leoture/internal/config"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/natefinch/lumberjack.v2"
)

var once sync.Once

// SetupSlog 初始化日志
func SetupSlog(cfg config.Log) error {
	var setupErr error
	once.Do(func() {
		// 解析日志级别
		level := parseLogLevel(cfg.Level)

		opts := &slog.HandlerOptions{
			Level:     level,
			AddSource: cfg.AddSource,
			ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
				// 敏感字段脱敏
				if isSensitive(a.Key) {
					return slog.String(a.Key, "***")
				}
				return a
			},
		}

		// 确定输出目标 (Writer)
		writers := []io.Writer{os.Stdout}

		if cfg.FilePath != "" {
			// 保证路径正确
			path, err := resolvePath(cfg.FilePath)
			if err != nil {
				setupErr = fmt.Errorf("slog: resolve logfile output path failed: %w", err)
				return
			}

			if err := ensureDir(path); err != nil {
				setupErr = fmt.Errorf("slog: ensure logfile output path failed: %w", err)
				return
			}

			// 使用 lumberjack 实现日志切割和归档
			writers = append(writers, &lumberjack.Logger{
				Filename:   path,
				MaxSize:    cfg.MaxSize,
				MaxBackups: cfg.MaxBackups,
				MaxAge:     cfg.MaxAge,
				Compress:   cfg.Compress,
			})
		}
		writer := io.MultiWriter(writers...)

		// 根据配置选择 Handler (JSON 或 Text)
		var handler slog.Handler
		if strings.ToLower(cfg.Format) == "json" {
			// JSON 格式：适合生产环境，方便 ELK/Loki 采集
			handler = slog.NewJSONHandler(writer, opts)
		} else {
			// Text 格式：适合开发环境，人类易读
			handler = slog.NewTextHandler(writer, opts)
		}

		// 设置为默认 Logger，后续可直接使用 slog.Info 等函数
		slog.SetDefault(slog.New(handler))
	})

	return setupErr
}

// parseLogLevel 解析日志级别
func parseLogLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// isSensitive 判断敏感字段
func isSensitive(key string) bool {
	key = strings.ToLower(key)
	for _, s := range []string{"password", "token", "secret", "authorization", "api_key"} {
		if strings.Contains(key, s) {
			return true
		}
	}
	return false
}

// resolvePath 解析路径
func resolvePath(path string) (string, error) {
	if filepath.IsAbs(path) {
		return path, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(exe), path), nil
}

// ensureDir 确保路径存在
func ensureDir(path string) error {
	return os.MkdirAll(filepath.Dir(path), 0755)
}
