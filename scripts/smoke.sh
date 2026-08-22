#!/usr/bin/env bash
# HTTP smoke checks against a running jimmy-gateway instance.
# Usage: PORT=38081 ./scripts/smoke.sh
#        BASE_URL=http://127.0.0.1:8080 API_KEY=secret ./scripts/smoke.sh
# Set SMOKE_LIVE=1 to run real upstream checks (chat, stream+usage, completions).
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

# Usage: curl_status <bodyfile> [curl args...]
# Always writes the response body to bodyfile. Callers must not pass -o
# (curl treats extra -o as extra URLs, so a second file is never created).
curl_status() {
  local bodyfile="$1"
  shift
  local code
  if ! code="$(curl -sS -o "${bodyfile}" -w '%{http_code}' "$@" 2>/tmp/jimmy-smoke-curl.err)"; then
    echo ""
    return 0
  fi
  echo "${code}"
}

check_status() {
  local name="$1"
  local want="$2"
  local method="$3"
  local path="$4"
  shift 4
  local extra=("$@")

  local out
  out="$(curl_status /tmp/jimmy-smoke-body.txt -X "${method}" "${BASE_URL}${path}" "${auth_args[@]}" "${extra[@]}")"
  if [[ -z "${out}" ]]; then
    echo "FAIL ${name} (curl error: $(head -c 200 /tmp/jimmy-smoke-curl.err))"
    fail=$((fail + 1))
  elif [[ "${out}" == "${want}" ]]; then
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

# Fail fast if nothing is listening (avoids 25 identical curl exit-7 lines under set -e)
probe="$(curl_status /dev/null -X GET "${BASE_URL}/health")"
if [[ -z "${probe}" ]]; then
  echo "FAIL connect ${BASE_URL} (curl error: $(head -c 200 /tmp/jimmy-smoke-curl.err))"
  echo
  echo "smoke done: 0 passed, 1 failed"
  echo "hint: start the gateway (e.g. make docker/dev or make run) and retry"
  exit 1
fi

# Health and routing
check_status "GET /health" 200 GET /health
check_status "GET /" 200 GET /
check_status "GET /healthz alias" 200 GET /healthz
check_status "GET /api/health alias" 200 GET /api/health
check_status "GET /api alias" 200 GET /api
check_status "GET /v1/models" 200 GET /v1/models
check_status "GET /models alias" 200 GET /models
check_status "GET /api/v1-models alias" 200 GET /api/v1-models

# Method / not found
check_status "GET /missing" 404 GET /missing
check_status "POST /v1/models" 405 POST /v1/models
check_status "POST /health" 405 POST /health
check_status "GET /v1/chat/completions" 405 GET /v1/chat/completions
check_status "GET /chat/completions alias" 405 GET /chat/completions
check_status "GET /v1/completions" 405 GET /v1/completions
check_status "GET /completions alias" 405 GET /completions
check_status "GET /api/v1-completions alias" 405 GET /api/v1-completions
check_status "GET /api/chat" 405 GET /api/chat "${auth_args[@]}"
check_status "GET /v1/messages" 405 GET /v1/messages "${auth_args[@]}"
check_status "GET gemini generate" 405 GET "/v1beta/models/gemini-1.5-flash:generateContent" "${auth_args[@]}"
check_status "POST gemini bad suffix" 404 POST "/v1beta/models/gemini-1.5-flash:unknownAction" "${auth_args[@]}"

# CORS preflight
preflight="$(curl_status /dev/null -X OPTIONS "${BASE_URL}/v1/chat/completions" \
  -H 'Origin: https://app.example' \
  -H 'Access-Control-Request-Method: POST')"
if [[ -z "${preflight}" ]]; then
  echo "FAIL OPTIONS /v1/chat/completions (curl error: $(head -c 200 /tmp/jimmy-smoke-curl.err))"
  fail=$((fail + 1))
elif [[ "${preflight}" == "204" ]]; then
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
  unauth="$(curl_status /dev/null -X POST "${BASE_URL}/v1/chat/completions" \
    -H 'Content-Type: application/json' \
    -d '{"messages":[{"role":"user","content":"hi"}]}')"
  if [[ -z "${unauth}" ]]; then
    echo "FAIL POST chat without key (curl error: $(head -c 200 /tmp/jimmy-smoke-curl.err))"
    fail=$((fail + 1))
  elif [[ "${unauth}" == "401" ]]; then
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
  live="$(curl_status /tmp/jimmy-smoke-live.txt -X POST "${BASE_URL}/v1/chat/completions" \
    "${auth_args[@]}" -H 'Content-Type: application/json' \
    -d '{"model":"llama3.1-8B","messages":[{"role":"user","content":"Reply with exactly: pong"}],"max_tokens":16}')"
  if [[ -z "${live}" ]]; then
    echo "FAIL POST /v1/chat/completions live (curl error: $(head -c 200 /tmp/jimmy-smoke-curl.err))"
    fail=$((fail + 1))
  elif [[ "${live}" == "200" ]] && grep -q '"choices"' /tmp/jimmy-smoke-live.txt; then
    if grep -E -q '<think>|<thinking>|<tool_call>|<\|stats\|>' /tmp/jimmy-smoke-live.txt; then
      echo "FAIL POST /v1/chat/completions live (leaked think/tool/stats markup)"
      fail=$((fail + 1))
    else
      echo "OK   POST /v1/chat/completions live (${live})"
      pass=$((pass + 1))
    fi
  else
    echo "FAIL POST /v1/chat/completions live (status ${live})"
    echo "     body: $(head -c 200 /tmp/jimmy-smoke-live.txt)"
    fail=$((fail + 1))
  fi

  echo "live chat stream + include_usage..."
  stream="$(curl_status /tmp/jimmy-smoke-stream.txt -D /tmp/jimmy-smoke-stream.hdr -X POST "${BASE_URL}/v1/chat/completions" \
    "${auth_args[@]}" -H 'Content-Type: application/json' \
    -d '{"model":"llama3.1-8B","messages":[{"role":"user","content":"Reply with exactly: pong"}],"max_tokens":16,"stream":true,"stream_options":{"include_usage":true}}')"
  if [[ -z "${stream}" ]]; then
    echo "FAIL POST chat stream (curl error: $(head -c 200 /tmp/jimmy-smoke-curl.err))"
    fail=$((fail + 1))
  elif [[ "${stream}" == "200" ]] \
    && grep -qi 'text/event-stream' /tmp/jimmy-smoke-stream.hdr \
    && grep -q 'data: \[DONE\]' /tmp/jimmy-smoke-stream.txt \
    && grep -q '"usage"' /tmp/jimmy-smoke-stream.txt; then
    echo "OK   POST /v1/chat/completions stream+usage (${stream})"
    pass=$((pass + 1))
  else
    echo "FAIL POST chat stream+usage (status ${stream})"
    echo "     hdr: $(head -c 200 /tmp/jimmy-smoke-stream.hdr)"
    echo "     body: $(head -c 200 /tmp/jimmy-smoke-stream.txt)"
    fail=$((fail + 1))
  fi

  echo "live completions..."
  cmpl="$(curl_status /tmp/jimmy-smoke-cmpl.txt -X POST "${BASE_URL}/v1/completions" \
    "${auth_args[@]}" -H 'Content-Type: application/json' \
    -d '{"model":"llama3.1-8B","prompt":"Reply with exactly: pong","max_tokens":16}')"
  if [[ -z "${cmpl}" ]]; then
    echo "FAIL POST /v1/completions live (curl error: $(head -c 200 /tmp/jimmy-smoke-curl.err))"
    fail=$((fail + 1))
  elif [[ "${cmpl}" == "200" ]] && grep -q '"text_completion"' /tmp/jimmy-smoke-cmpl.txt; then
    echo "OK   POST /v1/completions live (${cmpl})"
    pass=$((pass + 1))
  else
    echo "FAIL POST /v1/completions live (status ${cmpl})"
    echo "     body: $(head -c 200 /tmp/jimmy-smoke-cmpl.txt)"
    fail=$((fail + 1))
  fi
fi

echo
echo "smoke done: ${pass} passed, ${fail} failed"
if [[ "${fail}" -gt 0 ]]; then
  exit 1
fi