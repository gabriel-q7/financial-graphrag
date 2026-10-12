// Package config loads application settings from environment variables.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const minAdminTokenLen = 16

// Config holds every runtime setting. Secrets are never logged.
type Config struct {
	Env               string
	HTTPAddr          string
	CORSAllowedOrigin string
	LogLevel          slog.Level

	Neo4jURI      string
	Neo4jUser     string
	Neo4jPassword string
	Neo4jDatabase string

	OllamaURL             string
	OllamaChatModel       string
	OllamaEmbedModel      string
	OllamaEmbedDimensions int

	MigrationsDir string
	SeedFile      string

	AdminSeedEnabled bool
	AdminToken       string

	DBTimeout     time.Duration
	HealthTimeout time.Duration
}

// IsProduction reports whether the app runs with APP_ENV=production.
func (c Config) IsProduction() bool { return c.Env == "production" }

// AdminSeedAllowed reports whether the admin seed endpoint may be registered.
func (c Config) AdminSeedAllowed() bool { return c.AdminSeedEnabled && !c.IsProduction() }

// Load reads configuration from the environment using getenv (os.Getenv in production code).
func Load(getenv func(string) string) (Config, error) {
	if getenv == nil {
		getenv = os.Getenv
	}
	get := func(key, def string) string {
		if v := strings.TrimSpace(getenv(key)); v != "" {
			return v
		}
		return def
	}

	var errs []error
	cfg := Config{
		Env:                   get("APP_ENV", "development"),
		HTTPAddr:              get("HTTP_ADDR", "127.0.0.1:8080"),
		CORSAllowedOrigin:     get("CORS_ALLOWED_ORIGIN", "http://localhost:5173"),
		Neo4jURI:              get("NEO4J_URI", "neo4j://localhost:7687"),
		Neo4jUser:             get("NEO4J_USER", "neo4j"),
		Neo4jPassword:         getenv("NEO4J_PASSWORD"),
		Neo4jDatabase:         get("NEO4J_DATABASE", "neo4j"),
		OllamaURL:             strings.TrimRight(get("OLLAMA_URL", "http://localhost:11434"), "/"),
		OllamaChatModel:       get("OLLAMA_CHAT_MODEL", "qwen3:4b-instruct"),
		OllamaEmbedModel:      get("OLLAMA_EMBED_MODEL", "embeddinggemma:300m"),
		MigrationsDir:         get("MIGRATIONS_DIR", "../neo4j/migrations"),
		SeedFile:              get("SEED_FILE", "../data/seed/financial_network.json"),
		AdminToken:            getenv("ADMIN_TOKEN"),
		OllamaEmbedDimensions: 768,
		DBTimeout:             10 * time.Second,
		HealthTimeout:         3 * time.Second,
	}

	switch cfg.Env {
	case "development", "test", "production":
	default:
		errs = append(errs, fmt.Errorf("APP_ENV must be development, test or production, got %q", cfg.Env))
	}

	if err := cfg.LogLevel.UnmarshalText([]byte(get("LOG_LEVEL", "info"))); err != nil {
		errs = append(errs, fmt.Errorf("LOG_LEVEL: %w", err))
	}

	if cfg.Neo4jPassword == "" {
		errs = append(errs, errors.New("NEO4J_PASSWORD is required"))
	}

	if err := validateURL("CORS_ALLOWED_ORIGIN", cfg.CORSAllowedOrigin, "http", "https"); err != nil {
		errs = append(errs, err)
	}
	if err := validateURL("OLLAMA_URL", cfg.OllamaURL, "http", "https"); err != nil {
		errs = append(errs, err)
	}
	if err := validateURL("NEO4J_URI", cfg.Neo4jURI, "neo4j", "neo4j+s", "neo4j+ssc", "bolt", "bolt+s", "bolt+ssc"); err != nil {
		errs = append(errs, err)
	}

	if v := getenv("OLLAMA_EMBED_DIMENSIONS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			errs = append(errs, fmt.Errorf("OLLAMA_EMBED_DIMENSIONS must be a positive integer"))
		}
		cfg.OllamaEmbedDimensions = n
	}

	if v := getenv("ADMIN_SEED_ENABLED"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			errs = append(errs, fmt.Errorf("ADMIN_SEED_ENABLED must be a boolean"))
		}
		cfg.AdminSeedEnabled = b
	}
	if cfg.AdminSeedAllowed() && len(cfg.AdminToken) < minAdminTokenLen {
		errs = append(errs, fmt.Errorf("ADMIN_TOKEN must have at least %d characters when ADMIN_SEED_ENABLED=true", minAdminTokenLen))
	}

	return cfg, errors.Join(errs...)
}

func validateURL(name, raw string, schemes ...string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return fmt.Errorf("%s must be an absolute URL", name)
	}
	for _, s := range schemes {
		if u.Scheme == s {
			return nil
		}
	}
	return fmt.Errorf("%s has unsupported scheme %q", name, u.Scheme)
}
