package ollama

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientAgainstFakeServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/version":
			_, _ = w.Write([]byte(`{"version":"0.34.4"}`))
		case "/api/tags":
			_, _ = w.Write([]byte(`{"models":[{"name":"nomic-embed-text:latest"},{"name":"llama3.2:3b"}]}`))
		case "/api/embed":
			var body struct {
				Model string   `json:"model"`
				Input []string `json:"input"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body.Model != "nomic-embed-text" || len(body.Input) != 2 {
				http.Error(w, "bad", http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte(`{"embeddings":[[0.1,0.2],[0.3,0.4]]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := New(srv.URL+"/", "llama3.2:3b", "nomic-embed-text", nil)
	ctx := context.Background()

	v, err := c.Version(ctx)
	if err != nil || v != "0.34.4" {
		t.Fatalf("version: %q %v", v, err)
	}
	models, err := c.Models(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !HasModel(models, "nomic-embed-text") || !HasModel(models, "llama3.2:3b") || HasModel(models, "mistral") {
		t.Fatalf("HasModel mismatch for %v", models)
	}
	emb, err := c.Embed(ctx, []string{"a", "b"})
	if err != nil || len(emb) != 2 || len(emb[0]) != 2 {
		t.Fatalf("embed: %v %v", emb, err)
	}
}

func TestClientStatusError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()
	_, err := New(srv.URL, "", "", nil).Version(context.Background())
	if !errors.Is(err, ErrStatus) {
		t.Fatalf("expected ErrStatus, got %v", err)
	}
}
