#!/usr/bin/env bash
# Shared helpers for the scripts in this directory. Source it; do not execute it.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BACKEND_DIR="$ROOT_DIR/backend"
FRONTEND_DIR="$ROOT_DIR/frontend"
ENV_FILE="$ROOT_DIR/.env"
ENV_EXAMPLE="$ROOT_DIR/.env.example"
# Matches `name:` in docker-compose.yml; used to address named volumes.
COMPOSE_PROJECT="$(sed -n 's/^name:[[:space:]]*//p' "$ROOT_DIR/docker-compose.yml")"

if [[ -t 1 ]]; then
  C_RESET=$'\e[0m' C_BOLD=$'\e[1m' C_BLUE=$'\e[34m' C_GREEN=$'\e[32m' C_YELLOW=$'\e[33m' C_RED=$'\e[31m'
else
  C_RESET='' C_BOLD='' C_BLUE='' C_GREEN='' C_YELLOW='' C_RED=''
fi

step() { printf '%s==>%s %s%s%s\n' "$C_BLUE" "$C_RESET" "$C_BOLD" "$*" "$C_RESET"; }
info() { printf '    %s\n' "$*"; }
ok()   { printf '    %s✓%s %s\n' "$C_GREEN" "$C_RESET" "$*"; }
warn() { printf '    %s!%s %s\n' "$C_YELLOW" "$C_RESET" "$*" >&2; }
die()  { printf '%s✗ %s%s\n' "$C_RED" "$*" "$C_RESET" >&2; exit 1; }

# load_env exports every variable from .env into the current shell.
load_env() {
  [[ -f "$ENV_FILE" ]] || die ".env not found. Run: make env"
  set -a
  # shellcheck disable=SC1090
  source "$ENV_FILE"
  set +a
}

compose() { docker compose --project-directory "$ROOT_DIR" -f "$ROOT_DIR/docker-compose.yml" "$@"; }

# port_owner prints "pid command" of the process listening on a TCP port, if any.
port_owner() {
  local port="$1" pid
  pid="$(ss -ltnpH "sport = :$port" 2>/dev/null | grep -oP 'pid=\K[0-9]+' | head -1 || true)"
  if [[ -n "$pid" ]]; then
    printf '%s %s' "$pid" "$(ps -o args= -p "$pid" 2>/dev/null | cut -c1-70)"
  elif ss -ltnH "sport = :$port" 2>/dev/null | grep -q .; then
    printf '? (owned by another user or a container)'
  fi
}

# port_from_addr extracts the port from "host:port".
port_from_addr() { printf '%s' "${1##*:}"; }

# wait_for_http polls a URL until it answers with any HTTP status or the timeout expires.
wait_for_http() {
  local url="$1" timeout="${2:-60}" start=$SECONDS
  until curl -s -o /dev/null --max-time 2 "$url"; do
    (( SECONDS - start >= timeout )) && return 1
    sleep 1
  done
}
