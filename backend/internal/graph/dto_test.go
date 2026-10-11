package graph

import (
	"math"
	"testing"
	"time"

	"github.com/neo4j/neo4j-go-driver/v6/neo4j/dbtype"
)

func TestNodeFromRecordFiltersProperties(t *testing.T) {
	ts := time.Date(2026, 4, 2, 22, 10, 0, 0, time.FixedZone("BRT", -3*3600))
	n, err := NodeFromRecord("txn_005", []any{"Entity", "Transaction"}, map[string]any{
		"id":        "txn_005",
		"amount":    9800.0,
		"currency":  "BRL",
		"status":    "completed",
		"timestamp": ts,
		"embedding": []any{0.1, 0.2},
		"internal":  "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	if n.Type != Transaction || n.ID != "txn_005" {
		t.Fatalf("unexpected node: %+v", n)
	}
	if _, ok := n.Properties["embedding"]; ok {
		t.Error("embedding must not be exposed")
	}
	if _, ok := n.Properties["internal"]; ok {
		t.Error("non-whitelisted property must not be exposed")
	}
	if n.Properties["timestamp"] != "2026-04-03T01:10:00Z" {
		t.Errorf("timestamp not normalized to UTC RFC3339: %v", n.Properties["timestamp"])
	}
	if n.Properties["amount"] != 9800.0 {
		t.Errorf("amount: %v", n.Properties["amount"])
	}
}

func TestNodeFromRecordErrors(t *testing.T) {
	if _, err := NodeFromRecord(42, []any{"Person"}, nil); err == nil {
		t.Error("expected error for non-string id")
	}
	if _, err := NodeFromRecord("per_001", []any{"Entity"}, nil); err == nil {
		t.Error("expected error for missing domain label")
	}
	if _, err := NodeFromRecord("per_001", "Person", nil); err == nil {
		t.Error("expected error for labels not a list")
	}
}

func TestNodeFromRecordMissingProps(t *testing.T) {
	n, err := NodeFromRecord("per_001", []any{"Person", "Entity"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if n.Properties == nil || len(n.Properties) != 0 {
		t.Fatalf("expected empty, non-nil properties: %#v", n.Properties)
	}
}

func TestNormalizeValue(t *testing.T) {
	cases := []struct {
		in   any
		want any
		ok   bool
	}{
		{"x", "x", true},
		{int64(3), int64(3), true},
		{math.NaN(), nil, false},
		{dbtype.Date(time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)), "2026-01-02", true},
		{dbtype.Point2D{X: 1, Y: 2}, nil, false},
		{nil, nil, false},
	}
	for _, c := range cases {
		got, ok := normalizeValue(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("normalizeValue(%v) = %v,%v want %v,%v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestEdgeFromRecord(t *testing.T) {
	e, err := EdgeFromRecord("SENT", "acc_001", "txn_005")
	if err != nil {
		t.Fatal(err)
	}
	if e.ID != "acc_001:SENT:txn_005" || e.Source != "acc_001" || e.Target != "txn_005" {
		t.Fatalf("unexpected edge: %+v", e)
	}
	if _, err := EdgeFromRecord("KNOWS", "per_001", "per_002"); err == nil {
		t.Error("expected error for unknown type")
	}
	if _, err := EdgeFromRecord("SENT", nil, "txn_005"); err == nil {
		t.Error("expected error for nil endpoint")
	}
}

func TestSortSubgraph(t *testing.T) {
	g := Subgraph{
		Nodes: []Node{{ID: "per_002"}, {ID: "acc_001"}},
		Edges: []Edge{{ID: "z"}, {ID: "a"}},
	}
	sortSubgraph(&g)
	if g.Nodes[0].ID != "acc_001" || g.Edges[0].ID != "a" {
		t.Fatalf("not sorted: %+v", g)
	}
}
