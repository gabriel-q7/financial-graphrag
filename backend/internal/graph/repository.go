package graph

import (
	"context"
	"fmt"
	"strings"

	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
)

// Reader is what HTTP handlers need from the graph store.
type Reader interface {
	Neighborhood(ctx context.Context, q NeighborQuery) (Subgraph, error)
	Search(ctx context.Context, q SearchQuery) ([]Node, error)
}

// Repository reads the financial graph from Neo4j.
// All queries are parameterized; no client input is concatenated into Cypher.
type Repository struct {
	driver   neo4j.Driver
	database string
}

// NewRepository builds a repository bound to a database.
func NewRepository(driver neo4j.Driver, database string) *Repository {
	return &Repository{driver: driver, database: database}
}

const centerQuery = `
MATCH (n:Entity {id: $id})
RETURN n.id AS id, labels(n) AS labels, properties(n) AS props`

// layerQuery returns the next BFS layer: distinct unseen nodes adjacent to
// the frontier through allowed relationship types, bounded by $budget.
const layerQuery = `
UNWIND $frontier AS fid
MATCH (a:Entity {id: fid})-[r]-(b:Entity)
WHERE (size($types) = 0 OR type(r) IN $types) AND NOT b.id IN $visited
WITH DISTINCT b
ORDER BY b.id
LIMIT $budget
RETURN b.id AS id, labels(b) AS labels, properties(b) AS props`

// edgesQuery returns relationships among the collected nodes, bounded by $limit.
const edgesQuery = `
MATCH (a:Entity)-[r]->(b:Entity)
WHERE a.id IN $ids AND b.id IN $ids AND (size($types) = 0 OR type(r) IN $types)
RETURN type(r) AS type, a.id AS source, b.id AS target
ORDER BY source, type, target
LIMIT $limit`

const searchQuery = `
MATCH (n:Entity)
WHERE ($type = '' OR $type IN labels(n))
  AND ($q = '' OR toLower(n.id) CONTAINS $q OR toLower(coalesce(n.name, '')) CONTAINS $q)
RETURN n.id AS id, labels(n) AS labels, properties(n) AS props
ORDER BY n.id
LIMIT $limit`

// Neighborhood performs a bounded breadth-first expansion around q.ID.
// Depth, node and edge counts are capped by the validated query.
func (r *Repository) Neighborhood(ctx context.Context, q NeighborQuery) (Subgraph, error) {
	types := make([]string, len(q.RelationshipTypes))
	for i, t := range q.RelationshipTypes {
		types[i] = string(t)
	}

	center, err := r.readNodes(ctx, centerQuery, map[string]any{"id": q.ID})
	if err != nil {
		return Subgraph{}, err
	}
	if len(center) == 0 {
		return Subgraph{}, ErrNotFound
	}

	g := Subgraph{Center: center[0], Nodes: []Node{center[0]}, Edges: []Edge{}}
	visited := []string{q.ID}
	frontier := []string{q.ID}
	truncated := false

	for depth := 1; depth <= q.Depth && len(frontier) > 0; depth++ {
		budget := q.Limit - (len(visited) - 1)
		if budget <= 0 {
			truncated = true
			break
		}
		// Ask for one more than the budget to detect truncation.
		layer, err := r.readNodes(ctx, layerQuery, map[string]any{
			"frontier": frontier, "visited": visited, "types": types, "budget": budget + 1,
		})
		if err != nil {
			return Subgraph{}, err
		}
		if len(layer) > budget {
			truncated = true
			layer = layer[:budget]
		}
		frontier = frontier[:0]
		for _, n := range layer {
			g.Nodes = append(g.Nodes, n)
			visited = append(visited, n.ID)
			frontier = append(frontier, n.ID)
		}
	}

	edgeLimit := q.EdgeLimit()
	edges, err := r.readEdges(ctx, map[string]any{"ids": visited, "types": types, "limit": edgeLimit + 1})
	if err != nil {
		return Subgraph{}, err
	}
	if len(edges) > edgeLimit {
		truncated = true
		edges = edges[:edgeLimit]
	}
	g.Edges = edges

	sortSubgraph(&g)
	g.Meta = Meta{
		Depth:             q.Depth,
		NodeLimit:         q.Limit,
		EdgeLimit:         edgeLimit,
		RelationshipTypes: q.RelationshipTypes,
		NodeCount:         len(g.Nodes),
		EdgeCount:         len(g.Edges),
		Truncated:         truncated,
	}
	if g.Meta.RelationshipTypes == nil {
		g.Meta.RelationshipTypes = []RelType{}
	}
	return g, nil
}

// Search finds entities by id or name (case-insensitive substring).
func (r *Repository) Search(ctx context.Context, q SearchQuery) ([]Node, error) {
	nodes, err := r.readNodes(ctx, searchQuery, map[string]any{
		"q": strings.ToLower(q.Text), "type": string(q.Type), "limit": q.Limit,
	})
	if nodes == nil {
		nodes = []Node{}
	}
	return nodes, err
}

func (r *Repository) readNodes(ctx context.Context, query string, params map[string]any) ([]Node, error) {
	res, err := neo4j.ExecuteQuery(ctx, r.driver, query, params, neo4j.EagerResultTransformer,
		neo4j.ExecuteQueryWithDatabase(r.database), neo4j.ExecuteQueryWithReadersRouting())
	if err != nil {
		return nil, fmt.Errorf("graph query: %w", err)
	}
	nodes := make([]Node, 0, len(res.Records))
	for _, rec := range res.Records {
		id, _ := rec.Get("id")
		labels, _ := rec.Get("labels")
		props, _ := rec.Get("props")
		n, err := NodeFromRecord(id, labels, props)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, n)
	}
	return nodes, nil
}

func (r *Repository) readEdges(ctx context.Context, params map[string]any) ([]Edge, error) {
	res, err := neo4j.ExecuteQuery(ctx, r.driver, edgesQuery, params, neo4j.EagerResultTransformer,
		neo4j.ExecuteQueryWithDatabase(r.database), neo4j.ExecuteQueryWithReadersRouting())
	if err != nil {
		return nil, fmt.Errorf("graph query: %w", err)
	}
	edges := make([]Edge, 0, len(res.Records))
	for _, rec := range res.Records {
		t, _ := rec.Get("type")
		s, _ := rec.Get("source")
		d, _ := rec.Get("target")
		e, err := EdgeFromRecord(t, s, d)
		if err != nil {
			return nil, err
		}
		edges = append(edges, e)
	}
	return edges, nil
}
