// Package ollama is a small HTTP client for Ollama server metadata (version and
// installed models), used by health checks. Chat and embeddings go through
// langchaingo (internal/llm).
package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client talks to an Ollama server. Chat and embedding models are configured separately.
type Client struct {
	baseURL    string
	chatModel  string
	embedModel string
	http       *http.Client
}

// New builds a client. httpClient may be nil; calls are bounded by their context.
func New(baseURL, chatModel, embedModel string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), chatModel: chatModel, embedModel: embedModel, http: httpClient}
}

// ChatModel returns the configured chat model name.
func (c *Client) ChatModel() string { return c.chatModel }

// EmbedModel returns the configured embedding model name.
func (c *Client) EmbedModel() string { return c.embedModel }

// Version calls GET /api/version.
func (c *Client) Version(ctx context.Context) (string, error) {
	var out struct {
		Version string `json:"version"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/version", nil, &out); err != nil {
		return "", err
	}
	return out.Version, nil
}

// Models calls GET /api/tags and returns locally available model names.
func (c *Client) Models(ctx context.Context) ([]string, error) {
	var out struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/tags", nil, &out); err != nil {
		return nil, err
	}
	names := make([]string, len(out.Models))
	for i, m := range out.Models {
		names[i] = m.Name
	}
	return names, nil
}

// HasModel reports whether name is in models. A name without tag matches ":latest".
func HasModel(models []string, name string) bool {
	if name == "" {
		return false
	}
	want := name
	if !strings.Contains(want, ":") {
		want += ":latest"
	}
	for _, m := range models {
		if m == name || m == want {
			return true
		}
	}
	return false
}

// ErrStatus is returned for non-2xx responses.
var ErrStatus = errors.New("unexpected ollama status")

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, r)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("ollama %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
		return fmt.Errorf("%w %d on %s", ErrStatus, resp.StatusCode, path)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<20)).Decode(out); err != nil {
		return fmt.Errorf("decode ollama %s: %w", path, err)
	}
	return nil
}
