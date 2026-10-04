package model

import (
	"context"
	"errors"
	"leoture/internal/config"
	"leoture/internal/utils"

	"github.com/cloudwego/eino-ext/components/embedding/openai"
)

// 系统级别向量维度
const Dimensions = 1024

func NewEmbedderModel(ctx context.Context, cfgs []config.ModelService) (*openai.Embedder, error) {
	var embedderModel *openai.Embedder
	var err error
	for _, cfg := range cfgs {
		if cfg.Provider == LocalModelProvider {
			if cfg.APIKey == "" {
				err = errors.New("NewEmbedderModel: config api_key can not empty, ")
			}
			if cfg.BaseURL == "" {
				err = errors.New("NewEmbedderModel: config base_url can not empty")
			}
			if len(cfg.EmbeddingModels) == 0 {
				err = errors.New("NewEmbedderModel: config embedding_models can not empty")
			}
			httpClient := utils.NewRetryableClient(cfg)
			dim := Dimensions

			embedderModel, err = openai.NewEmbedder(ctx, &openai.EmbeddingConfig{
				APIKey:     cfg.APIKey,
				BaseURL:    cfg.BaseURL,
				HTTPClient: httpClient.HTTPClient,
				// 默认取第一个模型
				Model:      cfg.EmbeddingModels[0],
				Dimensions: &dim,
			})
			if err != nil {
				return nil, err
			}
			return embedderModel, nil
		}
	}

	return nil, errors.New("NewEmbedderModel: no local model provider configured")
}
