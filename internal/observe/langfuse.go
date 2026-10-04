package observe

import (
	"errors"
	"leoture/internal/config"
	"log/slog"
	"sync"

	cbLangfuse "github.com/cloudwego/eino-ext/callbacks/langfuse"
	"github.com/cloudwego/eino/callbacks"
)

var (
	once    sync.Once
	flusher func()
	handler callbacks.Handler
)

// SetupLangfuse 初始化 Langfuse 全局 callback，使用 sync.Once 保证只执行一次。
// 重复调用直接返回已有的 flusher（noop 或真实 flusher），不会重复挂载 handler。
//
// 返回的 flusher 必须在进程退出前调用一次，确保缓冲的 trace 全部上报。
// enabled=false 或必填项缺失时返回 noop flusher，零开销。
func SetupLangfuse(cfg config.Langfuse, appCfg config.App) (callbacks.Handler, func(), error) {
	if !cfg.Enabled {
		slog.Warn("langfuse: disabled")
		return nil, nil, nil
	}

	if appCfg.Name == "" || appCfg.Version == "" || appCfg.Env == "" {
		return nil, nil, errors.New("langfuse: app name / version / env config missing")

	}
	if cfg.Host == "" || cfg.PublicKey == "" || cfg.SecretKey == "" {
		return nil, nil, errors.New("langfuse: host / keys config missing")

	}
	var setupErr error
	once.Do(func() {
		handler, flusher = cbLangfuse.NewLangfuseHandler(&cbLangfuse.Config{
			Host:              cfg.Host,
			PublicKey:         cfg.PublicKey,
			SecretKey:         cfg.SecretKey,
			Threads:           cfg.Threads,
			Timeout:           cfg.Timeout,
			MaxTaskQueueSize:  cfg.MaxTaskQueueSize,
			MaxEventSizeBytes: cfg.MaxEventSizeBytes,
			FlushAt:           cfg.FlushAt,
			FlushInterval:     cfg.FlushInterval,
			SampleRate:        cfg.SampleRate,
			MaxRetry:          cfg.MaxRetry,
			Tags:              []string{appCfg.Name + " " + appCfg.Version, appCfg.Name + " " + appCfg.Env},
		})
		if handler == nil || flusher == nil {
			setupErr = errors.New("langfuse: handler / flusher is nil")
			return
		}

		callbacks.AppendGlobalHandlers(handler)

		slog.Info("langfuse: global callback registered",
			slog.String("host", cfg.Host),
			slog.String("name", appCfg.Name),
			slog.String("version", appCfg.Version),
			slog.String("env", appCfg.Env))
	})

	if setupErr != nil {
		return nil, nil, setupErr
	}

	return handler, flusher, nil
}
