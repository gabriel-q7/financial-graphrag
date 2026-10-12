#!/usr/bin/env bash
# One command to run the whole application locally:
#   .env → prerequisites → Neo4j + Ollama → models → migrations + seed → API + frontend.
# Every step is idempotent. Ctrl+C stops the API and the frontend (containers keep running;
# use `make stop` to stop them).
#
# Options (environment variables):
#   SKIP_MODELS=1  do not pull missing Ollama models (also skips embeddings)
#   SKIP_SEED=1    only apply migrations, do not (re)apply the dataset
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
SCRIPTS="$ROOT_DIR/scripts"

"$SCRIPTS/env-init.sh"
"$SCRIPTS/check-prereqs.sh"
load_env

API_PORT="$(port_from_addr "$HTTP_ADDR")"
WEB_PORT=5173
WEB_URL="http://localhost:$WEB_PORT"
API_URL="${VITE_API_BASE_URL:-http://localhost:$API_PORT}"

step "Ports"
for port in "$API_PORT" "$WEB_PORT"; do
  owner="$(port_owner "$port")"
  [[ -z "$owner" ]] || die "port $port is already in use by: $owner — stop it (make stop-apps) or free the port"
done
ok "$API_PORT (API) and $WEB_PORT (frontend) are free"

"$SCRIPTS/services-up.sh"
"$SCRIPTS/models.sh"
"$SCRIPTS/frontend-deps.sh"

step "Build API"
(cd "$BACKEND_DIR" && go build -o bin/api ./cmd/api) || die "API build failed"
ok "backend/bin/api"

# Embeddings need the Ollama embedding model; skip them when models were skipped.
seed_flags=()
[[ "${SKIP_MODELS:-0}" == "1" ]] && seed_flags+=(--skip-embeddings)
if [[ "${SKIP_SEED:-0}" == "1" ]]; then "$SCRIPTS/db-init.sh" --migrate-only; else "$SCRIPTS/db-init.sh" "${seed_flags[@]}"; fi

# --- Run API and frontend ---------------------------------------------------
pids=()
cleanup() {
  trap - INT TERM HUP EXIT
  if ((${#pids[@]})); then
    printf '\n'
    step "Stopping API and frontend"
    for pid in "${pids[@]}"; do kill -TERM -- "-$pid" 2>/dev/null || true; done
    wait 2>/dev/null || true
    ok "stopped (containers still running — make stop to stop them)"
  fi
}
# Ctrl+C is the normal way to stop: exit 0. Other signals keep their conventional codes.
trap 'cleanup; exit 0' INT
trap 'cleanup; exit 143' TERM
trap 'cleanup; exit 129' HUP
trap cleanup EXIT

# Each app runs in its own session (process group) so the whole tree can be stopped,
# and its output is prefixed to tell the two streams apart.
run_prefixed() {
  local name="$1" color="$2" dir="$3"; shift 3
  # The prefixer ignores Ctrl+C so the apps' shutdown logs are still printed.
  setsid bash -c 'cd "$1" && shift && exec "$@" 2>&1' _ "$dir" "$@" \
    | (trap '' INT; exec sed -u "s/^/${color}[${name}]${C_RESET} /") &
  # $! is sed; the session leader is the preceding job in the pipeline.
  pids+=("$(jobs -p | tail -1)")
}

step "Starting API and frontend"
run_prefixed api "$C_GREEN" "$BACKEND_DIR" ./bin/api
run_prefixed web "$C_BLUE" "$FRONTEND_DIR" npm run dev --silent -- --clearScreen false

wait_for_http "$API_URL/api/health" 30 || die "API did not start; see [api] logs above"
wait_for_http "$WEB_URL" 30 || die "frontend did not start; see [web] logs above"

health="$(curl -s --max-time 5 "$API_URL/api/health" | grep -oE '"status":"[a-z]+"' | head -1 | cut -d'"' -f4 || true)"
cat <<BANNER

  ${C_BOLD}Financial GraphRAG is running${C_RESET}
    Frontend      ${C_BOLD}$WEB_URL${C_RESET}
    API           $API_URL   (health: ${health:-unknown})
    Neo4j Browser http://localhost:${NEO4J_HTTP_PORT:-7474}   (user: $NEO4J_USER, password in .env)
    Ollama        $OLLAMA_URL

  Ctrl+C stops the API and the frontend.

BANNER

# Exit when either app dies, so a crash is not hidden behind the other one.
wait -n || true
warn "one of the apps exited; stopping the other"
exit 1
