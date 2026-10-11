#!/usr/bin/env bash
# Starts Neo4j and Ollama and waits until their healthchecks pass.
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
load_env

step "Docker services (Neo4j, Ollama)"
compose up -d --wait --wait-timeout 180 || {
  compose ps
  die "services did not become healthy; inspect with: make logs-services"
}
compose ps --format '    {{.Name}}: {{.Status}}'
