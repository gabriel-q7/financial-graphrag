// Package http wires handlers and middlewares into the API router.
package http

import (
	"log/slog"
	"net/http"

	"github.com/gabrielaquino/financial-graphrag/backend/internal/http/handlers"
	"github.com/gabrielaquino/financial-graphrag/backend/internal/http/middleware"
)

// Deps are the handlers' dependencies.
type Deps struct {
	Health        handlers.HealthRunner
	Graph         *handlers.Graph
	Ask           *handlers.Ask   // nil disables POST /api/ask
	Admin         *handlers.Admin // nil disables admin routes
	AllowedOrigin string
	Log           *slog.Logger
}

// NewRouter builds the API handler.
func NewRouter(d Deps) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", handlers.Health(d.Health))
	mux.HandleFunc("GET /api/meta", handlers.Meta)
	mux.HandleFunc("GET /api/entities", d.Graph.Search)
	mux.HandleFunc("GET /api/graph/{id}", d.Graph.Entity)
	mux.HandleFunc("GET /api/graph/{id}/neighbors", d.Graph.Neighbors)
	if d.Ask != nil {
		mux.HandleFunc("POST /api/ask", d.Ask.Handle)
	}
	if d.Admin != nil {
		mux.HandleFunc("POST /api/admin/seed", d.Admin.RunSeed)
	}
	mux.HandleFunc("/", handlers.NotFound)

	return middleware.Chain(mux,
		middleware.Recover(d.Log),
		middleware.Logging(d.Log),
		middleware.SecurityHeaders,
		middleware.CORS(d.AllowedOrigin),
		middleware.MaxBody(1<<20),
	)
}
