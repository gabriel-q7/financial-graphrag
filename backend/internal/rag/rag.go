// Package rag implements the GraphRAG question answering pipeline:
//
//  1. embed the question (Embedder, Ollama)
//  2. vector search over Document embeddings (Store, Neo4j vector index)
//  3. find entities named in the question (MatchEntities)
//  4. expand the graph around documents and entities (graph.Reader.Neighborhood)
//  5. build a compact, id-annotated context (BuildContext)
//  6. stream a grounded answer (Generator, Ollama chat)
//  7. validate the ids cited in the answer (ExtractCitations)
//
// Documents get their embeddings from Indexer (run by `go run ./cmd/seed`).
// Entity/relationship extraction from new documents (Extractor) is not implemented yet.
package rag

import (
	"context"
	"strings"

	"github.com/gabrielaquino/financial-graphrag/backend/internal/graph"
)

// Embedder turns texts into vectors. Vector size must equal OLLAMA_EMBED_DIMENSIONS.
// Implemented with langchaingo in internal/llm.
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	EmbedModel() string
}

// GenerationOptions bounds a completion.
type GenerationOptions struct {
	MaxTokens   int
	Temperature float64
}

// GenerationStats describes a completion (measured by the adapter).
type GenerationStats struct {
	Model           string  `json:"model"`
	PromptTokens    int     `json:"promptTokens"`
	GeneratedTokens int     `json:"generatedTokens"`
	FirstTokenMS    int64   `json:"firstTokenMs"`
	GenerationMS    int64   `json:"generationMs"`
	TokensPerSecond float64 `json:"tokensPerSecond"`
}

// Generator streams a completion for a system + user prompt.
// Implemented with langchaingo in internal/llm.
type Generator interface {
	Generate(ctx context.Context, system, user string, opts GenerationOptions, onToken func(string) error) (GenerationStats, error)
	ChatModel() string
}

// Extraction is the output of entity/relationship extraction for one document.
type Extraction struct {
	DocumentID    string
	Entities      []graph.Node
	Relationships []graph.Edge
}

// Extractor finds entities and relationships mentioned in a document.
// Not implemented yet: a 4B model invents relationships, so extraction needs a
// validation and review layer before it can write to the graph.
type Extractor interface {
	Extract(ctx context.Context, documentID, text string) (Extraction, error)
}

// embedPrefixes returns the task prompts each embedding model was trained with.
// Using them noticeably improves retrieval (see Docs/ai-models.md).
func embedPrefixes(model string) (document, query string) {
	switch {
	case strings.HasPrefix(model, "embeddinggemma"):
		return "title: none | text: ", "task: search result | query: "
	case strings.HasPrefix(model, "nomic-embed-text"):
		return "search_document: ", "search_query: "
	case strings.HasPrefix(model, "qwen3-embedding"):
		return "", "Instruct: Given a question in Portuguese, retrieve financial investigation documents that answer it\nQuery: "
	}
	return "", ""
}
