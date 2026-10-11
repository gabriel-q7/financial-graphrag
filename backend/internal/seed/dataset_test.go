package seed

import (
	"strings"
	"testing"

	"github.com/gabrielaquino/financial-graphrag/backend/internal/graph"
)

func TestRepositoryDatasetIsValid(t *testing.T) {
	d, err := LoadFile("../../../data/seed/financial_network.json")
	if err != nil {
		t.Fatalf("seed file invalid: %v", err)
	}
	counts := map[graph.NodeType]int{}
	for _, n := range d.Nodes {
		counts[n.Label]++
	}
	for _, s := range graph.NodeSpecs {
		if counts[s.Type] == 0 {
			t.Errorf("dataset has no %s nodes", s.Type)
		}
	}
	rels := map[graph.RelType]int{}
	for _, r := range d.Relationships {
		rels[r.Type]++
	}
	for _, s := range graph.RelSpecs {
		if rels[s.Type] == 0 {
			t.Errorf("dataset has no %s relationships", s.Type)
		}
	}
}

func TestEveryLabelAndRelTypeHasQuery(t *testing.T) {
	for _, s := range graph.NodeSpecs {
		if nodeQueries[s.Type] == "" {
			t.Errorf("missing node query for %s", s.Type)
		}
	}
	for _, s := range graph.RelSpecs {
		if !strings.Contains(relQueries[s.Type], "[:"+string(s.Type)+"]") {
			t.Errorf("missing relationship query for %s", s.Type)
		}
	}
}

func TestParseRejectsInvalidData(t *testing.T) {
	cases := map[string]string{
		"unknown field": `{"nodes":[{"id":"per_001","label":"Person","properties":{"name":"A"},"extra":1}]}`,
		"bad prefix":    `{"nodes":[{"id":"cmp_001","label":"Person","properties":{"name":"A"}}]}`,
		"bad property":  `{"nodes":[{"id":"per_001","label":"Person","properties":{"name":"A","cpf":"x"}}]}`,
		"missing prop":  `{"nodes":[{"id":"per_001","label":"Person","properties":{}}]}`,
		"duplicate id":  `{"nodes":[{"id":"per_001","label":"Person","properties":{"name":"A"}},{"id":"per_001","label":"Person","properties":{"name":"B"}}]}`,
		"bad amount":    `{"nodes":[{"id":"txn_001","label":"Transaction","properties":{"amount":"10","currency":"BRL","status":"ok","timestamp":"2026-01-01T00:00:00Z"}}]}`,
		"bad timestamp": `{"nodes":[{"id":"txn_001","label":"Transaction","properties":{"amount":10,"currency":"BRL","status":"ok","timestamp":"yesterday"}}]}`,
		"wrong direction": `{"nodes":[{"id":"per_001","label":"Person","properties":{"name":"A"}},{"id":"acc_001","label":"BankAccount","properties":{"bank":"B","status":"active"}}],
			"relationships":[{"type":"OWNS","from":"acc_001","to":"per_001"}]}`,
		"missing endpoint": `{"nodes":[{"id":"per_001","label":"Person","properties":{"name":"A"}}],
			"relationships":[{"type":"OWNS","from":"per_001","to":"acc_999"}]}`,
		"unknown rel": `{"nodes":[{"id":"per_001","label":"Person","properties":{"name":"A"}}],
			"relationships":[{"type":"KNOWS","from":"per_001","to":"per_001"}]}`,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse([]byte(raw)); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}
