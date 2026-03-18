#!/usr/bin/env bash
set -euo pipefail

if [[ "${TELEGRAM_LOCAL_API_ENABLED:-0}" != "1" ]]; then
  echo "TELEGRAM_LOCAL_API_ENABLED is not set to 1"
  exit 1
fi

: "${TELEGRAM_LOCAL_API_ID:?TELEGRAM_LOCAL_API_ID is required}"
: "${TELEGRAM_LOCAL_API_HASH:?TELEGRAM_LOCAL_API_HASH is required}"

BIN="${TELEGRAM_LOCAL_API_BINARY:-/usr/local/bin/telegram-bot-api}"
HOST="${TELEGRAM_LOCAL_API_HTTP_IP:-127.0.0.1}"
PORT="${TELEGRAM_LOCAL_API_PORT:-8081}"
STATE_DIR="${TELEGRAM_LOCAL_API_DIR:-/opt/prolinkrobot/shared/telegram-bot-api}"
TEMP_DIR="${TELEGRAM_LOCAL_API_TEMP_DIR:-$STATE_DIR/tmp}"
VERBOSITY="${TELEGRAM_LOCAL_API_LOG_VERBOSITY:-1}"

mkdir -p "$STATE_DIR" "$TEMP_DIR"

exec "$BIN" \
  --local \
  --api-id="$TELEGRAM_LOCAL_API_ID" \
  --api-hash="$TELEGRAM_LOCAL_API_HASH" \
  --http-ip-address="$HOST" \
  --http-port="$PORT" \
  --dir="$STATE_DIR" \
  --temp-dir="$TEMP_DIR" \
  --verbosity="$VERBOSITY"
