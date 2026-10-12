package rag

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
)

const embedBatchSize = 16

// IndexResult summarizes an indexing run.
type IndexResult struct {
	Model      string `json:"model"`
	Dimensions int    `json:"dimensions"`
	Documents  int    `json:"documents"`
	Embedded   int    `json:"embedded"`
	UpToDate   int    `json:"upToDate"`
}

// Indexer writes embeddings for Document nodes. It is idempotent: a document is
// re-embedded only when its text or the embedding model changed.
type Indexer struct {
	Store      *Neo4jStore
	Embedder   Embedder
	Dimensions int
	Log        *slog.Logger
}

// Run embeds missing or stale documents.
func (ix *Indexer) Run(ctx context.Context) (IndexResult, error) {
	model := ix.Embedder.EmbedModel()
	res := IndexResult{Model: model, Dimensions: ix.Dimensions}

	dims, err := ix.Store.VectorIndexDimensions(ctx)
	if err != nil {
		return res, err
	}
	if dims == 0 {
		return res, fmt.Errorf("vector index %q not found; apply migrations first", VectorIndexName)
	}
	if dims != ix.Dimensions {
		return res, fmt.Errorf("vector index has %d dimensions but OLLAMA_EMBED_DIMENSIONS=%d; add a migration recreating the index", dims, ix.Dimensions)
	}

	docs, err := ix.Store.documentsForIndex(ctx)
	if err != nil {
		return res, err
	}
	res.Documents = len(docs)

	docPrefix, _ := embedPrefixes(model)
	var pending []documentForIndex
	for _, d := range docs {
		text := docPrefix + d.Text
		hash := contentHash(model, text)
		if d.Model == model && d.Hash == hash {
			res.UpToDate++
			continue
		}
		pending = append(pending, documentForIndex{ID: d.ID, Text: text, Hash: hash})
	}

	for start := 0; start < len(pending); start += embedBatchSize {
		batch := pending[start:min(start+embedBatchSize, len(pending))]
		texts := make([]string, len(batch))
		for i, d := range batch {
			texts[i] = d.Text
		}
		vectors, err := ix.Embedder.Embed(ctx, texts)
		if err != nil {
			return res, fmt.Errorf("embed documents: %w", err)
		}
		rows := make([]map[string]any, len(batch))
		for i, d := range batch {
			if len(vectors[i]) != ix.Dimensions {
				return res, fmt.Errorf("model %s returned %d dimensions, expected %d (OLLAMA_EMBED_DIMENSIONS)", model, len(vectors[i]), ix.Dimensions)
			}
			rows[i] = map[string]any{"id": d.ID, "embedding": toFloat64(vectors[i]), "model": model, "hash": d.Hash}
		}
		if err := ix.Store.saveEmbeddings(ctx, rows); err != nil {
			return res, err
		}
		res.Embedded += len(batch)
	}
	if ix.Log != nil {
		ix.Log.Info("document embeddings", "model", model, "embedded", res.Embedded, "upToDate", res.UpToDate)
	}
	return res, nil
}

func contentHash(model, text string) string {
	sum := sha256.Sum256([]byte(model + "\x00" + text))
	return hex.EncodeToString(sum[:])
}
