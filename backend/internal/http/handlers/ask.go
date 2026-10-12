package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gabrielaquino/financial-graphrag/backend/internal/rag"
)

// Asker is the GraphRAG pipeline used by the ask endpoint.
type Asker interface {
	Retrieve(ctx context.Context, question string) (*rag.Retrieval, error)
	Generate(ctx context.Context, r *rag.Retrieval, onToken func(string) error) (rag.Answer, error)
}

// Ask serves POST /api/ask as a Server-Sent Events stream:
//
//	event: retrieval  evidence found (entities, documents, nodes, edges)
//	event: status     {"phase":"generating"} before the model starts
//	event: token      {"text": "..."} for every generated fragment
//	event: done       final answer, citations and timings
//	event: error      {"code","message"} if generation fails after the stream started
//
// Errors before the stream starts are regular JSON errors (400, 429, 503, 504).
// Only one question runs at a time: generation saturates the CPU.
type Ask struct {
	Service   Asker
	Timeout   time.Duration
	Heartbeat time.Duration
	Log       *slog.Logger
	slot      chan struct{}
	once      sync.Once
}

const minQuestionChars = 3

func (h *Ask) acquire() bool {
	h.once.Do(func() { h.slot = make(chan struct{}, 1) })
	select {
	case h.slot <- struct{}{}:
		return true
	default:
		return false
	}
}

func (h *Ask) release() { <-h.slot }

// Handle serves the request.
func (h *Ask) Handle(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Question string `json:"question"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorBody{Error: ErrorDetail{Code: "invalid_parameter", Message: "body must be {\"question\": string}", Field: "question"}})
		return
	}
	q := strings.TrimSpace(body.Question)
	if n := utf8.RuneCountInString(q); n < minQuestionChars || n > rag.MaxQuestionChars {
		writeJSON(w, http.StatusBadRequest, ErrorBody{Error: ErrorDetail{Code: "invalid_parameter",
			Message: fmt.Sprintf("question must have between %d and %d characters", minQuestionChars, rag.MaxQuestionChars), Field: "question"}})
		return
	}

	if !h.acquire() {
		writeError(w, http.StatusTooManyRequests, "busy", "another question is being answered; try again when it finishes")
		return
	}
	defer h.release()

	ctx, cancel := context.WithTimeout(r.Context(), h.Timeout)
	defer cancel()

	retrieval, err := h.Service.Retrieve(ctx, q)
	switch {
	case errors.Is(err, rag.ErrEmbedding):
		h.Log.Warn("ask: embedding failed", "err", err)
		writeError(w, http.StatusServiceUnavailable, "model_unavailable", "the embedding model is unavailable (is Ollama running with the configured models?)")
		return
	case errors.Is(err, context.DeadlineExceeded):
		writeError(w, http.StatusGatewayTimeout, "timeout", "retrieval timed out")
		return
	case err != nil:
		h.Log.Error("ask: retrieval failed", "err", err)
		writeError(w, http.StatusServiceUnavailable, "graph_unavailable", "graph database is unavailable")
		return
	}

	// Generation on CPU can take minutes: extend the server write deadline for this stream.
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Now().Add(h.Timeout + 30*time.Second))
	stream := &sseStream{w: w, rc: rc}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	if err := stream.send("retrieval", retrieval); err != nil {
		return
	}
	stopHeartbeat := stream.heartbeat(h.Heartbeat)
	defer stopHeartbeat()

	if !retrieval.NoEvidence {
		if err := stream.send("status", map[string]string{"phase": "generating"}); err != nil {
			return
		}
	}
	answer, err := h.Service.Generate(ctx, retrieval, func(tok string) error {
		return stream.send("token", map[string]string{"text": tok})
	})
	switch {
	case r.Context().Err() != nil:
		h.Log.Info("ask: client disconnected; generation cancelled")
	case errors.Is(err, context.DeadlineExceeded):
		_ = stream.send("error", ErrorDetail{Code: "timeout", Message: "generation timed out"})
	case err != nil:
		h.Log.Error("ask: generation failed", "err", err)
		_ = stream.send("error", ErrorDetail{Code: "model_unavailable", Message: "the chat model failed; check that Ollama is running"})
	default:
		h.Log.Info("ask: answered", "evidenceNodes", len(retrieval.Nodes), "documents", len(retrieval.Documents),
			"citations", len(answer.Citations.Valid), "invalidCitations", len(answer.Citations.Invalid),
			"tokens", answer.Stats.GeneratedTokens, "durationMs", answer.DurationMS)
		_ = stream.send("done", answer)
	}
}

// sseStream serializes writes from the handler and the heartbeat goroutine.
type sseStream struct {
	mu sync.Mutex
	w  http.ResponseWriter
	rc *http.ResponseController
}

func (s *sseStream) send(event string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return s.write(fmt.Sprintf("event: %s\ndata: %s\n\n", event, data))
}

func (s *sseStream) write(chunk string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := fmt.Fprint(s.w, chunk); err != nil {
		return err
	}
	return s.rc.Flush()
}

// heartbeat sends SSE comments while the model evaluates the prompt (no tokens yet),
// so proxies and clients do not consider the connection idle.
func (s *sseStream) heartbeat(every time.Duration) (stop func()) {
	if every <= 0 {
		return func() {}
	}
	done := make(chan struct{})
	go func() {
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
				_ = s.write(": keep-alive\n\n")
			}
		}
	}()
	return func() { close(done) }
}
