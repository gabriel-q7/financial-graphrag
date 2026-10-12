package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gabrielaquino/financial-graphrag/backend/internal/rag"
)

// fakeOllama answers /api/chat (streaming NDJSON) and /api/embed like Ollama does.
func fakeOllama(t *testing.T, chatStatus int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch r.URL.Path {
		case "/api/chat":
			if chatStatus != http.StatusOK {
				http.Error(w, `{"error":"model not found"}`, chatStatus)
				return
			}
			opts, _ := body["options"].(map[string]any)
			if body["model"] != "chat-model" || opts["num_predict"] != float64(50) || opts["num_ctx"] != float64(4096) {
				t.Errorf("unexpected chat request: %v", body)
			}
			msgs, _ := body["messages"].([]any)
			if len(msgs) != 2 || msgs[0].(map[string]any)["role"] != "system" {
				t.Errorf("unexpected messages: %v", msgs)
			}
			for _, line := range []string{
				`{"model":"chat-model","message":{"role":"assistant","content":"Olá "},"done":false}`,
				`{"model":"chat-model","message":{"role":"assistant","content":"[acc_001]"},"done":false}`,
				`{"model":"chat-model","message":{"role":"assistant","content":""},"done":true,"prompt_eval_count":120,"eval_count":2}`,
			} {
				_, _ = w.Write([]byte(line + "\n"))
				w.(http.Flusher).Flush()
			}
		case "/api/embed":
			if body["model"] != "embed-model" {
				t.Errorf("unexpected embed model: %v", body["model"])
			}
			_, _ = w.Write([]byte(`{"embeddings":[[0.1,0.2,0.3]]}`))
		default:
			http.NotFound(w, r)
		}
	}))
}

func newTestModels(t *testing.T, url string) (*Generator, *Embedder) {
	t.Helper()
	g, e, err := NewOllama(Config{OllamaURL: url, ChatModel: "chat-model", EmbedModel: "embed-model", NumCtx: 4096})
	if err != nil {
		t.Fatal(err)
	}
	return g, e
}

func TestGenerateStreamsAndReportsStats(t *testing.T) {
	srv := fakeOllama(t, http.StatusOK)
	defer srv.Close()
	g, _ := newTestModels(t, srv.URL)

	var got strings.Builder
	st, err := g.Generate(context.Background(), "sistema", "pergunta", rag.GenerationOptions{MaxTokens: 50, Temperature: 0.1},
		func(tok string) error { got.WriteString(tok); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if got.String() != "Olá [acc_001]" {
		t.Fatalf("streamed %q", got.String())
	}
	if st.Model != "chat-model" || st.PromptTokens != 120 || st.GeneratedTokens != 2 {
		t.Fatalf("stats: %+v", st)
	}
}

func TestGenerateErrors(t *testing.T) {
	srv := fakeOllama(t, http.StatusNotFound)
	defer srv.Close()
	g, _ := newTestModels(t, srv.URL)
	if _, err := g.Generate(context.Background(), "s", "u", rag.GenerationOptions{MaxTokens: 50}, func(string) error { return nil }); err == nil {
		t.Fatal("expected error from server")
	}

	ok := fakeOllama(t, http.StatusOK)
	defer ok.Close()
	g, _ = newTestModels(t, ok.URL)
	stop := errors.New("client gone")
	if _, err := g.Generate(context.Background(), "s", "u", rag.GenerationOptions{MaxTokens: 50}, func(string) error { return stop }); err == nil {
		t.Fatal("expected callback error to abort generation")
	}
}

func TestEmbedder(t *testing.T) {
	srv := fakeOllama(t, http.StatusOK)
	defer srv.Close()
	_, e := newTestModels(t, srv.URL)
	vecs, err := e.Embed(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	if len(vecs) != 2 || len(vecs[0]) != 3 || e.EmbedModel() != "embed-model" {
		t.Fatalf("got %v", vecs)
	}
}
