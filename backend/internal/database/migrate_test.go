package database

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSplitStatements(t *testing.T) {
	src := "// header\nCREATE INDEX a IF NOT EXISTS FOR (n:A) ON (n.x);\n\n  // note\nCREATE INDEX b IF NOT EXISTS\nFOR (n:B) ON (n.y);\n"
	got := SplitStatements(src)
	if len(got) != 2 {
		t.Fatalf("expected 2 statements, got %d: %q", len(got), got)
	}
	if got[1] != "CREATE INDEX b IF NOT EXISTS\nFOR (n:B) ON (n.y)" {
		t.Errorf("unexpected statement: %q", got[1])
	}
}

func TestLoadMigrationsOrderAndFilter(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("002_second.cypher", "RETURN 2;")
	write("001_first.cypher", "RETURN 1;")
	write("README.md", "ignored")

	ms, err := LoadMigrations(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) != 2 || ms[0].Version != "001" || ms[1].Version != "002" {
		t.Fatalf("unexpected migrations: %+v", ms)
	}

	write("002_dup.cypher", "RETURN 3;")
	if _, err := LoadMigrations(dir); err == nil {
		t.Fatal("expected duplicate version error")
	}
}

func TestRepositoryMigrationsParse(t *testing.T) {
	ms, err := LoadMigrations("../../../neo4j/migrations")
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) == 0 {
		t.Fatal("expected at least one migration")
	}
}
