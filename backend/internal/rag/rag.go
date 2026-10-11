// Package rag defines the seams of the GraphRAG pipeline planned for the next stage:
//
//  1. ingestion of documents            -> Document nodes (seed today, ingestion later)
//  2. entity/relationship extraction    -> Extractor
//  3. embeddings via Ollama             -> Embedder (implemented by *ollama.Client)
//  4. vector index in Neo4j             -> migration created once the model dimension is fixed
//  5. vector retrieval                  -> Retriever
//  6. graph expansion around hits       -> graph.Reader.Neighborhood (implemented)
//  7. context building for the LLM      -> ContextBuilder
//  8. grounded answer generation        -> Generator
//
// Only the Embedder and the graph expansion have real implementations today.
// No API endpoint uses this package yet: there is intentionally no chat endpoint
// until retrieval is real.
package rag

import (
	"context"

	"github.com/gabrielaquino/financial-graphrag/backend/internal/graph"
	"github.com/gabrielaquino/financial-graphrag/backend/internal/ollama"
)

// Embedder turns texts into vectors. Vector size must equal OLLAMA_EMBED_DIMENSIONS.
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

var _ Embedder = (*ollama.Client)(nil)

// Extraction is the output of entity/relationship extraction for one document.
type Extraction struct {
	DocumentID    string
	Entities      []graph.Node
	Relationships []graph.Edge
}

// Extractor finds entities and relationships mentioned in a document.
type Extractor interface {
	Extract(ctx context.Context, documentID, text string) (Extraction, error)
}

// Hit is a retrieved node with its similarity score.
type Hit struct {
	Node  graph.Node
	Score float64
}

// Retriever runs vector search over indexed nodes.
type Retriever interface {
	Search(ctx context.Context, embedding []float32, k int) ([]Hit, error)
}

// Evidence is the grounded context handed to the LLM; every fact keeps its source ids.
type Evidence struct {
	Question string
	Hits     []Hit
	Subgraph graph.Subgraph
}

// ContextBuilder renders evidence into a bounded prompt context.
type ContextBuilder interface {
	Build(ev Evidence) (string, error)
}

// Answer is a generated response with the ids of the evidence it cites.
type Answer struct {
	Text      string
	SourceIDs []string
}

// Generator produces an answer from a question and built context.
type Generator interface {
	Generate(ctx context.Context, question, context string) (Answer, error)
}
