#!/usr/bin/env bash
# Applies Neo4j migrations, loads the synthetic dataset and computes document
# embeddings (all idempotent). Extra flags are passed to the seed command:
#   --migrate-only, --embeddings-only, --skip-embeddings
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
load_env

step "Database (migrations, seed, embeddings)"
mkdir -p "$BACKEND_DIR/bin"
(cd "$BACKEND_DIR" && go build -o bin/seed ./cmd/seed) || die "failed to build the seed command"

out="$(cd "$BACKEND_DIR" && ./bin/seed "$@" 2>&1)" || { printf '%s\n' "$out" >&2; die "seed failed"; }
if [[ "$*" == *--migrate-only* ]]; then
  ok "migrations up to date"
else
  summary="$(sed -n '/^{/,$p' <<<"$out" | tr -d '\n ' )"
  ok "done: $summary"
fi
