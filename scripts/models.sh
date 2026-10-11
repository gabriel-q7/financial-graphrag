#!/usr/bin/env bash
# Pulls the configured Ollama models that are not available yet.
# Already present models are never downloaded again. Set SKIP_MODELS=1 to skip this step.
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
load_env

step "Ollama models"
if [[ "${SKIP_MODELS:-0}" == "1" ]]; then
  warn "SKIP_MODELS=1 — skipping (health will report Ollama as degraded until models exist)"
  exit 0
fi

installed="$(compose exec -T ollama ollama list 2>/dev/null | awk 'NR > 1 {print $1}')" ||
  die "Ollama is not running; run: make services"

for model in "$OLLAMA_CHAT_MODEL" "$OLLAMA_EMBED_MODEL"; do
  [[ "$model" == *:* ]] && tag="$model" || tag="$model:latest"
  if grep -qxF "$tag" <<<"$installed"; then
    ok "$model already available"
  else
    info "pulling $model (first time only; may take a few minutes)…"
    if [[ -t 1 ]]; then
      compose exec ollama ollama pull "$model" || die "failed to pull $model"   # live progress bar
    else
      compose exec -T ollama ollama pull "$model" >/dev/null 2>&1 || die "failed to pull $model"
    fi
    ok "$model pulled"
  fi
done
