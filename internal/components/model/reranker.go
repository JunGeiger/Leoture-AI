package model

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"leoture/internal/config"
	"leoture/internal/utils"
	"net/http"

	"github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/schema"
)

// Reranker 只持有基础设施参数，业务参数全部动态传入
type Reranker struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

// 动态参数，每次 Transform 时通过 Option 传入
type rerankOptions struct {
	model string
	topN  int
	query string
	// 0 表示不过滤
	scoreThreshold float64
	// rerank 失败时返回原 docs
	fallbackOnError bool
	// 可选，用于日志/观测关联
	traceID string
}

func NewReranker(ctx context.Context, cfgs []config.ModelService) (*Reranker, error) {
	for _, cfg := range cfgs {
		if cfg.Provider == LocalModelProvider {
			return &Reranker{
				apiKey:  cfg.APIKey,
				baseURL: cfg.BaseURL,
				client:  utils.NewRetryableClient(cfg).HTTPClient,
			}, nil
		}
	}
	return nil, errors.New("NewReranker: no local model provider configured")
}

// Transform 对输入的文档列表进行重排序。
//
// 所有业务参数（model、query、topN 等）通过 TransformerOption 动态传入。
// 返回结果按 rerank 模型打分从高到低排列。
func (r *Reranker) Transform(
	ctx context.Context,
	src []*schema.Document,
	opts ...document.TransformerOption) ([]*schema.Document, error) {
	// 空输入直接返回，避免无效网络请求
	if len(src) == 0 {
		return src, nil
	}

	// 解析动态参数
	options := &rerankOptions{}
	options = document.GetTransformerImplSpecificOptions(options, opts...)

	if options.model == "" {
		return nil, errors.New("Reranker: model is empty, use WithRerankModel")
	}
	if options.query == "" {
		return nil, errors.New("Reranker: query is empty, use WithRerankQuery")
	}

	ranked, err := r.doRerank(ctx, src, options)
	if err != nil {
		if options.fallbackOnError {
			// 降级：返回原 docs，不写 score
			return src, nil
		}
		return nil, err
	}

	return ranked, nil
}

func (r *Reranker) doRerank(
	ctx context.Context,
	src []*schema.Document,
	options *rerankOptions,
) ([]*schema.Document, error) {
	// 提取文档内容用于 rerank 请求
	texts := make([]string, len(src))
	for i, d := range src {
		texts[i] = d.Content
	}

	topN := options.topN
	if topN <= 0 || topN > len(src) {
		topN = len(src)
	}

	reqBody := map[string]any{
		"model":     options.model,
		"query":     options.query,
		"documents": texts,
		"top_n":     topN,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("Rerank: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("Rerank: new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if r.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+r.apiKey)
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Rerank: http: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("Rerank: read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Rerank: status %d, body: %s", resp.StatusCode, string(respBody))
	}

	var out struct {
		Results []struct {
			Index int     `json:"index"`
			Score float64 `json:"relevance_score"`
		} `json:"results"`
	}
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, fmt.Errorf("Rerank: unmarshal: %w", err)
	}

	ranked := make([]*schema.Document, 0, len(out.Results))
	for _, ri := range out.Results {
		if ri.Index < 0 || ri.Index >= len(src) {
			continue
		}
		if options.scoreThreshold > 0 && ri.Score < options.scoreThreshold {
			continue
		}
		d := src[ri.Index]
		d.WithScore(ri.Score)
		ranked = append(ranked, d)
	}

	return ranked, nil
}

func WithRerankModel(model string) document.TransformerOption {
	return document.WrapTransformerImplSpecificOptFn(func(o *rerankOptions) {
		o.model = model
	})
}

func WithRerankTopN(topN int) document.TransformerOption {
	return document.WrapTransformerImplSpecificOptFn(func(o *rerankOptions) {
		o.topN = topN
	})
}

func WithRerankQuery(query string) document.TransformerOption {
	return document.WrapTransformerImplSpecificOptFn(func(o *rerankOptions) {
		o.query = query
	})
}

func WithRerankScoreThreshold(t float64) document.TransformerOption {
	return document.WrapTransformerImplSpecificOptFn(func(o *rerankOptions) {
		o.scoreThreshold = t
	})
}

// WithRerankFallbackOnError 设置 rerank 失败时是否降级返回原 docs（原始顺序，无 score）
func WithRerankFallbackOnError(enable bool) document.TransformerOption {
	return document.WrapTransformerImplSpecificOptFn(func(o *rerankOptions) {
		o.fallbackOnError = enable
	})
}

func WithRerankTraceID(id string) document.TransformerOption {
	return document.WrapTransformerImplSpecificOptFn(func(o *rerankOptions) {
		o.traceID = id
	})
}
