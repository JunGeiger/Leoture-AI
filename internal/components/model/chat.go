package model

import (
	"context"
	"leoture/internal/config"
	"leoture/internal/utils"

	"github.com/cloudwego/eino-ext/callbacks/langfuse"
	"github.com/cloudwego/eino-ext/components/model/openai"
)

const (
	// embedder、extractor、raranker模型提供者，固定为本地模型
	LocalModelProvider = "leoture"
)

// SetupChatModel 根据配置初始化 openai.ChatModel 实例
// 返回的 Model 已经过 Langfuse Tags 包装（如配置了 Tags）
func NewChatModel(ctx context.Context, cfgs []config.ModelService) (map[string]*openai.ChatModel, error) {
	chatModelMap := make(map[string]*openai.ChatModel, len(cfgs))
	for _, cfg := range cfgs {
		if !cfg.Enabled || cfg.Provider == LocalModelProvider {
			continue
		}

		httpClient := utils.NewRetryableClient(cfg)
		ctx = langfuse.SetTrace(ctx, langfuse.WithTags(cfg.Tags...))
		chatModel, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
			APIKey:     cfg.APIKey,
			BaseURL:    cfg.BaseURL,
			HTTPClient: httpClient.HTTPClient,
		})
		if err != nil {
			return nil, err
		}
		chatModelMap[cfg.Provider] = chatModel
	}

	return chatModelMap, nil
}
