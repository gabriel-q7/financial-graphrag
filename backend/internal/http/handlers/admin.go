package handlers

import (
	"context"
	"crypto/subtle"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// SeedFunc loads the synthetic dataset (and indexes it) and returns a JSON-serializable summary.
type SeedFunc func(ctx context.Context) (any, error)

// Admin serves local-development administrative endpoints.
// It is only registered when ADMIN_SEED_ENABLED=true and APP_ENV != production.
type Admin struct {
	Token   string
	Seed    SeedFunc
	Timeout time.Duration
	Log     *slog.Logger
}

// RunSeed serves POST /api/admin/seed. Requires the X-Admin-Token header and a loopback client.
func (h *Admin) RunSeed(w http.ResponseWriter, r *http.Request) {
	if !isLoopback(r.RemoteAddr) {
		writeError(w, http.StatusForbidden, "forbidden", "admin endpoints are only available from localhost")
		return
	}
	token := r.Header.Get("X-Admin-Token")
	if token == "" || subtle.ConstantTimeCompare([]byte(token), []byte(h.Token)) != 1 {
		writeError(w, http.StatusUnauthorized, "unauthorized", "missing or invalid admin token")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.Timeout)
	defer cancel()

	res, err := h.Seed(ctx)
	if err != nil {
		h.Log.Error("seed failed", "err", err)
		writeError(w, http.StatusInternalServerError, "seed_failed", "seed failed; check server logs")
		return
	}
	h.Log.Info("seed completed")
	writeJSON(w, http.StatusOK, res)
}

func isLoopback(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
