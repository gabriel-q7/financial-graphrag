// Package health checks the application dependencies.
package health

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/gabrielaquino/financial-graphrag/backend/internal/ollama"
)

// Status values used in the health report.
const (
	StatusOK          = "ok"
	StatusDegraded    = "degraded"
	StatusUnavailable = "unavailable"
)

// Pinger verifies a dependency is reachable (e.g. neo4j.Driver.VerifyConnectivity).
type Pinger interface {
	VerifyConnectivity(ctx context.Context) error
}

// OllamaAPI is the subset of the Ollama client used by health checks.
type OllamaAPI interface {
	Version(ctx context.Context) (string, error)
	Models(ctx context.Context) ([]string, error)
	ChatModel() string
	EmbedModel() string
}

// Check is the state of one dependency. Error messages are client-safe.
type Check struct {
	Status    string         `json:"status"`
	Essential bool           `json:"essential"`
	LatencyMS int64          `json:"latencyMs"`
	Error     string         `json:"error,omitempty"`
	Details   map[string]any `json:"details,omitempty"`
}

// Report is the /api/health payload.
type Report struct {
	Status    string           `json:"status"`
	Checks    map[string]Check `json:"checks"`
	Timestamp time.Time        `json:"timestamp"`
}

// Checker runs dependency checks with a per-check timeout.
// Neo4j is essential (503 when down); Ollama is optional at this stage (degraded).
type Checker struct {
	Neo4j   Pinger
	Ollama  OllamaAPI
	Timeout time.Duration
	Log     *slog.Logger
}

// Run executes all checks concurrently.
func (c *Checker) Run(ctx context.Context) Report {
	var (
		wg              sync.WaitGroup
		neo4jC, ollamaC Check
	)
	wg.Go(func() { neo4jC = c.checkNeo4j(ctx) })
	wg.Go(func() { ollamaC = c.checkOllama(ctx) })
	wg.Wait()

	r := Report{Status: StatusOK, Timestamp: time.Now().UTC(), Checks: map[string]Check{"neo4j": neo4jC, "ollama": ollamaC}}
	for _, ch := range r.Checks {
		switch {
		case ch.Status != StatusOK && ch.Essential:
			r.Status = StatusUnavailable
		case ch.Status != StatusOK && r.Status == StatusOK:
			r.Status = StatusDegraded
		}
	}
	return r
}

func (c *Checker) checkNeo4j(ctx context.Context) Check {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()
	start := time.Now()
	ch := Check{Status: StatusOK, Essential: true}
	if err := c.Neo4j.VerifyConnectivity(ctx); err != nil {
		c.Log.Warn("neo4j health check failed", "err", err)
		ch.Status, ch.Error = StatusUnavailable, "neo4j is unreachable"
	}
	ch.LatencyMS = time.Since(start).Milliseconds()
	return ch
}

func (c *Checker) checkOllama(ctx context.Context) Check {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()
	start := time.Now()
	ch := Check{Status: StatusOK, Essential: false}

	version, err := c.Ollama.Version(ctx)
	if err != nil {
		c.Log.Warn("ollama health check failed", "err", err)
		ch.Status, ch.Error = StatusUnavailable, "ollama is unreachable"
		ch.LatencyMS = time.Since(start).Milliseconds()
		return ch
	}
	models, err := c.Ollama.Models(ctx)
	if err != nil {
		c.Log.Warn("ollama model listing failed", "err", err)
		ch.Status, ch.Error = StatusDegraded, "could not list ollama models"
		ch.LatencyMS = time.Since(start).Milliseconds()
		return ch
	}
	chatOK := ollama.HasModel(models, c.Ollama.ChatModel())
	embedOK := ollama.HasModel(models, c.Ollama.EmbedModel())
	ch.Details = map[string]any{
		"version":        version,
		"chatModel":      map[string]any{"name": c.Ollama.ChatModel(), "available": chatOK},
		"embeddingModel": map[string]any{"name": c.Ollama.EmbedModel(), "available": embedOK},
	}
	if !chatOK || !embedOK {
		ch.Status, ch.Error = StatusDegraded, "configured models are not pulled yet (see README)"
	}
	ch.LatencyMS = time.Since(start).Milliseconds()
	return ch
}
