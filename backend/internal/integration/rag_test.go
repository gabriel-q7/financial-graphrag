//go:build integration

package integration

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/gabrielaquino/financial-graphrag/backend/internal/graph"
	"github.com/gabrielaquino/financial-graphrag/backend/internal/llm"
	"github.com/gabrielaquino/financial-graphrag/backend/internal/ollama"
	"github.com/gabrielaquino/financial-graphrag/backend/internal/rag"
)

// TestRetrieval runs indexing and retrieval against the real Neo4j and Ollama
// (embedding model only; the chat model is not called). Skipped if Ollama is down.
func TestRetrieval(t *testing.T) {
	ctx, driver, cfg := setup(t)
	client := ollama.New(cfg.OllamaURL, cfg.OllamaChatModel, cfg.OllamaEmbedModel, nil)
	pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if _, err := client.Version(pingCtx); err != nil {
		t.Skipf("ollama not reachable: %v", err)
	}

	generator, embedder, err := llm.NewOllama(llm.Config{OllamaURL: cfg.OllamaURL, ChatModel: cfg.OllamaChatModel, EmbedModel: cfg.OllamaEmbedModel})
	if err != nil {
		t.Fatal(err)
	}
	store := rag.NewNeo4jStore(driver, cfg.Neo4jDatabase)
	ix := rag.Indexer{Store: store, Embedder: embedder, Dimensions: cfg.OllamaEmbedDimensions}
	if _, err := ix.Run(ctx); err != nil {
		t.Fatalf("index: %v", err)
	}
	again, err := ix.Run(ctx)
	if err != nil || again.Embedded != 0 || again.UpToDate != again.Documents {
		t.Fatalf("indexing is not idempotent: %+v %v", again, err)
	}

	s := &rag.Service{Embedder: embedder, Generator: generator, Store: store, Graph: graph.NewRepository(driver, cfg.Neo4jDatabase)}
	ids := func(ns []graph.Node) []string {
		var out []string
		for _, n := range ns {
			out = append(out, n.ID)
		}
		return out
	}

	r, err := s.Retrieve(ctx, "Quais pessoas compartilham o mesmo dispositivo?")
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Documents) == 0 || r.Documents[0].Node.ID != "doc_003" {
		t.Fatalf("expected doc_003 first, got %+v", r.Documents)
	}
	for _, id := range []string{"per_004", "per_005", "per_006", "dev_003"} {
		if !slices.Contains(ids(r.Nodes), id) {
			t.Errorf("evidence missing %s: %v", id, ids(r.Nodes))
		}
	}

	r, err = s.Retrieve(ctx, "Por que a conta da Ana Ribeiro merece revisão?")
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Entities) != 1 || r.Entities[0].ID != "per_001" {
		t.Fatalf("expected per_001, got %v", ids(r.Entities))
	}
	for _, id := range []string{"txn_005", "txn_006", "txn_007", "txn_009"} {
		if !r.Context.IDs[id] {
			t.Errorf("money trail missing %s", id)
		}
	}
	if len(r.Nodes) > rag.MaxContextNodes {
		t.Errorf("context exceeds node cap: %d", len(r.Nodes))
	}

	r, err = s.Retrieve(ctx, "Qual é a capital da França?")
	if err != nil || !r.NoEvidence {
		t.Fatalf("expected no evidence: %+v %v", r, err)
	}
}
