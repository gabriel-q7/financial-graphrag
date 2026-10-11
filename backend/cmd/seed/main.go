// Command seed applies Neo4j migrations and loads the synthetic dataset.
// It is idempotent: running it repeatedly does not create duplicates.
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
)

func main() {
	migrateOnly := flag.Bool("migrate-only", false, "apply migrations without loading the dataset")
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := run(*migrateOnly, log); err != nil {
		log.Error("seed failed", "err", err)
		os.Exit(1)
	}
}

func run(migrateOnly bool, log *slog.Logger) error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	driver, err := database.NewDriver(cfg.Neo4jURI, cfg.Neo4jUser, cfg.Neo4jPassword)
	if err != nil {
		return err
	}
	defer driver.Close(context.Background())
	if err := driver.VerifyConnectivity(ctx); err != nil {
		return err
	}

	if migrateOnly {
		ms, err := database.LoadMigrations(cfg.MigrationsDir)
		if err != nil {
			return err
		}
		return database.Migrate(ctx, driver, cfg.Neo4jDatabase, ms, log)
	}

	res, err := database.MigrateAndSeed(ctx, driver, cfg.Neo4jDatabase, cfg.MigrationsDir, cfg.SeedFile, log)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(res)
}
