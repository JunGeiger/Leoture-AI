package main

import (
	"context"
	"leoture/internal/cache"
	"leoture/internal/components/indexer"
	"leoture/internal/components/memory"
	"leoture/internal/components/model"
	"leoture/internal/components/retriever"
	"leoture/internal/compose/general_chat"
	"leoture/internal/config"
	"leoture/internal/database"
	"leoture/internal/handler"
	"leoture/internal/logger"
	"leoture/internal/observe"
	"leoture/internal/service"
	"leoture/internal/types"
	"leoture/internal/utils"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/middlewares/server/recovery"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
)

func main() {
	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	// 初始化配置
	cfg, err := config.SetupViper()
	if err != nil {
		log.Fatalf("Setup viper error: %v", err)
	}

	// 初始化日志
	if err := logger.SetupSlog(cfg.Log); err != nil {
		log.Fatalf("Setup slog error: %v", err)
	}

	// 初始化 Reids 客户端
	redisCli, err := cache.SetupRedisCli(cfg.Redis)
	if err != nil {
		log.Fatalf("Setup RedisCli error: %v", err)
	}

	// 初始化 Qdrant 客户端
	qdrantCli, err := database.SetupQdrant(cfg.Qdrant)
	if err != nil {
		log.Fatalf("Setup QdrantCli error: %v", err)
	}

	if !database.QdrantReady(cfg.Qdrant.DialTimeout) {
		log.Fatalf("Setup QdrantCli error, health is bad, please check log")
	}

	// 初始化 Langfuse 观测组件
	callbackHandler, flusher, err := observe.SetupLangfuse(cfg.Langfuse, cfg.App)
	if err != nil {
		log.Fatalf("Setup Langfuse error: %v", err)
	}

	// 获取所有云端对话模型组件实例
	chatModelMap, err := model.NewChatModel(ctx, cfg.ModelServices)
	if err != nil {
		log.Fatalf("New ChatModelMap error: %v", err)
	}

	// 获取本地Embedder组件实例
	embedder, err := model.NewEmbedderModel(ctx, cfg.ModelServices)
	if err != nil {
		log.Fatalf("New EmbedderModel error: %v", err)
	}

	// 获取本地摘要模型组件实例
	extractor, err := model.NewExtractorModel(ctx, cfg.ModelServices)
	if err != nil {
		log.Fatalf("New ExtractorModel error: %v", err)
	}

	// 获取本地索引组件实例（依赖Qdrant、本地Embedder，需配置集合列表）
	indexerMap, err := indexer.NewQdrantIndexer(ctx, qdrantCli, cfg.Qdrant.Collections, embedder)
	if err != nil {
		log.Fatalf("New QdrantIndexer error: %v", err)
	}

	// 获取本地召回组件实例（依赖Qdrant、本地embedder，需配置集合列表）
	retrieverMap, err := retriever.NewQdrantRetriever(ctx, qdrantCli, cfg.Qdrant.Collections, embedder)
	if err != nil {
		log.Fatalf("New QdrantRetriever error: %v", err)
	}

	// 获取本地重排模型组件实例
	// reranker, err := model.NewReranker(ctx, cfg.ModelServices)
	// if err != nil {
	// 	log.Fatalf("New Reranker error: %v", err)
	// }

	// 获取对话历史记录封装实例
	dm := memory.NewDialogueMemory(redisCli)
	// 获取对话历史记录保存编排实例
	dialogueMemory, err := general_chat.NewDialogueMemorySaveGraph(ctx, extractor, indexerMap, dm)
	if err != nil {
		log.Fatalf("New DialogueMemorySaveGraph error: %v", err)
	}

	// 获取通用对话场景下模型编排实例（为实现自由切换模型厂商，每个模型供应商对应一个编排）
	generalChatGraphMap, err := general_chat.NewGeneralChatGraph(ctx, chatModelMap, dm, retrieverMap)
	if err != nil {
		log.Fatalf("New GeneralChatGraphMap error: %v", err)
	}

	// 获取Service层实例
	serviceSet := service.NewSet(cfg, generalChatGraphMap, dialogueMemory, callbackHandler)
	// 获取Handler层实例
	handlerSet := handler.NewSet(serviceSet)

	// 初始化Hertz
	hostPort := cfg.Server.Host + ":" + strconv.Itoa(cfg.Server.Port)
	h := server.Default(server.WithHostPorts(hostPort))
	h.Use(recoveryMiddleware())
	h.Use(processingMiddleware)
	setupRouter(h, handlerSet)

	// 健康检查
	h.GET("/health", func(ctx context.Context, c *app.RequestContext) {
		c.JSON(consts.StatusOK, map[string]string{"status": "success", "message": "ok"})
	})

	slog.Info("leoture ai system started")
	h.Spin()

	<-ctx.Done()
	slog.Info("shutdown signal received")

	// 观察组件 flush
	flusher()

	// 关闭资源
	qdrantCli.Close()
	cache.Close()
	slog.Info("leoture ai system exited gracefully")
}

// 初始化Hertz路由
func setupRouter(h *server.Hertz, set *handler.Set) {
	api_v1 := h.Group("/api/v1")
	api_v1.POST("/generalChat", set.GeneralChatHandler.GeneralChat)
}

// 全局错误处理中间件
func recoveryMiddleware() app.HandlerFunc {
	return recovery.Recovery(
		recovery.WithRecoveryHandler(func(ctx context.Context, c *app.RequestContext, err interface{}, stack []byte) {
			slog.ErrorContext(ctx, "handling global error", slog.Any("err", err), slog.String("stack", string(stack)))
			c.AbortWithStatusJSON(consts.StatusInternalServerError, map[string]any{"status": types.ResponseFailed, "message": "internal error"})
		}),
	)
}

// 请求处理中间件
func processingMiddleware(ctx context.Context, c *app.RequestContext) {
	start := time.Now()

	// 设置RequestID
	requestID := c.Request.Header.Get(types.ApiRequestIDKey)
	if requestID == "" {
		requestID = utils.GenUUIDv7().String()
		c.Request.Header.Set(types.ApiRequestIDKey, requestID)
	}

	// 读取 request body（注意：需要缓存以便后续 handler 可以再次读取）
	var reqBody []byte
	if c.Request.Body() != nil {
		reqBody = c.Request.Body()
		// 重新设置 body，因为读取后 body 会被消费
		c.Request.SetBody(reqBody)
	}

	slog.Info("HTTP Request",
		slog.String("request_id", requestID),
		slog.String("method", string(c.Request.Method())),
		slog.String("path", string(c.Request.Path())),
		slog.String("query", string(c.Request.QueryString())),
		slog.String("req_body", string(reqBody)),
		slog.String("client_ip", c.ClientIP()),
		slog.String("user_agent", string(c.Request.Header.UserAgent())),
		slog.String("request_time", start.String()),
	)

	// 处理请求
	c.Next(ctx)

	c.Response.Header.Set(types.ApiRequestIDKey, requestID)
	slog.Info("HTTP Response",
		slog.String("request_id", requestID),
		slog.Int("status", c.Response.StatusCode()),
		slog.String("resp_body", string(c.Response.Body())),
		slog.String("latency", time.Since(start).String()),
		slog.String("response_time", time.Now().String()),
	)
}
