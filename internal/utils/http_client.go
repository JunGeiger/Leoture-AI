package utils

import (
	"leoture/internal/config"
	"time"

	"github.com/hashicorp/go-retryablehttp"
)

const DefaultRequestTimeout = 60 * time.Second

func NewRetryableClient(cfg config.ModelService) *retryablehttp.Client {
	client := retryablehttp.NewClient()

	// 重试次数
	client.RetryMax = *cfg.MaxRetries

	// 重试等待策略（指数退避）
	client.RetryWaitMin = 500 * time.Millisecond
	client.RetryWaitMax = 10 * time.Second

	// 哪些状态码重试
	client.CheckRetry = retryablehttp.DefaultRetryPolicy

	// 请求超时
	if cfg.Timeout != 0 {
		client.HTTPClient.Timeout = cfg.Timeout
	} else {
		client.HTTPClient.Timeout = DefaultRequestTimeout
	}

	return client
}
