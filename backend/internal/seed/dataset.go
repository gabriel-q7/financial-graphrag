// Package seed loads the synthetic financial dataset into Neo4j idempotently.
package seed

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"time"

	"github.com/gabrielaquino/financial-graphrag/backend/internal/graph"
)

// Dataset is the on-disk seed format (data/seed/*.json).
type Dataset struct {
	Version       string         `json:"version"`
	Description   string         `json:"description"`
	Nodes         []NodeRecord   `json:"nodes"`
	Relationships []Relationship `json:"relationships"`
}

// NodeRecord is one node in the seed file.
type NodeRecord struct {
	ID         string         `json:"id"`
	Label      graph.NodeType `json:"label"`
	Properties map[string]any `json:"properties"`
}

// Relationship is one directed relationship in the seed file.
type Relationship struct {
	Type graph.RelType `json:"type"`
	From string        `json:"from"`
	To   string        `json:"to"`
}

// LoadFile reads and validates a dataset file.
func LoadFile(path string) (Dataset, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Dataset{}, fmt.Errorf("read seed file: %w", err)
	}
	return Parse(raw)
}

// Parse decodes and validates a dataset. Unknown JSON fields are rejected.
func Parse(raw []byte) (Dataset, error) {
	var d Dataset
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	dec.UseNumber()
	if err := dec.Decode(&d); err != nil {
		return Dataset{}, fmt.Errorf("decode seed file: %w", err)
	}
	return d, d.Validate()
}

// Validate checks ids, labels, properties and relationship direction.
func (d Dataset) Validate() error {
	var errs []error
	types := map[string]graph.NodeType{}

	for _, n := range d.Nodes {
		spec, ok := graph.SpecFor(n.Label)
		if !ok {
			errs = append(errs, fmt.Errorf("node %s: unknown label %q", n.ID, n.Label))
			continue
		}
		if t, ok := graph.TypeForID(n.ID); !ok || t != n.Label {
			errs = append(errs, fmt.Errorf("node %s: id must use prefix %q_", n.ID, spec.IDPrefix))
			continue
		}
		if _, dup := types[n.ID]; dup {
			errs = append(errs, fmt.Errorf("node %s: duplicate id", n.ID))
			continue
		}
		types[n.ID] = n.Label
		for k := range n.Properties {
			if !slices.Contains(spec.Properties, k) {
				errs = append(errs, fmt.Errorf("node %s: property %q not allowed for %s", n.ID, k, n.Label))
			}
		}
		for _, k := range spec.Properties {
			if err := validateProperty(n.Label, k, n.Properties[k]); err != nil {
				errs = append(errs, fmt.Errorf("node %s: %w", n.ID, err))
			}
		}
	}

	seen := map[string]bool{}
	for _, r := range d.Relationships {
		spec, ok := graph.RelSpecFor(r.Type)
		if !ok {
			errs = append(errs, fmt.Errorf("relationship %s->%s: unknown type %q", r.From, r.To, r.Type))
			continue
		}
		from, okFrom := types[r.From]
		to, okTo := types[r.To]
		if !okFrom || !okTo {
			errs = append(errs, fmt.Errorf("relationship %s %s->%s: endpoint not found", r.Type, r.From, r.To))
			continue
		}
		if !slices.Contains(spec.From, from) || !slices.Contains(spec.To, to) {
			errs = append(errs, fmt.Errorf("relationship %s %s->%s: invalid direction %s->%s", r.Type, r.From, r.To, from, to))
		}
		key := graph.EdgeID(r.From, r.Type, r.To)
		if seen[key] {
			errs = append(errs, fmt.Errorf("relationship %s: duplicate", key))
		}
		seen[key] = true
	}
	return errors.Join(errs...)
}

func validateProperty(label graph.NodeType, key string, v any) error {
	switch {
	case key == "amount":
		n, ok := v.(json.Number)
		if !ok {
			return fmt.Errorf("amount must be a number")
		}
		f, err := n.Float64()
		if err != nil || f <= 0 {
			return fmt.Errorf("amount must be positive")
		}
	case key == "timestamp" || key == "created_at":
		s, ok := v.(string)
		if _, err := time.Parse(time.RFC3339, s); !ok || err != nil {
			return fmt.Errorf("%s must be an RFC3339 timestamp", key)
		}
	default:
		s, ok := v.(string)
		if !ok || s == "" {
			return fmt.Errorf("%s.%s must be a non-empty string", label, key)
		}
	}
	return nil
}
