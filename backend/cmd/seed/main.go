// Command seed applies Neo4j migrations, loads the synthetic dataset and writes
// document embeddings. Every step is idempotent: running it repeatedly does not
// create duplicates and only re-embeds documents whose text or model changed.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"log/slog"
	"os"
	"time"

	"github.com/gabrielaquino/financial-graphrag/backend/internal/config"
	"github.com/gabrielaquino/financial-graphrag/backend/internal/database"
	"github.com/gabrielaquino/financial-graphrag/backend/internal/llm"
	"github.com/gabrielaquino/financial-graphrag/backend/internal/rag"
)

func main() {
	migrateOnly := flag.Bool("migrate-only", false, "apply migrations only")
	skipEmbeddings := flag.Bool("skip-embeddings", false, "do not compute document embeddings")
	embeddingsOnly := flag.Bool("embeddings-only", false, "only compute missing/stale document embeddings")
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := run(*migrateOnly, *skipEmbeddings, *embeddingsOnly, log); err != nil {
		log.Error("seed failed", "err", err)
		os.Exit(1)
	}
}

func run(migrateOnly, skipEmbeddings, embeddingsOnly bool, log *slog.Logger) error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	driver, err := database.NewDriver(cfg.Neo4jURI, cfg.Neo4jUser, cfg.Neo4jPassword)
	if err != nil {
		return err
	}
	defer driver.Close(context.Background())
	if err := driver.VerifyConnectivity(ctx); err != nil {
		return err
	}

	ms, err := database.LoadMigrations(cfg.MigrationsDir)
	if err != nil {
		return err
	}
	if err := database.Migrate(ctx, driver, cfg.Neo4jDatabase, ms, log); err != nil {
		return err
	}
	if migrateOnly {
		return nil
	}

	out := map[string]any{}
	if !embeddingsOnly {
		res, err := database.MigrateAndSeed(ctx, driver, cfg.Neo4jDatabase, cfg.MigrationsDir, cfg.SeedFile, log)
		if err != nil {
			return err
		}
		out["seed"] = res
	}
	if !skipEmbeddings {
		_, embedder, err := llm.NewOllama(llm.Config{OllamaURL: cfg.OllamaURL, ChatModel: cfg.OllamaChatModel, EmbedModel: cfg.OllamaEmbedModel})
		if err != nil {
			return err
		}
		ix := rag.Indexer{
			Store:      rag.NewNeo4jStore(driver, cfg.Neo4jDatabase),
			Embedder:   embedder,
			Dimensions: cfg.OllamaEmbedDimensions,
			Log:        log,
		}
		res, err := ix.Run(ctx)
		if err != nil {
			return err
		}
		out["embeddings"] = res
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
