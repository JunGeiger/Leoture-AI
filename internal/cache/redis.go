package cache

import (
	"context"
	"fmt"
	"leoture/internal/config"
	"leoture/internal/utils"
	"log/slog"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	redisCli *redis.Client
	once     sync.Once
)

// SetupRedisCli Get global Redis client instance (singleton pattern).
//
//	获取全局 Redis 客户端实例（单例模式）
func SetupRedisCli(cfg config.Redis) (*redis.Client, error) {
	// ctx: 上下文，用于控制初始化超时
	ctx, cancel := context.WithTimeout(context.Background(), cfg.DialTimeout)
	defer cancel()

	var setupErr error
	once.Do(func() {
		setupErr = setup(ctx, cfg)
	})

	if setupErr != nil {
		return nil, setupErr
	}
	return redisCli, nil
}

func setup(ctx context.Context, cfg config.Redis) error {
	// 验证地址是否正确
	tcpProbeCfg := utils.DefaultTCPProbeConfig(cfg.Addr)
	if err := utils.TCPProbe(tcpProbeCfg); err != nil {
		return fmt.Errorf("redis: config address verification failed: %w", err)
	}

	// 初始化 Redis 客户端
	redisCli = redis.NewClient(&redis.Options{
		Addr:            cfg.Addr,
		Password:        cfg.Password,
		DB:              cfg.Database,
		PoolSize:        cfg.PoolSize,
		MinIdleConns:    cfg.MinIdleConns,
		ConnMaxLifetime: cfg.ConnMaxLifetime,
		ConnMaxIdleTime: cfg.ConnMaxIdleTime,
		DialTimeout:     cfg.DialTimeout,
		ReadTimeout:     cfg.ReadTimeout,
		WriteTimeout:    cfg.WriteTimeout,
		PoolTimeout:     cfg.PoolTimeout,
	})

	if err := redisCli.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis: connection verification failed: %w", err)
	}

	slog.Info("redis: connection succeeded", slog.String("addr", cfg.Addr), slog.Int("database", cfg.Database))

	return nil
}

// RedisReady health check.
//
//	Redis 健康检查
func RedisReady(ctx context.Context, dialTimeout time.Duration) bool {
	if redisCli == nil {
		return false
	}

	ctx, cancel := context.WithTimeout(ctx, dialTimeout)
	defer cancel()

	if err := redisCli.Ping(ctx).Err(); err != nil {
		slog.Error("redis: health check failed", slog.String("error", err.Error()))
		return false
	}
	return true
}

// Close Close the Redis connection, which is recommended to be called when the program exits.
//
//	关闭Redis连接，建议在程序退出时调用
func Close() {
	if redisCli != nil {
		err := redisCli.Close()
		if err != nil {
			slog.Error("redis: client close failed", slog.String("error", err.Error()))
			return
		}
		redisCli = nil
	}
	slog.Info("redis: client closed")
}
