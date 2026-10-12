package ollama

import (
	"context"
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
			_, _ = w.Write([]byte(`{"models":[{"name":"embeddinggemma:300m"},{"name":"qwen3:4b-instruct"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := New(srv.URL+"/", "qwen3:4b-instruct", "embeddinggemma:300m", nil)
	ctx := context.Background()

	v, err := c.Version(ctx)
	if err != nil || v != "0.34.4" {
		t.Fatalf("version: %q %v", v, err)
	}
	models, err := c.Models(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !HasModel(models, "embeddinggemma:300m") || !HasModel(models, "qwen3:4b-instruct") || HasModel(models, "mistral") {
		t.Fatalf("HasModel mismatch for %v", models)
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

func TestHasModel(t *testing.T) {
	models := []string{"nomic-embed-text:latest", "qwen3:4b-instruct"}
	if !HasModel(models, "nomic-embed-text") || !HasModel(models, "qwen3:4b-instruct") {
		t.Fatal("expected matches")
	}
	if HasModel(models, "qwen3:4b") || HasModel(models, "") {
		t.Fatal("unexpected match")
	}
}
