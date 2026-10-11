package handlers

import (
	"context"
	"net/http"

	"github.com/gabrielaquino/financial-graphrag/backend/internal/health"
)

// HealthRunner produces a health report.
type HealthRunner interface {
	Run(ctx context.Context) health.Report
}

// Health serves GET /api/health: 200 when essential dependencies are up
// (status "ok" or "degraded"), 503 when an essential dependency is down.
func Health(checker HealthRunner) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		report := checker.Run(r.Context())
		status := http.StatusOK
		if report.Status == health.StatusUnavailable {
			status = http.StatusServiceUnavailable
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, status, report)
	}
}
