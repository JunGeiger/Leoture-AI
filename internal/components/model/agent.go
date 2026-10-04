package model

import (
	"context"
	"leoture/internal/config"
	"leoture/internal/utils"

	"github.com/cloudwego/eino-ext/components/model/agenticclaude"
)

// SetupAgentModel 根据配置初始化 openai.ChatModel 实例
// 返回的 Model 已经过 Langfuse Tags 包装（如配置了 Tags）
func SetupAgentModel(ctx context.Context, cfgs []config.ModelService) (map[string]*agenticclaude.Model, error) {
	agentModelMap := make(map[string]*agenticclaude.Model, len(cfgs))
	for _, cfg := range cfgs {
		if !cfg.Enabled || cfg.Provider == LocalModelProvider {
			continue
		}

		httpClient := utils.NewRetryableClient(cfg)
		agentModel, err := agenticclaude.New(ctx, &agenticclaude.Config{
			APIKey:     cfg.APIKey,
			BaseURL:    cfg.BaseURL,
			HTTPClient: httpClient.HTTPClient,
		})
		if err != nil {
			return nil, err
		}
		agentModelMap[cfg.Provider] = agentModel
	}

	return agentModelMap, nil
}
