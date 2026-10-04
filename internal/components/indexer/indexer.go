package indexer

import (
	"context"
	"errors"
	"fmt"

	"github.com/cloudwego/eino-ext/components/embedding/openai"
	qdrant_indexer "github.com/cloudwego/eino-ext/components/indexer/qdrant"

	"github.com/qdrant/go-client/qdrant"
)

const QdrantMetaData = "metadata"

// NewQdrantIndexer 创建封装后的 Indexer
//
//	Dimension 由图调用方单独传入
func NewQdrantIndexer(
	ctx context.Context,
	cli *qdrant.Client,
	collections []string,
	embedder *openai.Embedder,
) (map[string]*qdrant_indexer.Indexer, error) {
	if cli == nil {
		return nil, errors.New("NewQdrantIndexer: qdrant client is nil")
	}
	if len(collections) == 0 {
		return nil, errors.New("NewQdrantIndexer: qdrant collections is empty")
	}
	if embedder == nil {
		return nil, errors.New("NewQdrantIndexer: embedder model is nil")
	}

	// 每个 Indexer 固定处理一个 Collection
	qim := make(map[string]*qdrant_indexer.Indexer, len(collections))
	for _, collection := range collections {
		qi, err := qdrant_indexer.NewIndexer(ctx, &qdrant_indexer.Config{
			Client:     cli,
			Embedding:  embedder,
			Collection: collection,
		})
		if err != nil {
			return nil, fmt.Errorf("NewQdrantIndexer: create qdrant indexer failed %s: %w", collection, err)
		}
		qim[collection] = qi
	}

	return qim, nil
}
