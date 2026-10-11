#!/usr/bin/env bash
# Verifies required tools, versions and free ports for local development.
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

step "Prerequisites"
errors=0
need() {
  if command -v "$1" >/dev/null; then ok "$1 $($2 2>/dev/null | head -1 | cut -c1-60)"; else warn "$1 not found — $3"; errors=$((errors + 1)); fi
}
need docker  "docker --version"  "install Docker Engine"
need go      "go version"        "install Go 1.27+"
need node    "node --version"    "install Node.js 22.12+"
need npm     "npm --version"     "comes with Node.js"
need curl    "curl --version"    "install curl"
need openssl "openssl version"   "install openssl"
need ss      "ss --version"      "install iproute2"

if command -v docker >/dev/null; then
  docker compose version >/dev/null 2>&1 || { warn "docker compose v2 plugin not found"; errors=$((errors + 1)); }
  docker info >/dev/null 2>&1 || { warn "Docker daemon not reachable (is it running? is your user in the docker group?)"; errors=$((errors + 1)); }
fi

if command -v go >/dev/null; then
  gomin="$(go env GOVERSION | sed -E 's/^go1\.([0-9]+).*/\1/')"
  (( gomin >= 27 )) || { warn "Go 1.27+ required, found $(go env GOVERSION)"; errors=$((errors + 1)); }
fi
if command -v node >/dev/null; then
  IFS=. read -r nmaj nmin _ <<<"$(node --version | tr -d v)"
  (( nmaj > 22 || (nmaj == 22 && nmin >= 12) )) || { warn "Node.js 22.12+ required, found $(node --version)"; errors=$((errors + 1)); }
fi

(( errors == 0 )) || die "$errors prerequisite(s) missing"
