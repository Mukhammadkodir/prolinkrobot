#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

if [[ -f "$ROOT_DIR/.env" ]]; then
  set -a
  # shellcheck disable=SC1091
  source "$ROOT_DIR/.env"
  set +a
fi

if [[ "${TELEGRAM_LOCAL_API_ENABLED:-0}" == "1" ]]; then
  if [[ "${TELEGRAM_LOCAL_API_ID:-}" == "your_api_id" || ! "${TELEGRAM_LOCAL_API_ID:-}" =~ ^[0-9]+$ ]]; then
    echo "TELEGRAM_LOCAL_API_ID must be a real numeric api_id from https://my.telegram.org."
    exit 1
  fi
  if [[ "${TELEGRAM_LOCAL_API_HASH:-}" == "your_api_hash" || -z "${TELEGRAM_LOCAL_API_HASH:-}" ]]; then
    echo "TELEGRAM_LOCAL_API_HASH must be a real api_hash from https://my.telegram.org."
    exit 1
  fi
  export TELEGRAM_CACHE_API_ENDPOINT="${TELEGRAM_CACHE_API_ENDPOINT:-http://127.0.0.1:8081/bot%s/%s}"
  export CACHE_MAX_UPLOAD_BYTES="${CACHE_MAX_UPLOAD_BYTES:-2147483648}"
fi

tmpgocache="$(mktemp -d /tmp/go-build.XXXXXX)"
tmpgomod="$(mktemp -d /tmp/gomodcache.XXXXXX)"
cleanup() {
  rm -rf "$tmpgocache" "$tmpgomod"
}
trap cleanup EXIT

cd "$ROOT_DIR"
exec env GOCACHE="$tmpgocache" GOMODCACHE="$tmpgomod" go run ./cmd
