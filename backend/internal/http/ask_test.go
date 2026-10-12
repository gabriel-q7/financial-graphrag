package http

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gabrielaquino/financial-graphrag/backend/internal/graph"
	"github.com/gabrielaquino/financial-graphrag/backend/internal/http/handlers"
	"github.com/gabrielaquino/financial-graphrag/backend/internal/rag"
)

type fakeAsker struct {
	retrieveErr error
	genErr      error
	noEvidence  bool
	block       chan struct{}
}

func (f *fakeAsker) Retrieve(ctx context.Context, q string) (*rag.Retrieval, error) {
	if f.block != nil {
		<-f.block
	}
	if f.retrieveErr != nil {
		return nil, f.retrieveErr
	}
	return &rag.Retrieval{Question: q, NoEvidence: f.noEvidence, Entities: []graph.Node{}, Documents: []rag.DocumentHit{},
		Nodes: []graph.Node{}, Edges: []graph.Edge{}}, nil
}

func (f *fakeAsker) Generate(ctx context.Context, r *rag.Retrieval, on func(string) error) (rag.Answer, error) {
	if f.genErr != nil {
		return rag.Answer{}, f.genErr
	}
	_ = on("Olá ")
	_ = on("[acc_001]")
	return rag.Answer{Text: "Olá [acc_001]", NoEvidence: r.NoEvidence}, nil
}

func askRouter(a *fakeAsker) http.Handler {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewRouter(Deps{
		Health: fakeHealth{"ok"}, Graph: &handlers.Graph{Reader: &fakeReader{}, Timeout: time.Second, Log: log},
		Ask:           &handlers.Ask{Service: a, Timeout: 5 * time.Second, Log: log},
		AllowedOrigin: origin, Log: log,
	})
}

func postAsk(h http.Handler, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", "/api/ask", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

var eventRe = regexp.MustCompile(`(?m)^event: (\w+)$`)

func events(body string) []string {
	var out []string
	for _, m := range eventRe.FindAllStringSubmatch(body, -1) {
		out = append(out, m[1])
	}
	return out
}

func TestAskStreamsEventsInOrder(t *testing.T) {
	rec := postAsk(askRouter(&fakeAsker{}), `{"question":"Por que a Ana?"}`)
	if rec.Code != 200 || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/event-stream") {
		t.Fatalf("got %d %v", rec.Code, rec.Header())
	}
	got := strings.Join(events(rec.Body.String()), ",")
	if got != "retrieval,status,token,token,done" {
		t.Fatalf("events = %s\n%s", got, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), `data: {"text":"[acc_001]"}`) {
		t.Fatalf("token payload missing:\n%s", rec.Body)
	}
}

func TestAskNoEvidenceHasNoStatusEvent(t *testing.T) {
	rec := postAsk(askRouter(&fakeAsker{noEvidence: true}), `{"question":"capital da França?"}`)
	if got := strings.Join(events(rec.Body.String()), ","); got != "retrieval,token,token,done" {
		t.Fatalf("events = %s", got)
	}
}

func TestAskValidation(t *testing.T) {
	h := askRouter(&fakeAsker{})
	for _, body := range []string{``, `{}`, `{"question":"oi"}`, `{"question":"` + strings.Repeat("a", 501) + `"}`, `{"question":"ok ok","x":1}`, `[1]`} {
		if rec := postAsk(h, body); rec.Code != 400 || errorCode(t, rec) != "invalid_parameter" {
			t.Errorf("%q: got %d %s", body, rec.Code, rec.Body)
		}
	}
}

func TestAskErrorsBeforeStream(t *testing.T) {
	cases := []struct {
		err  error
		code int
		name string
	}{
		{rag.ErrEmbedding, 503, "model_unavailable"},
		{errors.New("neo4j down password=x"), 503, "graph_unavailable"},
		{context.DeadlineExceeded, 504, "timeout"},
	}
	for _, c := range cases {
		rec := postAsk(askRouter(&fakeAsker{retrieveErr: c.err}), `{"question":"pergunta"}`)
		if rec.Code != c.code || errorCode(t, rec) != c.name || strings.Contains(rec.Body.String(), "password") {
			t.Errorf("%v: got %d %s", c.err, rec.Code, rec.Body)
		}
	}
}

func TestAskGenerationErrorEvent(t *testing.T) {
	rec := postAsk(askRouter(&fakeAsker{genErr: rag.ErrGeneration}), `{"question":"pergunta"}`)
	if got := strings.Join(events(rec.Body.String()), ","); got != "retrieval,status,error" {
		t.Fatalf("events = %s", got)
	}
}

func TestAskOneAtATime(t *testing.T) {
	a := &fakeAsker{block: make(chan struct{})}
	h := askRouter(a)
	done := make(chan *httptest.ResponseRecorder)
	go func() { done <- postAsk(h, `{"question":"primeira"}`) }()
	time.Sleep(50 * time.Millisecond) // first request holds the slot inside Retrieve
	if rec := postAsk(h, `{"question":"segunda"}`); rec.Code != 429 || errorCode(t, rec) != "busy" {
		t.Fatalf("expected 429 busy, got %d %s", rec.Code, rec.Body)
	}
	close(a.block)
	if rec := <-done; rec.Code != 200 {
		t.Fatalf("first request: %d", rec.Code)
	}
	if rec := postAsk(h, `{"question":"terceira"}`); rec.Code != 200 {
		t.Fatalf("slot not released: %d", rec.Code)
	}
}
