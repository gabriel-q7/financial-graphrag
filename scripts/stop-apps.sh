#!/usr/bin/env bash
# Stops API and frontend processes listening on their ports (e.g. left over from a closed terminal).
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
[[ -f "$ENV_FILE" ]] && load_env

step "Stopping API and frontend"
for port in "$(port_from_addr "${HTTP_ADDR:-127.0.0.1:8080}")" 5173; do
  owner="$(port_owner "$port")"
  if [[ -z "$owner" ]]; then
    ok "nothing on port $port"
  elif [[ "$owner" == \?* ]]; then
    warn "port $port is used by a process this user cannot stop"
  else
    pid="${owner%% *}"
    kill -TERM "$pid" && ok "stopped pid $pid on port $port (${owner#* })"
  fi
done
