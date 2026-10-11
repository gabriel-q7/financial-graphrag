#!/usr/bin/env bash
# Deletes all Neo4j data (volume) and recreates the database with migrations + seed.
# Ollama models are kept. Asks for confirmation unless FORCE=1.
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
load_env

step "Reset Neo4j data"
if [[ "${FORCE:-0}" != "1" ]]; then
  read -r -p "    This deletes ALL Neo4j data (volume neo4j_data). Continue? [y/N] " answer
  [[ "$answer" =~ ^[yYsS]$ ]] || die "aborted"
fi
compose rm -sf neo4j >/dev/null
docker volume rm -f "${COMPOSE_PROJECT}_neo4j_data" >/dev/null
ok "neo4j data removed"
"$ROOT_DIR/scripts/services-up.sh"
"$ROOT_DIR/scripts/db-init.sh"
