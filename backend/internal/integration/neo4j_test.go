//go:build integration

// Package integration runs tests against a real Neo4j started with docker compose.
// Run with: go test -tags integration ./internal/integration/
// It uses the same NEO4J_* variables as the API and seeds the synthetic dataset
// (idempotent), so it is safe on the local development database.
package integration

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/gabrielaquino/financial-graphrag/backend/internal/config"
	"github.com/gabrielaquino/financial-graphrag/backend/internal/database"
	"github.com/gabrielaquino/financial-graphrag/backend/internal/graph"
	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
)

func setup(t *testing.T) (context.Context, neo4j.Driver, config.Config) {
	t.Helper()
	cfg, err := config.Load(func(k string) string {
		switch k {
		case "MIGRATIONS_DIR":
			return "../../../neo4j/migrations"
		case "SEED_FILE":
			return "../../../data/seed/financial_network.json"
		}
		return os.Getenv(k)
	})
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	driver, err := database.NewDriver(cfg.Neo4jURI, cfg.Neo4jUser, cfg.Neo4jPassword)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = driver.Close(context.Background()) })
	if err := driver.VerifyConnectivity(ctx); err != nil {
		t.Fatalf("neo4j not reachable (start it with docker compose): %v", err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if _, err := database.MigrateAndSeed(ctx, driver, cfg.Neo4jDatabase, cfg.MigrationsDir, cfg.SeedFile, log); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return ctx, driver, cfg
}

func count(t *testing.T, ctx context.Context, d neo4j.Driver, db, q string) int64 {
	t.Helper()
	res, err := neo4j.ExecuteQuery(ctx, d, q, nil, neo4j.EagerResultTransformer, neo4j.ExecuteQueryWithDatabase(db))
	if err != nil {
		t.Fatal(err)
	}
	v, _ := res.Records[0].Get("c")
	return v.(int64)
}

func TestSeedIsIdempotent(t *testing.T) {
	ctx, driver, cfg := setup(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	nodesBefore := count(t, ctx, driver, cfg.Neo4jDatabase, `MATCH (n:Entity) RETURN count(n) AS c`)
	relsBefore := count(t, ctx, driver, cfg.Neo4jDatabase, `MATCH (:Entity)-[r]->(:Entity) RETURN count(r) AS c`)

	res, err := database.MigrateAndSeed(ctx, driver, cfg.Neo4jDatabase, cfg.MigrationsDir, cfg.SeedFile, log)
	if err != nil {
		t.Fatal(err)
	}
	if res.NodesCreated != 0 || res.RelationshipsCreated != 0 {
		t.Fatalf("re-run created data: %+v", res)
	}
	if int64(res.Nodes) != nodesBefore || int64(res.Relationships) != relsBefore {
		t.Fatalf("db has %d nodes/%d rels, dataset has %d/%d", nodesBefore, relsBefore, res.Nodes, res.Relationships)
	}
	if n := count(t, ctx, driver, cfg.Neo4jDatabase, `MATCH (n:Entity) RETURN count(n) AS c`); n != nodesBefore {
		t.Fatalf("node count changed: %d -> %d", nodesBefore, n)
	}
}

func TestNeighborhood(t *testing.T) {
	ctx, driver, cfg := setup(t)
	repo := graph.NewRepository(driver, cfg.Neo4jDatabase)

	// per_001 (Ana): OWNS acc_001, CONTROLS cmp_001 + cmp_002, USES dev_001, mentioned by doc_001.
	g, err := repo.Neighborhood(ctx, graph.NeighborQuery{ID: "per_001", Depth: 1, Limit: 25})
	if err != nil {
		t.Fatal(err)
	}
	ids := func(g graph.Subgraph) []string {
		var out []string
		for _, n := range g.Nodes {
			out = append(out, n.ID)
		}
		return out
	}
	want := []string{"acc_001", "cmp_001", "cmp_002", "dev_001", "doc_001", "per_001"}
	if !slices.Equal(ids(g), want) {
		t.Fatalf("depth 1 nodes = %v, want %v", ids(g), want)
	}
	if g.Center.Type != graph.Person || g.Center.Properties["name"] != "Ana Ribeiro" {
		t.Fatalf("center: %+v", g.Center)
	}
	// 5 direct edges + doc_001 MENTIONS cmp_002 (both endpoints loaded).
	if len(g.Edges) != 6 || g.Meta.Truncated {
		t.Fatalf("edges=%d truncated=%v", len(g.Edges), g.Meta.Truncated)
	}
	for _, e := range g.Edges {
		if e.Type == graph.Mentions && e.Source != "doc_001" {
			t.Fatalf("MENTIONS direction wrong: %+v", e)
		}
	}

	// Depth 3 following money: acc_001 -SENT-> txn_005 -TO_ACCOUNT-> acc_005 -SENT-> txn_006.
	g, err = repo.Neighborhood(ctx, graph.NeighborQuery{ID: "acc_001", Depth: 3, Limit: 50,
		RelationshipTypes: []graph.RelType{graph.Sent, graph.ToAccount}})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"txn_005", "acc_005", "txn_006", "txn_009", "acc_010"} {
		if !slices.Contains(ids(g), id) {
			t.Errorf("depth 3 money trail missing %s: %v", id, ids(g))
		}
	}
	for _, n := range g.Nodes {
		if n.Type != graph.BankAccount && n.Type != graph.Transaction {
			t.Errorf("relationship filter leaked %s (%s)", n.ID, n.Type)
		}
	}

	// Limits are enforced and reported.
	g, err = repo.Neighborhood(ctx, graph.NeighborQuery{ID: "per_001", Depth: 3, Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Nodes) != 4 || !g.Meta.Truncated {
		t.Fatalf("limit 3: got %d nodes (incl. center), truncated=%v", len(g.Nodes), g.Meta.Truncated)
	}

	if _, err := repo.Neighborhood(ctx, graph.NeighborQuery{ID: "per_999", Depth: 1, Limit: 5}); !errors.Is(err, graph.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestSearch(t *testing.T) {
	ctx, driver, cfg := setup(t)
	repo := graph.NewRepository(driver, cfg.Neo4jDatabase)
	nodes, err := repo.Search(ctx, graph.SearchQuery{Text: "ribeiro", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 || nodes[0].ID != "per_001" {
		t.Fatalf("search: %+v", nodes)
	}
	nodes, err = repo.Search(ctx, graph.SearchQuery{Type: graph.Device, Limit: 50})
	if err != nil || len(nodes) != 6 {
		t.Fatalf("type filter: %d %v", len(nodes), err)
	}
}
