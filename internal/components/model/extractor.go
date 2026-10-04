package model

import (
	"context"
	"errors"
	"leoture/internal/config"
	"leoture/internal/utils"

	"github.com/cloudwego/eino-ext/components/model/openai"
)

const ExtractorPrompt = "你是文本关键信息分析助手，负责从文本中提取出最关键的 10 个句子。" +
	"规则：1.逐字复制原文，不改写、不总结；" +
	"2.不输出引用位置、不添加任何解释；" +
	"3.如果原文关键信息较少，只输出关键句子，禁止编造。" +
	"4.输出内容不需要编号，直接合并为一段话。"

// SetupExtractorModel 根据配置初始化 openai.ChatModel 实例
// 返回的 Model 已经过 Langfuse Tags 包装（如配置了 Tags）
func NewExtractorModel(ctx context.Context, cfgs []config.ModelService) (*openai.ChatModel, error) {
	var extractorModel *openai.ChatModel
	var err error
	for _, cfg := range cfgs {
		if cfg.Provider == LocalModelProvider {
			httpClient := utils.NewRetryableClient(cfg)
			extractorModel, err = openai.NewChatModel(ctx, &openai.ChatModelConfig{
				APIKey:     cfg.APIKey,
				BaseURL:    cfg.BaseURL,
				HTTPClient: httpClient.HTTPClient,
				// 默认取第一个模型
				Model: cfg.ChatModels[0],
			})
			if err != nil {
				return nil, err
			}
			return extractorModel, nil
		}
	}

	return nil, errors.New("NewExtractorModel: no local model provider configured")
}
