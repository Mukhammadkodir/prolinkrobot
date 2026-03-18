#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

if [[ -f "$ROOT_DIR/.env" ]]; then
  set -a
  # shellcheck disable=SC1091
  source "$ROOT_DIR/.env"
  set +a
fi

export TELEGRAM_LOCAL_API_ENABLED="${TELEGRAM_LOCAL_API_ENABLED:-1}"
: "${TELEGRAM_LOCAL_API_ID:?TELEGRAM_LOCAL_API_ID is required}"
: "${TELEGRAM_LOCAL_API_HASH:?TELEGRAM_LOCAL_API_HASH is required}"

if [[ "$TELEGRAM_LOCAL_API_ID" == "your_api_id" || ! "$TELEGRAM_LOCAL_API_ID" =~ ^[0-9]+$ ]]; then
  echo "TELEGRAM_LOCAL_API_ID must be a real numeric api_id from https://my.telegram.org."
  echo "Current value in .env is invalid: $TELEGRAM_LOCAL_API_ID"
  exit 1
fi

if [[ "$TELEGRAM_LOCAL_API_HASH" == "your_api_hash" || -z "$TELEGRAM_LOCAL_API_HASH" ]]; then
  echo "TELEGRAM_LOCAL_API_HASH must be a real api_hash from https://my.telegram.org."
  echo "Current value in .env is invalid."
  exit 1
fi

export TELEGRAM_LOCAL_API_HTTP_IP="${TELEGRAM_LOCAL_API_HTTP_IP:-127.0.0.1}"
export TELEGRAM_LOCAL_API_PORT="${TELEGRAM_LOCAL_API_PORT:-8081}"
export TELEGRAM_LOCAL_API_DIR="${TELEGRAM_LOCAL_API_DIR:-$ROOT_DIR/.local/telegram-bot-api}"
export TELEGRAM_LOCAL_API_TEMP_DIR="${TELEGRAM_LOCAL_API_TEMP_DIR:-$TELEGRAM_LOCAL_API_DIR/tmp}"
export TELEGRAM_LOCAL_API_LOG_VERBOSITY="${TELEGRAM_LOCAL_API_LOG_VERBOSITY:-1}"

if [[ -z "${TELEGRAM_LOCAL_API_BINARY:-}" ]]; then
  if [[ -x "$ROOT_DIR/.local/bin/telegram-bot-api" ]]; then
    export TELEGRAM_LOCAL_API_BINARY="$ROOT_DIR/.local/bin/telegram-bot-api"
  elif command -v telegram-bot-api >/dev/null 2>&1; then
    export TELEGRAM_LOCAL_API_BINARY="$(command -v telegram-bot-api)"
  elif command -v /usr/local/bin/telegram-bot-api >/dev/null 2>&1; then
    export TELEGRAM_LOCAL_API_BINARY="/usr/local/bin/telegram-bot-api"
  else
    echo "telegram-bot-api binary not found."
    echo "Run ./scripts/install_local_telegram_bot_api.sh first or set TELEGRAM_LOCAL_API_BINARY in .env."
    exit 1
  fi
fi

mkdir -p "$TELEGRAM_LOCAL_API_DIR" "$TELEGRAM_LOCAL_API_TEMP_DIR"

exec bash "$ROOT_DIR/deploy/lightsail/run-telegram-bot-api.sh"
