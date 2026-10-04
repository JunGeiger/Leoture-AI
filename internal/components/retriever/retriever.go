package retriever

import (
	"context"
	"errors"
	"fmt"

	"github.com/cloudwego/eino-ext/components/embedding/openai"
	qdrant_retriever "github.com/cloudwego/eino-ext/components/retriever/qdrant"
	"github.com/qdrant/go-client/qdrant"
)

func NewQdrantRetriever(
	ctx context.Context,
	cli *qdrant.Client,
	collections []string,
	embedder *openai.Embedder,
) (map[string]*qdrant_retriever.Retriever, error) {
	if cli == nil {
		return nil, errors.New("NewQdrantRetriever: qdrant client is nil")
	}
	if len(collections) == 0 {
		return nil, errors.New("NewQdrantRetriever: qdrant collections is empty")
	}
	if embedder == nil {
		return nil, errors.New("NewQdrantRetriever: qdrant embedder is nil")
	}

	// 每个 Retriever 固定查询一个 Collection
	var scoreThreshold float64 = 0.5
	qrm := make(map[string]*qdrant_retriever.Retriever, len(collections))
	for _, collection := range collections {
		cfg := &qdrant_retriever.Config{
			Client:         cli,
			Embedding:      embedder,
			Collection:     collection,
			ScoreThreshold: &scoreThreshold,
		}

		qr, err := qdrant_retriever.NewRetriever(ctx, cfg)
		if err != nil {
			return nil, fmt.Errorf("NewQdrantRetriever: create qdrant retriever failed %s: %w", collection, err)
		}
		qrm[collection] = qr
	}

	return qrm, nil
}
