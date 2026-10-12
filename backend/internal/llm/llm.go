// Package llm wires the language models through langchaingo
// (github.com/tmc/langchaingo). The rest of the application depends only on the
// small interfaces in internal/rag, so switching provider (Ollama today; OpenAI,
// Anthropic, etc. are available in langchaingo) is a change to this package only.
package llm

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/tmc/langchaingo/embeddings"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/ollama"

	"github.com/gabrielaquino/financial-graphrag/backend/internal/rag"
)

// Config selects the Ollama server and models.
type Config struct {
	OllamaURL  string
	ChatModel  string
	EmbedModel string
	// NumCtx is the context window allocated by Ollama for the chat model.
	// Retrieval keeps prompts under ~2,500 tokens, so 4096 fits prompt + answer.
	NumCtx int
	// KeepAlive keeps the model loaded between questions (avoids 5–12 s reloads).
	KeepAlive string
}

// httpClient has no overall timeout: answers stream for minutes on CPU and every
// call is bounded by its context.
func httpClient() *http.Client {
	return &http.Client{Transport: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		ResponseHeaderTimeout: 3 * time.Minute, // model load + prompt evaluation
		IdleConnTimeout:       90 * time.Second,
	}}
}

// NewOllama builds the chat generator and the embedder.
func NewOllama(cfg Config) (*Generator, *Embedder, error) {
	common := []ollama.Option{ollama.WithServerURL(cfg.OllamaURL), ollama.WithHTTPClient(httpClient())}
	if cfg.KeepAlive != "" {
		common = append(common, ollama.WithKeepAlive(cfg.KeepAlive))
	}

	chatOpts := append([]ollama.Option{ollama.WithModel(cfg.ChatModel)}, common...)
	if cfg.NumCtx > 0 {
		chatOpts = append(chatOpts, ollama.WithRunnerNumCtx(cfg.NumCtx))
	}
	chat, err := ollama.New(chatOpts...)
	if err != nil {
		return nil, nil, fmt.Errorf("ollama chat model: %w", err)
	}

	embedLLM, err := ollama.New(append([]ollama.Option{ollama.WithModel(cfg.EmbedModel)}, common...)...)
	if err != nil {
		return nil, nil, fmt.Errorf("ollama embedding model: %w", err)
	}
	// Keep newlines: the task prompts and documents are short and newlines carry structure.
	emb, err := embeddings.NewEmbedder(embedLLM, embeddings.WithStripNewLines(false), embeddings.WithBatchSize(16))
	if err != nil {
		return nil, nil, fmt.Errorf("embedder: %w", err)
	}
	return &Generator{model: chat, name: cfg.ChatModel}, &Embedder{embedder: emb, name: cfg.EmbedModel}, nil
}

// Generator adapts an llms.Model to rag.Generator.
type Generator struct {
	model llms.Model
	name  string
}

// NewGenerator wraps any langchaingo model (used by tests and other providers).
func NewGenerator(model llms.Model, name string) *Generator {
	return &Generator{model: model, name: name}
}

// ChatModel returns the model name.
func (g *Generator) ChatModel() string { return g.name }

// Generate streams a completion for a system + user prompt.
func (g *Generator) Generate(ctx context.Context, system, user string, opts rag.GenerationOptions, onToken func(string) error) (rag.GenerationStats, error) {
	stats := rag.GenerationStats{Model: g.name}
	start := time.Now()
	var first time.Time
	resp, err := g.model.GenerateContent(ctx,
		[]llms.MessageContent{
			llms.TextParts(llms.ChatMessageTypeSystem, system),
			llms.TextParts(llms.ChatMessageTypeHuman, user),
		},
		llms.WithMaxTokens(opts.MaxTokens),
		llms.WithTemperature(opts.Temperature),
		llms.WithStreamingFunc(func(_ context.Context, chunk []byte) error {
			if len(chunk) == 0 {
				return nil
			}
			if first.IsZero() {
				first = time.Now()
			}
			return onToken(string(chunk))
		}),
	)
	if err != nil {
		return stats, err
	}
	end := time.Now()
	if len(resp.Choices) > 0 {
		info := resp.Choices[0].GenerationInfo
		stats.PromptTokens = asInt(info["PromptTokens"])
		stats.GeneratedTokens = asInt(info["CompletionTokens"])
	}
	if !first.IsZero() {
		stats.FirstTokenMS = first.Sub(start).Milliseconds()
		stats.GenerationMS = end.Sub(first).Milliseconds()
		if secs := end.Sub(first).Seconds(); secs > 0 && stats.GeneratedTokens > 1 {
			stats.TokensPerSecond = float64(stats.GeneratedTokens-1) / secs
		}
	}
	return stats, nil
}

// Embedder adapts an embeddings.Embedder to rag.Embedder.
type Embedder struct {
	embedder embeddings.Embedder
	name     string
}

// EmbedModel returns the model name (used to pick task prompts and to tag vectors).
func (e *Embedder) EmbedModel() string { return e.name }

// Embed returns one vector per text.
func (e *Embedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	return e.embedder.EmbedDocuments(ctx, texts)
}

func asInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	}
	return 0
}

var (
	_ rag.Generator = (*Generator)(nil)
	_ rag.Embedder  = (*Embedder)(nil)
)
