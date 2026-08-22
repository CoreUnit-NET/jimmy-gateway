#!/usr/bin/env bash
# HTTP smoke checks against a running jimmy-gateway instance.
# Usage: PORT=38081 ./scripts/smoke.sh
#        BASE_URL=http://127.0.0.1:8080 API_KEY=secret ./scripts/smoke.sh
# Set SMOKE_LIVE=1 to run one real chat completion (slow; hits ChatJimmy upstream).
set -euo pipefail

PORT="${PORT:-8080}"
BASE_URL="${BASE_URL:-http://127.0.0.1:${PORT}}"
API_KEY="${API_KEY:-}"
SMOKE_LIVE="${SMOKE_LIVE:-0}"

auth_args=()
if [[ -n "${API_KEY}" ]]; then
  auth_args=(-H "Authorization: Bearer ${API_KEY}")
fi

pass=0
fail=0

check_status() {
  local name="$1"
  local want="$2"
  local method="$3"
  local path="$4"
  shift 4
  local extra=("$@")

  local out
  out="$(curl -sS -o /tmp/jimmy-smoke-body.txt -w '%{http_code}' -X "${method}" "${BASE_URL}${path}" "${auth_args[@]}" "${extra[@]}")"
  if [[ "${out}" == "${want}" ]]; then
    echo "OK   ${name} (${want})"
    pass=$((pass + 1))
  else
    echo "FAIL ${name} (got ${out}, want ${want})"
    echo "     body: $(head -c 200 /tmp/jimmy-smoke-body.txt)"
    fail=$((fail + 1))
  fi
}

echo "smoke: ${BASE_URL}"
echo

# Health and routing
check_status "GET /health" 200 GET /health
check_status "GET /" 200 GET /
check_status "GET /api/health alias" 200 GET /api/health
check_status "GET /api alias" 200 GET /api
check_status "GET /v1/models" 200 GET /v1/models
check_status "GET /api/v1-models alias" 200 GET /api/v1-models

# Method / not found
check_status "GET /missing" 404 GET /missing
check_status "POST /v1/models" 405 POST /v1/models
check_status "POST /health" 405 POST /health
check_status "GET /v1/chat/completions" 405 GET /v1/chat/completions
check_status "GET /v1/completions" 405 GET /v1/completions
check_status "GET /api/v1-completions alias" 405 GET /api/v1-completions
check_status "GET /api/chat" 405 GET /api/chat "${auth_args[@]}"
check_status "GET /v1/messages" 405 GET /v1/messages "${auth_args[@]}"
check_status "GET gemini generate" 405 GET "/v1beta/models/gemini-1.5-flash:generateContent" "${auth_args[@]}"
check_status "POST gemini bad suffix" 404 POST "/v1beta/models/gemini-1.5-flash:unknownAction" "${auth_args[@]}"

# CORS preflight
preflight="$(curl -sS -o /dev/null -w '%{http_code}' -X OPTIONS "${BASE_URL}/v1/chat/completions" \
  -H 'Origin: https://app.example' \
  -H 'Access-Control-Request-Method: POST')"
if [[ "${preflight}" == "204" ]]; then
  echo "OK   OPTIONS /v1/chat/completions (204)"
  pass=$((pass + 1))
else
  echo "FAIL OPTIONS /v1/chat/completions (got ${preflight}, want 204)"
  fail=$((fail + 1))
fi

# JSON validation (no upstream when body invalid)
check_status "POST chat invalid JSON" 400 POST /v1/chat/completions \
  "${auth_args[@]}" -H 'Content-Type: application/json' -d 'not-json'
check_status "POST chat empty messages" 400 POST /v1/chat/completions \
  "${auth_args[@]}" -H 'Content-Type: application/json' -d '{"messages":[]}'
check_status "POST completions invalid JSON" 400 POST /v1/completions \
  "${auth_args[@]}" -H 'Content-Type: application/json' -d 'not-json'
check_status "POST completions missing prompt" 400 POST /v1/completions \
  "${auth_args[@]}" -H 'Content-Type: application/json' -d '{"model":"llama3.1-8B"}'
check_status "POST completions empty prompt" 400 POST /v1/completions \
  "${auth_args[@]}" -H 'Content-Type: application/json' -d '{"prompt":""}'
check_status "POST native empty messages" 400 POST /api/chat \
  "${auth_args[@]}" -H 'Content-Type: application/json' -d '{"messages":[]}'
check_status "POST anthropic empty messages" 400 POST /v1/messages \
  "${auth_args[@]}" -H 'Content-Type: application/json' \
  -d '{"model":"claude-3-haiku-20240307","max_tokens":32,"messages":[]}'
check_status "POST gemini empty contents" 400 POST "/v1beta/models/gemini-1.5-flash:generateContent" \
  "${auth_args[@]}" -H 'Content-Type: application/json' -d '{"contents":[]}'

# Auth when API_KEY is configured
if [[ -n "${API_KEY}" ]]; then
  unauth="$(curl -sS -o /dev/null -w '%{http_code}' -X POST "${BASE_URL}/v1/chat/completions" \
    -H 'Content-Type: application/json' \
    -d '{"messages":[{"role":"user","content":"hi"}]}')"
  if [[ "${unauth}" == "401" ]]; then
    echo "OK   POST chat without key (401)"
    pass=$((pass + 1))
  else
    echo "FAIL POST chat without key (got ${unauth}, want 401)"
    fail=$((fail + 1))
  fi
fi

# Optional live upstream chat (real ChatJimmy call)
if [[ "${SMOKE_LIVE}" == "1" ]]; then
  echo
  echo "live chat (upstream)..."
  live="$(curl -sS -o /tmp/jimmy-smoke-live.txt -w '%{http_code}' -X POST "${BASE_URL}/v1/chat/completions" \
    "${auth_args[@]}" -H 'Content-Type: application/json' \
    -d '{"model":"llama3.1-8B","messages":[{"role":"user","content":"Reply with exactly: pong"}],"max_tokens":16}')"
  if [[ "${live}" == "200" ]] && grep -q '"choices"' /tmp/jimmy-smoke-live.txt; then
    echo "OK   POST /v1/chat/completions live (${live})"
    pass=$((pass + 1))
  else
    echo "FAIL POST /v1/chat/completions live (status ${live})"
    echo "     body: $(head -c 200 /tmp/jimmy-smoke-live.txt)"
    fail=$((fail + 1))
  fi
fi

echo
echo "smoke done: ${pass} passed, ${fail} failed"
if [[ "${fail}" -gt 0 ]]; then
  exit 1
fi
