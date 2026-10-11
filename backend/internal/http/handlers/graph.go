package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gabrielaquino/financial-graphrag/backend/internal/graph"
)

// Graph serves the graph exploration endpoints.
type Graph struct {
	Reader  graph.Reader
	Timeout time.Duration
	Log     *slog.Logger
}

// Entity serves GET /api/graph/{id}: the entity and its direct connections.
// Only "limit" is accepted; depth is fixed at 1 and all relationship types are included.
func (h *Graph) Entity(w http.ResponseWriter, r *http.Request) {
	v := r.URL.Query()
	v.Del("depth")
	v.Del("relationshipTypes")
	h.neighborhood(w, r, v)
}

// Neighbors serves GET /api/graph/{id}/neighbors with depth, limit and relationshipTypes.
func (h *Graph) Neighbors(w http.ResponseWriter, r *http.Request) {
	h.neighborhood(w, r, r.URL.Query())
}

func (h *Graph) neighborhood(w http.ResponseWriter, r *http.Request, v map[string][]string) {
	q, err := graph.ParseNeighborQuery(r.PathValue("id"), v)
	if err != nil {
		writeValidationError(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.Timeout)
	defer cancel()

	g, err := h.Reader.Neighborhood(ctx, q)
	switch {
	case errors.Is(err, graph.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "entity not found")
	case err != nil:
		h.storeError(w, r, err)
	default:
		writeJSON(w, http.StatusOK, g)
	}
}

// Search serves GET /api/entities?q=&type=&limit= to pick a starting entity.
func (h *Graph) Search(w http.ResponseWriter, r *http.Request) {
	q, err := graph.ParseSearchQuery(r.URL.Query())
	if err != nil {
		writeValidationError(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.Timeout)
	defer cancel()

	nodes, err := h.Reader.Search(ctx, q)
	if err != nil {
		h.storeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": nodes, "limit": q.Limit})
}

// Meta serves GET /api/meta: schema and server-side limits for the UI.
func Meta(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"nodeTypes":         graph.NodeSpecs,
		"relationshipTypes": graph.RelSpecs,
		"limits": map[string]int{
			"defaultDepth":    graph.DefaultDepth,
			"maxDepth":        graph.MaxDepth,
			"defaultLimit":    graph.DefaultLimit,
			"maxLimit":        graph.MaxLimit,
			"edgeLimitFactor": graph.EdgeLimitFactor,
		},
	})
}

func (h *Graph) storeError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, context.DeadlineExceeded) {
		h.Log.Warn("graph query timed out", "path", r.URL.Path)
		writeError(w, http.StatusGatewayTimeout, "timeout", "graph query timed out")
		return
	}
	h.Log.Error("graph query failed", "path", r.URL.Path, "err", err)
	writeError(w, http.StatusServiceUnavailable, "graph_unavailable", "graph database is unavailable")
}

func writeValidationError(w http.ResponseWriter, err error) {
	var ve *graph.ValidationError
	if errors.As(err, &ve) {
		writeJSON(w, http.StatusBadRequest, ErrorBody{Error: ErrorDetail{Code: "invalid_parameter", Message: ve.Message, Field: ve.Field}})
		return
	}
	writeError(w, http.StatusBadRequest, "invalid_parameter", "invalid request")
}
