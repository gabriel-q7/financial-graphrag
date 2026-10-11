#!/usr/bin/env bash
# Installs frontend dependencies when node_modules is missing or older than package-lock.json.
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

step "Frontend dependencies"
marker="$FRONTEND_DIR/node_modules/.package-lock.json"
if [[ -f "$marker" && "$marker" -nt "$FRONTEND_DIR/package-lock.json" ]]; then
  ok "node_modules up to date"
else
  info "running npm ci…"
  (cd "$FRONTEND_DIR" && npm ci --no-audit --no-fund --loglevel=error) || die "npm ci failed"
  ok "dependencies installed"
fi
