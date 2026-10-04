package database

import (
	"context"
	"fmt"
	"leoture/internal/config"
	"leoture/internal/utils"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/qdrant/go-client/qdrant"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
)

var (
	qdrantCli *qdrant.Client
	once      sync.Once
)

func SetupQdrant(cfg config.Qdrant) (*qdrant.Client, error) {
	// ctx: 上下文，用于控制初始化超时
	ctx, cancel := context.WithTimeout(context.Background(), cfg.DialTimeout)
	defer cancel()

	var setupErr error
	once.Do(func() {
		qdrantCli, setupErr = setup(ctx, cfg)
	})

	if setupErr != nil {
		return nil, setupErr
	}
	return qdrantCli, nil
}

func setup(ctx context.Context, cfg config.Qdrant) (*qdrant.Client, error) {
	// 验证地址是否正确
	if cfg.Host == "" || cfg.GRPCPort == 0 {
		return nil, fmt.Errorf("qdrant: config host or grpc_prot is empty")
	}
	addr := cfg.Host + ":" + strconv.Itoa(cfg.GRPCPort)
	tcpProbeCfg := utils.DefaultTCPProbeConfig(addr)
	if err := utils.TCPProbe(tcpProbeCfg); err != nil {
		return nil, fmt.Errorf("qdrant: config grpc address verification failed: %w", err)
	}

	qdrantCfg := &qdrant.Config{
		Host:                   cfg.Host,
		Port:                   cfg.GRPCPort,
		APIKey:                 cfg.APIKey,
		SkipCompatibilityCheck: cfg.SkipCompatCheck,
		PoolSize:               cfg.PoolSize,
		KeepAliveTime:          cfg.KeepAliveTime,
		KeepAliveTimeout:       cfg.KeepAliveTimeout,
		RetryConfig: &qdrant.RetryConfig{
			MaxRetries: cfg.MaxRetries,
		},
	}

	grpcOptions := make([]grpc.DialOption, 0, 2)
	if cfg.DialTimeout > 0 {
		grpcOptions = append(grpcOptions, grpc.WithConnectParams(grpc.ConnectParams{
			MinConnectTimeout: cfg.DialTimeout,
			Backoff: backoff.Config{
				BaseDelay:  1.0 * time.Second,
				Multiplier: 1.6,
				Jitter:     0.2,
				MaxDelay:   cfg.DialTimeout * 3,
			},
		}))
	}
	if cfg.DialIdleTimeout > 0 {
		grpcOptions = append(grpcOptions, grpc.WithIdleTimeout(cfg.DialIdleTimeout))
	}
	if len(grpcOptions) > 0 {
		qdrantCfg.GrpcOptions = grpcOptions
	}

	qdrantCli, err := qdrant.NewClient(qdrantCfg)
	if err != nil {
		return nil, fmt.Errorf("qdrant: setup qdrant client failed: %w", err)
	}
	return qdrantCli, nil
}

// QdrantReady health check.
//
//	Qdrant 健康检查
func QdrantReady(dialTimeout time.Duration) bool {
	if qdrantCli == nil {
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
	defer cancel()
	if checkReply, err := qdrantCli.HealthCheck(ctx); err != nil {
		slog.Warn("qdrant: health check failed", slog.String("error", err.Error()))
		return false
	} else {
		slog.Warn("QdrantHealth: ", slog.Any("CheckReply", checkReply))
	}
	return true
}

func Close() {
	if qdrantCli != nil {
		err := qdrantCli.Close()
		if err != nil {
			slog.Error("qdrant: client close failed", slog.String("error", err.Error()))
			return
		}
		qdrantCli = nil
	}
	slog.Info("qdrant: client closed")
}
