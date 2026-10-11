package config

import (
	"strings"
	"testing"
)

func envFrom(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load(envFrom(map[string]string{"NEO4J_PASSWORD": "secret-pass"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Env != "development" || cfg.HTTPAddr != "127.0.0.1:8080" {
		t.Errorf("unexpected defaults: %+v", cfg)
	}
	if cfg.AdminSeedAllowed() {
		t.Error("admin seed must be disabled by default")
	}
}

func TestLoadRequiresPassword(t *testing.T) {
	_, err := Load(envFrom(map[string]string{}))
	if err == nil || !strings.Contains(err.Error(), "NEO4J_PASSWORD") {
		t.Fatalf("expected NEO4J_PASSWORD error, got %v", err)
	}
}

func TestAdminSeedRules(t *testing.T) {
	base := map[string]string{"NEO4J_PASSWORD": "x", "ADMIN_SEED_ENABLED": "true"}

	if _, err := Load(envFrom(base)); err == nil || !strings.Contains(err.Error(), "ADMIN_TOKEN") {
		t.Fatalf("expected ADMIN_TOKEN error, got %v", err)
	}

	base["ADMIN_TOKEN"] = "0123456789abcdef"
	cfg, err := Load(envFrom(base))
	if err != nil || !cfg.AdminSeedAllowed() {
		t.Fatalf("expected admin seed allowed, err=%v", err)
	}

	base["APP_ENV"] = "production"
	cfg, err = Load(envFrom(base))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.AdminSeedAllowed() {
		t.Error("admin seed must never be allowed in production")
	}
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	cases := map[string]map[string]string{
		"env":        {"APP_ENV": "staging"},
		"cors":       {"CORS_ALLOWED_ORIGIN": "localhost:5173"},
		"neo4j uri":  {"NEO4J_URI": "http://localhost:7687"},
		"dimensions": {"OLLAMA_EMBED_DIMENSIONS": "-1"},
		"bool":       {"ADMIN_SEED_ENABLED": "maybe"},
		"log level":  {"LOG_LEVEL": "loud"},
	}
	for name, env := range cases {
		t.Run(name, func(t *testing.T) {
			env["NEO4J_PASSWORD"] = "x"
			if _, err := Load(envFrom(env)); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
