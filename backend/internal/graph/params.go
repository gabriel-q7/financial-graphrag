package graph

import (
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

// Server-side traversal limits. Clients can lower them but never exceed them.
const (
	DefaultDepth = 1
	MaxDepth     = 3
	DefaultLimit = 25
	MaxLimit     = 100
	// EdgeLimitFactor bounds relationships relative to the node limit.
	EdgeLimitFactor = 3

	DefaultSearchLimit = 20
	MaxSearchLimit     = 50
	MaxSearchQueryLen  = 100
)

// NeighborQuery is a validated neighborhood request.
type NeighborQuery struct {
	ID                string
	Depth             int
	Limit             int
	RelationshipTypes []RelType
}

// EdgeLimit is the maximum number of relationships returned for the query.
func (q NeighborQuery) EdgeLimit() int { return q.Limit * EdgeLimitFactor }

// ValidationError carries a client-safe message for a 400 response.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Message }

// ParseNeighborQuery validates id and query string parameters
// (depth, limit, relationshipTypes) and applies defaults and server limits.
func ParseNeighborQuery(id string, v url.Values) (NeighborQuery, error) {
	q := NeighborQuery{ID: id, Depth: DefaultDepth, Limit: DefaultLimit}
	if err := ValidateID(id); err != nil {
		return q, &ValidationError{"id", "must match <prefix>_<digits>, e.g. per_001"}
	}

	var err error
	if q.Depth, err = intParam(v, "depth", DefaultDepth, 1, MaxDepth); err != nil {
		return q, err
	}
	if q.Limit, err = intParam(v, "limit", DefaultLimit, 1, MaxLimit); err != nil {
		return q, err
	}
	if q.RelationshipTypes, err = relTypesParam(v, "relationshipTypes"); err != nil {
		return q, err
	}
	return q, nil
}

// SearchQuery is a validated entity search request.
type SearchQuery struct {
	Text  string
	Type  NodeType
	Limit int
}

// ParseSearchQuery validates q, type and limit.
func ParseSearchQuery(v url.Values) (SearchQuery, error) {
	s := SearchQuery{Text: strings.TrimSpace(v.Get("q"))}
	if len(s.Text) > MaxSearchQueryLen {
		return s, &ValidationError{"q", fmt.Sprintf("must have at most %d characters", MaxSearchQueryLen)}
	}
	if t := v.Get("type"); t != "" {
		if _, ok := SpecFor(NodeType(t)); !ok {
			return s, &ValidationError{"type", "unknown entity type"}
		}
		s.Type = NodeType(t)
	}
	var err error
	s.Limit, err = intParam(v, "limit", DefaultSearchLimit, 1, MaxSearchLimit)
	return s, err
}

func intParam(v url.Values, name string, def, lo, hi int) (int, error) {
	raw := v.Get(name)
	if raw == "" {
		return def, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < lo || n > hi {
		return 0, &ValidationError{name, fmt.Sprintf("must be an integer between %d and %d", lo, hi)}
	}
	return n, nil
}

// relTypesParam accepts both repeated (?relationshipTypes=A&relationshipTypes=B)
// and comma separated (?relationshipTypes=A,B) values.
func relTypesParam(v url.Values, name string) ([]RelType, error) {
	var out []RelType
	for _, raw := range v[name] {
		for part := range strings.SplitSeq(raw, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			t := RelType(part)
			if _, ok := RelSpecFor(t); !ok {
				return nil, &ValidationError{name, fmt.Sprintf("unknown relationship type %q", part)}
			}
			if !slices.Contains(out, t) {
				out = append(out, t)
			}
		}
	}
	return out, nil
}
