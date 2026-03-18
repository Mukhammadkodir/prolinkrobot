#!/usr/bin/env bash
set -euo pipefail

APP_DIR="${APP_DIR:-/opt/prolinkrobot}"
SHARED_DIR="${APP_DIR}/shared"
SRC_DIR="${1:-$(pwd)}"
BIN_PATH="${TELEGRAM_LOCAL_API_BINARY:-/usr/local/bin/telegram-bot-api}"
SRC_REPO_DIR="${TELEGRAM_LOCAL_API_SRC_DIR:-$SHARED_DIR/telegram-bot-api-src}"
FORCE_REBUILD="${TELEGRAM_LOCAL_API_FORCE_REBUILD:-0}"

if [[ "${TELEGRAM_LOCAL_API_ENABLED:-0}" != "1" ]]; then
  exit 0
fi

: "${TELEGRAM_LOCAL_API_ID:?TELEGRAM_LOCAL_API_ID is required when TELEGRAM_LOCAL_API_ENABLED=1}"
: "${TELEGRAM_LOCAL_API_HASH:?TELEGRAM_LOCAL_API_HASH is required when TELEGRAM_LOCAL_API_ENABLED=1}"

apt-get update -y
DEBIAN_FRONTEND=noninteractive apt-get install -y \
  build-essential \
  ca-certificates \
  cmake \
  git \
  gperf \
  libssl-dev \
  zlib1g-dev

mkdir -p "$SHARED_DIR"

if [[ ! -d "$SRC_REPO_DIR/.git" ]]; then
  rm -rf "$SRC_REPO_DIR"
  git clone --recursive https://github.com/tdlib/telegram-bot-api.git "$SRC_REPO_DIR"
else
  git -C "$SRC_REPO_DIR" fetch --depth 1 origin master
  git -C "$SRC_REPO_DIR" reset --hard FETCH_HEAD
  git -C "$SRC_REPO_DIR" submodule sync --recursive
  git -C "$SRC_REPO_DIR" submodule update --init --recursive --depth 1
fi

if [[ "$FORCE_REBUILD" == "1" || ! -x "$BIN_PATH" ]]; then
  rm -rf "$SRC_REPO_DIR/build"
  cmake -S "$SRC_REPO_DIR" -B "$SRC_REPO_DIR/build" -DCMAKE_BUILD_TYPE=Release -DCMAKE_INSTALL_PREFIX=/usr/local
  cmake --build "$SRC_REPO_DIR/build" --target install --parallel "$(nproc)"
fi

sed \
  -e "s|__APP_USER__|${APP_USER:-ubuntu}|g" \
  -e "s|__APP_DIR__|$APP_DIR|g" \
  "$SRC_DIR/deploy/lightsail/prolinkrobot-telegram-bot-api.service" > "/etc/systemd/system/prolinkrobot-telegram-bot-api.service"

systemctl daemon-reload
systemctl enable prolinkrobot-telegram-bot-api
systemctl restart prolinkrobot-telegram-bot-api
