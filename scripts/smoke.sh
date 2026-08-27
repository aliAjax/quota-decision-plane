#!/bin/sh
set -eu
BASE_URL="${BASE_URL:-http://127.0.0.1:18333}"
API_KEY="${QUOTA_API_KEY:-dev-secret}"
curl -fsS "$BASE_URL/healthz"
curl -fsS "$BASE_URL/readyz"
curl -fsS -H "X-API-Key: $API_KEY" -H 'Content-Type: application/json' -d '{"tenant_id":"demo","resource":"api_calls","dimensions":{"service":"payments","method":"POST","region":"ap-northeast-1","customer":"smoke"},"cost":1,"idempotency_key":"smoke-check"}' "$BASE_URL/v1/check"
RESERVE=$(curl -fsS -H "X-API-Key: $API_KEY" -H 'Content-Type: application/json' -d '{"tenant_id":"demo","resource":"jobs","dimensions":{"service":"worker","region":"ap-northeast-1"},"cost":1,"ttl_ms":30000,"idempotency_key":"smoke-reserve"}' "$BASE_URL/v1/reserve")
RID=$(printf '%s' "$RESERVE" | sed -n 's/.*"reservation_id":"\([^"]*\)".*/\1/p')
test -n "$RID"
curl -fsS -H "X-API-Key: $API_KEY" -H 'Content-Type: application/json' -d '{"idempotency_key":"smoke-commit"}' "$BASE_URL/v1/reservations/$RID/commit"
curl -fsS "$BASE_URL/metrics" | grep quota_decisions_total

