package database

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
)

var migrationName = regexp.MustCompile(`^(\d{3})_[a-z0-9_]+\.cypher$`)

// Migration is one schema file.
type Migration struct {
	Version    string
	Name       string
	Statements []string
}

// LoadMigrations reads NNN_name.cypher files from dir in lexical order.
func LoadMigrations(dir string) ([]Migration, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read migrations dir: %w", err)
	}
	var out []Migration
	for _, e := range entries {
		m := migrationName.FindStringSubmatch(e.Name())
		if e.IsDir() || m == nil {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", e.Name(), err)
		}
		stmts := SplitStatements(string(raw))
		if len(stmts) == 0 {
			return nil, fmt.Errorf("migration %s has no statements", e.Name())
		}
		out = append(out, Migration{Version: m[1], Name: e.Name(), Statements: stmts})
	}
	slices.SortFunc(out, func(a, b Migration) int { return strings.Compare(a.Name, b.Name) })
	for i := 1; i < len(out); i++ {
		if out[i].Version == out[i-1].Version {
			return nil, fmt.Errorf("duplicate migration version %s", out[i].Version)
		}
	}
	return out, nil
}

// SplitStatements removes // comment lines and splits on ';'.
// Migrations are trusted repository files, kept simple on purpose:
// semicolons inside string literals are not supported.
func SplitStatements(src string) []string {
	var b strings.Builder
	for line := range strings.Lines(src) {
		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue
		}
		b.WriteString(line)
	}
	var out []string
	for part := range strings.SplitSeq(b.String(), ";") {
		if s := strings.TrimSpace(part); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// Migrate applies pending migrations and records each as (:Migration {version}).
// Statements are idempotent (IF NOT EXISTS), so a partially applied file can be re-run.
func Migrate(ctx context.Context, driver neo4j.Driver, database string, migrations []Migration, log *slog.Logger) error {
	opts := []neo4j.ExecuteQueryConfigurationOption{neo4j.ExecuteQueryWithDatabase(database)}
	res, err := neo4j.ExecuteQuery(ctx, driver, `MATCH (m:Migration) RETURN m.version AS version`, nil,
		neo4j.EagerResultTransformer, opts...)
	if err != nil {
		return fmt.Errorf("list applied migrations: %w", err)
	}
	applied := map[string]bool{}
	for _, rec := range res.Records {
		if v, ok := rec.Get("version"); ok {
			if s, ok := v.(string); ok {
				applied[s] = true
			}
		}
	}

	for _, m := range migrations {
		if applied[m.Version] {
			continue
		}
		// Schema statements cannot share a transaction with data writes,
		// so each runs in its own auto-managed transaction.
		for i, stmt := range m.Statements {
			if _, err := neo4j.ExecuteQuery(ctx, driver, stmt, nil, neo4j.EagerResultTransformer, opts...); err != nil {
				return fmt.Errorf("migration %s statement %d: %w", m.Name, i+1, err)
			}
		}
		_, err := neo4j.ExecuteQuery(ctx, driver,
			`MERGE (m:Migration {version: $version}) ON CREATE SET m.name = $name, m.applied_at = datetime()`,
			map[string]any{"version": m.Version, "name": m.Name}, neo4j.EagerResultTransformer, opts...)
		if err != nil {
			return fmt.Errorf("record migration %s: %w", m.Name, err)
		}
		log.Info("migration applied", "name", m.Name)
	}
	return nil
}
