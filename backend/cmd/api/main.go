// Command api runs the Financial GraphRAG HTTP API.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gabrielaquino/financial-graphrag/backend/internal/config"
	"github.com/gabrielaquino/financial-graphrag/backend/internal/database"
	"github.com/gabrielaquino/financial-graphrag/backend/internal/graph"
	"github.com/gabrielaquino/financial-graphrag/backend/internal/health"
	apihttp "github.com/gabrielaquino/financial-graphrag/backend/internal/http"
	"github.com/gabrielaquino/financial-graphrag/backend/internal/http/handlers"
	"github.com/gabrielaquino/financial-graphrag/backend/internal/ollama"
	"github.com/gabrielaquino/financial-graphrag/backend/internal/seed"
)

func main() {
	if err := run(); err != nil {
		slog.Error("api stopped", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	driver, err := database.NewDriver(cfg.Neo4jURI, cfg.Neo4jUser, cfg.Neo4jPassword)
	if err != nil {
		return err
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := driver.Close(closeCtx); err != nil {
			log.Warn("close neo4j driver", "err", err)
		}
	}()

	// Apply schema migrations when Neo4j is reachable. The API still starts when it
	// is not, so /api/health can report the dependency as unavailable.
	startCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	if err := driver.VerifyConnectivity(startCtx); err != nil {
		log.Warn("neo4j unreachable at start-up; migrations skipped", "err", err)
	} else if ms, err := database.LoadMigrations(cfg.MigrationsDir); err != nil {
		cancel()
		return err
	} else if err := database.Migrate(startCtx, driver, cfg.Neo4jDatabase, ms, log); err != nil {
		log.Error("migrations failed", "err", err)
	}
	cancel()

	ollamaClient := ollama.New(cfg.OllamaURL, cfg.OllamaChatModel, cfg.OllamaEmbedModel, nil)

	deps := apihttp.Deps{
		Health:        &health.Checker{Neo4j: driver, Ollama: ollamaClient, Timeout: cfg.HealthTimeout, Log: log},
		Graph:         &handlers.Graph{Reader: graph.NewRepository(driver, cfg.Neo4jDatabase), Timeout: cfg.DBTimeout, Log: log},
		AllowedOrigin: cfg.CORSAllowedOrigin,
		Log:           log,
	}
	if cfg.AdminSeedAllowed() {
		deps.Admin = &handlers.Admin{
			Token:   cfg.AdminToken,
			Timeout: 60 * time.Second,
			Log:     log,
			Seed: func(ctx context.Context) (seed.Result, error) {
				return database.MigrateAndSeed(ctx, driver, cfg.Neo4jDatabase, cfg.MigrationsDir, cfg.SeedFile, log)
			},
		}
		log.Warn("admin seed endpoint enabled (development only)")
	}

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           apihttp.NewRouter(deps),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      90 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("api listening", "addr", cfg.HTTPAddr, "env", cfg.Env)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		log.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return err
		}
	}
	return nil
}
