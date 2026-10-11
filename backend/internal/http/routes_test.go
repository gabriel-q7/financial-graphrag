package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gabrielaquino/financial-graphrag/backend/internal/graph"
	"github.com/gabrielaquino/financial-graphrag/backend/internal/health"
	"github.com/gabrielaquino/financial-graphrag/backend/internal/http/handlers"
	"github.com/gabrielaquino/financial-graphrag/backend/internal/seed"
)

const origin = "http://localhost:5173"
const adminToken = "0123456789abcdef-test"

type fakeReader struct {
	err    error
	lastQ  graph.NeighborQuery
	called bool
	search []graph.Node
}

func (f *fakeReader) Neighborhood(_ context.Context, q graph.NeighborQuery) (graph.Subgraph, error) {
	f.called, f.lastQ = true, q
	if f.err != nil {
		return graph.Subgraph{}, f.err
	}
	c := graph.Node{ID: q.ID, Type: graph.Person, Properties: map[string]any{"name": "Ana"}}
	return graph.Subgraph{Center: c, Nodes: []graph.Node{c}, Edges: []graph.Edge{}, Meta: graph.Meta{Depth: q.Depth, NodeLimit: q.Limit}}, nil
}

func (f *fakeReader) Search(context.Context, graph.SearchQuery) ([]graph.Node, error) {
	return f.search, f.err
}

type fakeHealth struct{ status string }

func (f fakeHealth) Run(context.Context) health.Report {
	return health.Report{Status: f.status, Checks: map[string]health.Check{}}
}

func newTestRouter(reader *fakeReader, healthStatus string, withAdmin bool, seedErr error) http.Handler {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	d := Deps{
		Health:        fakeHealth{healthStatus},
		Graph:         &handlers.Graph{Reader: reader, Timeout: time.Second, Log: log},
		AllowedOrigin: origin,
		Log:           log,
	}
	if withAdmin {
		d.Admin = &handlers.Admin{Token: adminToken, Timeout: time.Second, Log: log,
			Seed: func(context.Context) (seed.Result, error) { return seed.Result{Nodes: 3}, seedErr }}
	}
	return NewRouter(d)
}

func do(h http.Handler, method, target string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	req.RemoteAddr = "127.0.0.1:50000"
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body handlers.ErrorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid error body %q: %v", rec.Body.String(), err)
	}
	return body.Error.Code
}

func TestHealthStatusCodes(t *testing.T) {
	for status, want := range map[string]int{
		health.StatusOK: 200, health.StatusDegraded: 200, health.StatusUnavailable: 503,
	} {
		rec := do(newTestRouter(&fakeReader{}, status, false, nil), "GET", "/api/health", nil)
		if rec.Code != want {
			t.Errorf("%s: got %d want %d", status, rec.Code, want)
		}
	}
}

func TestGraphEntity(t *testing.T) {
	r := &fakeReader{}
	rec := do(newTestRouter(r, "ok", false, nil), "GET", "/api/graph/per_001?depth=3&limit=10", nil)
	if rec.Code != 200 {
		t.Fatalf("got %d: %s", rec.Code, rec.Body)
	}
	if r.lastQ.Depth != 1 || r.lastQ.Limit != 10 {
		t.Fatalf("entity endpoint must force depth=1 and honor limit: %+v", r.lastQ)
	}
	var g graph.Subgraph
	if err := json.Unmarshal(rec.Body.Bytes(), &g); err != nil || g.Center.ID != "per_001" {
		t.Fatalf("bad body: %s", rec.Body)
	}
}

func TestGraphNeighborsValidation(t *testing.T) {
	cases := []struct {
		target string
		code   int
	}{
		{"/api/graph/per_001/neighbors?depth=2&limit=50&relationshipTypes=OWNS,SENT", 200},
		{"/api/graph/per_001/neighbors?depth=99", 400},
		{"/api/graph/per_001/neighbors?limit=100000", 400},
		{"/api/graph/per_001/neighbors?relationshipTypes=DELETE", 400},
		{"/api/graph/not-an-id/neighbors", 400},
		{"/api/graph/per_001%27%20OR%201=1/neighbors", 400},
	}
	for _, c := range cases {
		r := &fakeReader{}
		rec := do(newTestRouter(r, "ok", false, nil), "GET", c.target, nil)
		if rec.Code != c.code {
			t.Errorf("%s: got %d want %d (%s)", c.target, rec.Code, c.code, rec.Body)
		}
		if c.code == 400 && r.called {
			t.Errorf("%s: store must not be called for invalid input", c.target)
		}
	}
}

func TestGraphErrorsAreSanitized(t *testing.T) {
	cases := []struct {
		err  error
		code int
		name string
	}{
		{graph.ErrNotFound, 404, "not_found"},
		{errors.New("ConnectivityError: bolt://neo4j:secret@db"), 503, "graph_unavailable"},
		{context.DeadlineExceeded, 504, "timeout"},
	}
	for _, c := range cases {
		rec := do(newTestRouter(&fakeReader{err: c.err}, "ok", false, nil), "GET", "/api/graph/per_001", nil)
		if rec.Code != c.code || errorCode(t, rec) != c.name {
			t.Errorf("%v: got %d %s", c.err, rec.Code, rec.Body)
		}
		if strings.Contains(rec.Body.String(), "secret") {
			t.Errorf("internal error leaked: %s", rec.Body)
		}
	}
}

func TestSearch(t *testing.T) {
	r := &fakeReader{search: []graph.Node{{ID: "per_001", Type: graph.Person}}}
	rec := do(newTestRouter(r, "ok", false, nil), "GET", "/api/entities?q=ana&type=Person", nil)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "per_001") {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
	rec = do(newTestRouter(r, "ok", false, nil), "GET", "/api/entities?type=Robot", nil)
	if rec.Code != 400 {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestCORS(t *testing.T) {
	h := newTestRouter(&fakeReader{}, "ok", false, nil)
	rec := do(h, "OPTIONS", "/api/graph/per_001", map[string]string{"Origin": origin, "Access-Control-Request-Method": "GET"})
	if rec.Code != 204 || rec.Header().Get("Access-Control-Allow-Origin") != origin {
		t.Fatalf("preflight: %d %v", rec.Code, rec.Header())
	}
	rec = do(h, "GET", "/api/meta", map[string]string{"Origin": "https://evil.example"})
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("foreign origin must not be allowed")
	}
}

func TestAdminSeed(t *testing.T) {
	// Disabled: route does not exist.
	rec := do(newTestRouter(&fakeReader{}, "ok", false, nil), "POST", "/api/admin/seed", map[string]string{"X-Admin-Token": adminToken})
	if rec.Code != 404 {
		t.Fatalf("disabled admin: got %d", rec.Code)
	}

	h := newTestRouter(&fakeReader{}, "ok", true, nil)
	if rec := do(h, "POST", "/api/admin/seed", nil); rec.Code != 401 {
		t.Fatalf("missing token: got %d", rec.Code)
	}
	if rec := do(h, "POST", "/api/admin/seed", map[string]string{"X-Admin-Token": "wrong"}); rec.Code != 401 {
		t.Fatalf("wrong token: got %d", rec.Code)
	}
	if rec := do(h, "POST", "/api/admin/seed", map[string]string{"X-Admin-Token": adminToken}); rec.Code != 200 {
		t.Fatalf("valid token: got %d %s", rec.Code, rec.Body)
	}

	req := httptest.NewRequest("POST", "/api/admin/seed", nil)
	req.RemoteAddr = "10.0.0.5:4000"
	req.Header.Set("X-Admin-Token", adminToken)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 403 {
		t.Fatalf("non-loopback: got %d", rec.Code)
	}

	failing := newTestRouter(&fakeReader{}, "ok", true, errors.New("neo4j password=x"))
	rec = do(failing, "POST", "/api/admin/seed", map[string]string{"X-Admin-Token": adminToken})
	if rec.Code != 500 || strings.Contains(rec.Body.String(), "password") {
		t.Fatalf("seed error: %d %s", rec.Code, rec.Body)
	}
}

func TestUnknownRouteAndMeta(t *testing.T) {
	h := newTestRouter(&fakeReader{}, "ok", false, nil)
	if rec := do(h, "GET", "/api/nope", nil); rec.Code != 404 || errorCode(t, rec) != "not_found" {
		t.Fatalf("unknown route: %d %s", rec.Code, rec.Body)
	}
	rec := do(h, "GET", "/api/meta", nil)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "BankAccount") || rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("meta: %d %s", rec.Code, rec.Body)
	}
}
