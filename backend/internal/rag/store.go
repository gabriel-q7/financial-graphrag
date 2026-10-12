package rag

import (
	"context"
	"fmt"

	"github.com/gabrielaquino/financial-graphrag/backend/internal/graph"
	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
)

// VectorIndexName is created by neo4j/migrations/002_document_embeddings.cypher.
const VectorIndexName = "document_embedding"

// Store is the Neo4j access needed by retrieval.
type Store interface {
	SearchDocuments(ctx context.Context, vector []float32, k int) ([]DocumentHit, error)
	NamedEntities(ctx context.Context) ([]NamedEntity, error)
}

// Neo4jStore implements Store and the indexer queries.
type Neo4jStore struct {
	driver   neo4j.Driver
	database string
}

// NewNeo4jStore builds a store bound to a database.
func NewNeo4jStore(driver neo4j.Driver, database string) *Neo4jStore {
	return &Neo4jStore{driver: driver, database: database}
}

func (s *Neo4jStore) query(ctx context.Context, q string, params map[string]any) (*neo4j.EagerResult, error) {
	return neo4j.ExecuteQuery(ctx, s.driver, q, params, neo4j.EagerResultTransformer,
		neo4j.ExecuteQueryWithDatabase(s.database))
}

// The map projection avoids sending the 768-float embedding back to the API.
const searchDocumentsQuery = `
CALL db.index.vector.queryNodes($index, $k, $vector) YIELD node, score
RETURN node.id AS id, labels(node) AS labels,
       node {.text, .source, .created_at} AS props, score
ORDER BY score DESC`

// SearchDocuments returns the k documents most similar to vector.
func (s *Neo4jStore) SearchDocuments(ctx context.Context, vector []float32, k int) ([]DocumentHit, error) {
	res, err := s.query(ctx, searchDocumentsQuery, map[string]any{
		"index": VectorIndexName, "k": k, "vector": toFloat64(vector),
	})
	if err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
	}
	hits := make([]DocumentHit, 0, len(res.Records))
	for _, rec := range res.Records {
		id, _ := rec.Get("id")
		labels, _ := rec.Get("labels")
		props, _ := rec.Get("props")
		score, _ := rec.Get("score")
		n, err := graph.NodeFromRecord(id, labels, props)
		if err != nil {
			return nil, err
		}
		f, _ := score.(float64)
		hits = append(hits, DocumentHit{Node: n, Score: f})
	}
	return hits, nil
}

// NamedEntities lists people and companies with their names.
func (s *Neo4jStore) NamedEntities(ctx context.Context) ([]NamedEntity, error) {
	res, err := s.query(ctx, `
MATCH (n:Entity) WHERE (n:Person OR n:Company) AND n.name IS NOT NULL
RETURN n.id AS id, n.name AS name, CASE WHEN n:Person THEN 'Person' ELSE 'Company' END AS type`, nil)
	if err != nil {
		return nil, fmt.Errorf("list named entities: %w", err)
	}
	out := make([]NamedEntity, 0, len(res.Records))
	for _, rec := range res.Records {
		id, _ := rec.Get("id")
		name, _ := rec.Get("name")
		t, _ := rec.Get("type")
		sid, _ := id.(string)
		sname, _ := name.(string)
		st, _ := t.(string)
		out = append(out, NamedEntity{ID: sid, Name: sname, Type: graph.NodeType(st)})
	}
	return out, nil
}

// VectorIndexDimensions returns the configured dimension of the vector index, or 0 if it does not exist.
func (s *Neo4jStore) VectorIndexDimensions(ctx context.Context) (int, error) {
	res, err := s.query(ctx, `
SHOW VECTOR INDEXES YIELD name, options WHERE name = $index
RETURN options.indexConfig['vector.dimensions'] AS dims`, map[string]any{"index": VectorIndexName})
	if err != nil {
		return 0, fmt.Errorf("read vector index: %w", err)
	}
	if len(res.Records) == 0 {
		return 0, nil
	}
	v, _ := res.Records[0].Get("dims")
	n, _ := v.(int64)
	return int(n), nil
}

// documentForIndex is a document with its current embedding metadata.
type documentForIndex struct {
	ID, Text, Model, Hash string
}

func (s *Neo4jStore) documentsForIndex(ctx context.Context) ([]documentForIndex, error) {
	res, err := s.query(ctx, `
MATCH (d:Document)
RETURN d.id AS id, d.text AS text, d.embedding_model AS model, d.embedding_hash AS hash
ORDER BY d.id`, nil)
	if err != nil {
		return nil, fmt.Errorf("list documents: %w", err)
	}
	out := make([]documentForIndex, 0, len(res.Records))
	for _, rec := range res.Records {
		get := func(k string) string { v, _ := rec.Get(k); s, _ := v.(string); return s }
		out = append(out, documentForIndex{ID: get("id"), Text: get("text"), Model: get("model"), Hash: get("hash")})
	}
	return out, nil
}

func (s *Neo4jStore) saveEmbeddings(ctx context.Context, rows []map[string]any) error {
	_, err := s.query(ctx, `
UNWIND $rows AS row
MATCH (d:Document {id: row.id})
SET d.embedding = row.embedding, d.embedding_model = row.model,
    d.embedding_hash = row.hash, d.embedded_at = datetime()`, map[string]any{"rows": rows})
	if err != nil {
		return fmt.Errorf("save embeddings: %w", err)
	}
	return nil
}

func toFloat64(v []float32) []float64 {
	out := make([]float64, len(v))
	for i, f := range v {
		out[i] = float64(f)
	}
	return out
}
