// Package database owns the Neo4j driver lifecycle and schema migrations.
package database

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/gabrielaquino/financial-graphrag/backend/internal/seed"
	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
	neo4jconfig "github.com/neo4j/neo4j-go-driver/v6/neo4j/config"
)

// NewDriver creates a driver. Connections are opened lazily, so this succeeds even
// when Neo4j is down; use driver.VerifyConnectivity to check. The caller must Close it.
// Errors never include the password.
func NewDriver(uri, user, password string) (neo4j.Driver, error) {
	driver, err := neo4j.NewDriver(uri, neo4j.BasicAuth(user, password, ""), func(c *neo4jconfig.Config) {
		c.MaxConnectionPoolSize = 50
		c.ConnectionAcquisitionTimeout = 5 * time.Second
		c.SocketConnectTimeout = 5 * time.Second
		// Fail fast when Neo4j is down instead of retrying until the request timeout.
		c.MaxTransactionRetryTime = 3 * time.Second
	})
	if err != nil {
		return nil, fmt.Errorf("create neo4j driver: %w", err)
	}
	return driver, nil
}

// MigrateAndSeed applies migrations from dir, then loads the seed file.
func MigrateAndSeed(ctx context.Context, driver neo4j.Driver, database, migrationsDir, seedFile string, log *slog.Logger) (seed.Result, error) {
	ms, err := LoadMigrations(migrationsDir)
	if err != nil {
		return seed.Result{}, err
	}
	if err := Migrate(ctx, driver, database, ms, log); err != nil {
		return seed.Result{}, err
	}
	d, err := seed.LoadFile(seedFile)
	if err != nil {
		return seed.Result{}, err
	}
	return seed.Run(ctx, driver, database, d)
}
