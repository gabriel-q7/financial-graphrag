# Financial GraphRAG — local development entry point.
# Run `make` or `make help` to list targets. `make dev` starts everything.

SHELL := /usr/bin/env bash
.SHELLFLAGS := -euo pipefail -c
.DEFAULT_GOAL := help

SCRIPTS := ./scripts
COMPOSE := docker compose

.PHONY: help dev setup env check services models db seed migrate stop-apps stop down reset-db \
        status logs logs-services test test-backend test-frontend test-integration lint build clean

help: ## List available targets
	@awk 'BEGIN {FS = ":.*## "; printf "Usage: make <target>\n\n"} \
	  /^##@/ {printf "\n\033[1m%s\033[0m\n", substr($$0, 5)} \
	  /^[a-zA-Z_-]+:.*## / {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)
	@printf "\nOptions: SKIP_MODELS=1 (do not pull models), SKIP_SEED=1 (migrations only), FORCE=1 (no confirmation)\n"

##@ Run

dev: ## Run everything: .env, services, models, DB, API and frontend (Ctrl+C stops apps)
	@$(SCRIPTS)/dev.sh

setup: ## Prepare everything without starting API/frontend (first-time setup, CI)
	@$(SCRIPTS)/env-init.sh
	@$(SCRIPTS)/check-prereqs.sh
	@$(SCRIPTS)/services-up.sh
	@$(SCRIPTS)/models.sh
	@$(SCRIPTS)/frontend-deps.sh
	@$(SCRIPTS)/db-init.sh

stop-apps: ## Stop API/frontend left running on ports 8080/5173
	@$(SCRIPTS)/stop-apps.sh

stop: stop-apps ## Stop API/frontend and the Docker services (data is kept)
	@$(COMPOSE) stop

down: stop-apps ## Remove containers (volumes with data and models are kept)
	@$(COMPOSE) down

##@ Building blocks

env: ## Create .env with random local secrets (never overwrites)
	@$(SCRIPTS)/env-init.sh

check: ## Check required tools and versions
	@$(SCRIPTS)/check-prereqs.sh

services: ## Start Neo4j and Ollama and wait until healthy
	@$(SCRIPTS)/services-up.sh

models: ## Pull configured Ollama models that are missing
	@$(SCRIPTS)/models.sh

db: seed ## Alias for seed

seed: ## Apply migrations and the synthetic dataset (idempotent)
	@$(SCRIPTS)/db-init.sh

migrate: ## Apply migrations only
	@$(SCRIPTS)/db-init.sh --migrate-only

reset-db: ## DELETE all Neo4j data and reseed (asks for confirmation)
	@$(SCRIPTS)/reset-db.sh

##@ Inspect

status: ## Show containers, app ports and API health
	@$(COMPOSE) ps --format 'table {{.Name}}\t{{.Status}}\t{{.Ports}}'
	@echo
	@for p in 8080 5173; do \
	  if ss -ltnH "sport = :$$p" | grep -q .; then echo "port $$p: listening"; else echo "port $$p: free"; fi; \
	done
	@echo
	@curl -s --max-time 5 http://localhost:8080/api/health || echo "API not reachable"
	@echo

logs-services: ## Follow Neo4j and Ollama logs
	@$(COMPOSE) logs -f --tail 100

logs: logs-services ## Alias for logs-services

##@ Quality

test: test-backend test-frontend ## Unit tests (backend + frontend)

test-backend: ## Go unit tests
	@cd backend && go test ./...

test-frontend: ## Frontend unit tests
	@$(SCRIPTS)/frontend-deps.sh >/dev/null
	@cd frontend && npm test --silent

test-integration: services migrate ## Go integration tests against the local Neo4j
	@set -a && source .env && set +a && cd backend && go test -tags integration -count=1 ./internal/integration/

lint: ## gofmt, go vet and TypeScript typecheck
	@cd backend && unformatted="$$(gofmt -l .)" && { [ -z "$$unformatted" ] || { echo "gofmt needed: $$unformatted"; exit 1; }; }
	@cd backend && go vet ./... && go vet -tags integration ./...
	@$(SCRIPTS)/frontend-deps.sh >/dev/null
	@cd frontend && npm run --silent typecheck
	@echo "lint ok"

build: ## Build API/seed binaries and the frontend production bundle
	@cd backend && go build -o bin/ ./cmd/...
	@$(SCRIPTS)/frontend-deps.sh >/dev/null
	@cd frontend && npm run --silent build
	@echo "build ok: backend/bin/{api,seed}, frontend/dist"

clean: ## Remove build outputs (keeps containers, volumes and node_modules)
	@rm -rf backend/bin frontend/dist frontend/node_modules/.tmp
	@echo "clean ok"
