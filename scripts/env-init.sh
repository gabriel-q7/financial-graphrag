#!/usr/bin/env bash
# Creates .env from .env.example with random local secrets. Never overwrites an existing .env.
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

step "Environment file"
if [[ -f "$ENV_FILE" ]]; then
  ok ".env already exists (kept as is)"
  # Warn about variables added to .env.example after .env was created.
  missing="$(comm -23 \
    <(grep -oE '^[A-Z_]+=' "$ENV_EXAMPLE" | sort -u) \
    <(grep -oE '^[A-Z_]+=' "$ENV_FILE" | sort -u) | tr -d '=' | tr '\n' ' ')"
  [[ -z "$missing" ]] || warn "variables missing from .env (see .env.example): $missing"
  exit 0
fi

command -v openssl >/dev/null || die "openssl is required to generate local secrets"
umask 077
sed -e "s/^NEO4J_PASSWORD=.*/NEO4J_PASSWORD=$(openssl rand -hex 16)/" \
    -e "s/^ADMIN_TOKEN=.*/ADMIN_TOKEN=$(openssl rand -hex 24)/" \
    "$ENV_EXAMPLE" > "$ENV_FILE"
ok "created .env with random NEO4J_PASSWORD and ADMIN_TOKEN (permissions 600)"
