package graph

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/neo4j/neo4j-go-driver/v6/neo4j/dbtype"
)

// Node is the API representation of a graph node.
type Node struct {
	ID         string         `json:"id"`
	Type       NodeType       `json:"type"`
	Properties map[string]any `json:"properties"`
}

// Edge is the API representation of a relationship. Direction is Source -> Target.
type Edge struct {
	ID     string  `json:"id"`
	Type   RelType `json:"type"`
	Source string  `json:"source"`
	Target string  `json:"target"`
}

// Meta describes how a subgraph was bounded.
type Meta struct {
	Depth             int       `json:"depth"`
	NodeLimit         int       `json:"nodeLimit"`
	EdgeLimit         int       `json:"edgeLimit"`
	RelationshipTypes []RelType `json:"relationshipTypes"`
	NodeCount         int       `json:"nodeCount"`
	EdgeCount         int       `json:"edgeCount"`
	Truncated         bool      `json:"truncated"`
}

// Subgraph is the response for graph endpoints.
type Subgraph struct {
	Center Node   `json:"center"`
	Nodes  []Node `json:"nodes"`
	Edges  []Edge `json:"edges"`
	Meta   Meta   `json:"meta"`
}

// EdgeID builds a deterministic relationship id so clients can de-duplicate.
// The model has at most one relationship of a given type between two nodes.
func EdgeID(source string, t RelType, target string) string {
	return source + ":" + string(t) + ":" + target
}

// NodeFromRecord converts raw Neo4j values (id, labels, properties) into a Node,
// keeping only the properties allowed for the node type.
func NodeFromRecord(id any, labels any, props any) (Node, error) {
	sid, ok := id.(string)
	if !ok || ValidateID(sid) != nil {
		return Node{}, fmt.Errorf("node has invalid id %v", id)
	}
	labelList, ok := labels.([]any)
	if !ok {
		return Node{}, fmt.Errorf("node %s: labels must be a list", sid)
	}
	var spec NodeSpec
	found := false
	for _, l := range labelList {
		if s, ok := l.(string); ok {
			if spec, found = SpecFor(NodeType(s)); found {
				break
			}
		}
	}
	if !found {
		return Node{}, fmt.Errorf("node %s has no known domain label", sid)
	}
	propMap, _ := props.(map[string]any)
	out := Node{ID: sid, Type: spec.Type, Properties: map[string]any{}}
	for _, key := range spec.Properties {
		if v, ok := propMap[key]; ok {
			if nv, ok := normalizeValue(v); ok {
				out.Properties[key] = nv
			}
		}
	}
	return out, nil
}

// EdgeFromRecord converts raw Neo4j values into an Edge.
func EdgeFromRecord(relType, source, target any) (Edge, error) {
	t, _ := relType.(string)
	if _, ok := RelSpecFor(RelType(t)); !ok {
		return Edge{}, fmt.Errorf("unknown relationship type %v", relType)
	}
	s, ok1 := source.(string)
	d, ok2 := target.(string)
	if !ok1 || !ok2 || ValidateID(s) != nil || ValidateID(d) != nil {
		return Edge{}, fmt.Errorf("relationship %s has invalid endpoints", t)
	}
	return Edge{ID: EdgeID(s, RelType(t), d), Type: RelType(t), Source: s, Target: d}, nil
}

// normalizeValue converts driver values into JSON friendly values.
// Unsupported values (points, byte arrays, NaN) are dropped.
func normalizeValue(v any) (any, bool) {
	switch x := v.(type) {
	case nil:
		return nil, false
	case string, bool, int64:
		return x, true
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return nil, false
		}
		return x, true
	case time.Time:
		return x.UTC().Format(time.RFC3339), true
	case dbtype.Date:
		return x.String(), true
	case dbtype.LocalDateTime:
		return x.String(), true
	case dbtype.LocalTime:
		return x.String(), true
	case dbtype.Time:
		return x.String(), true
	case dbtype.Duration:
		return x.String(), true
	case []any:
		out := make([]any, 0, len(x))
		for _, item := range x {
			if nv, ok := normalizeValue(item); ok {
				out = append(out, nv)
			}
		}
		return out, true
	default:
		return nil, false
	}
}

// sortSubgraph gives responses a deterministic order.
func sortSubgraph(g *Subgraph) {
	slices.SortFunc(g.Nodes, func(a, b Node) int { return strings.Compare(a.ID, b.ID) })
	slices.SortFunc(g.Edges, func(a, b Edge) int { return strings.Compare(a.ID, b.ID) })
}
