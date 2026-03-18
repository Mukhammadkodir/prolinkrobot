#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
LOCAL_DIR="${ROOT_DIR}/.local"
SRC_REPO_DIR="${TELEGRAM_LOCAL_API_SRC_DIR:-$LOCAL_DIR/telegram-bot-api-src}"
BIN_DIR="${LOCAL_DIR}/bin"
BIN_PATH="${TELEGRAM_LOCAL_API_BINARY:-$BIN_DIR/telegram-bot-api}"
FORCE_REBUILD="${TELEGRAM_LOCAL_API_FORCE_REBUILD:-0}"

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1"
    exit 1
  fi
}

require_cmd git
require_cmd cmake

if command -v nproc >/dev/null 2>&1; then
  JOBS="$(nproc)"
else
  JOBS="$(sysctl -n hw.ncpu)"
fi

mkdir -p "$LOCAL_DIR" "$BIN_DIR"

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

  CMAKE_ARGS=(
    -S "$SRC_REPO_DIR"
    -B "$SRC_REPO_DIR/build"
    -DCMAKE_BUILD_TYPE=Release
    -DCMAKE_INSTALL_PREFIX="$LOCAL_DIR"
  )

  if [[ "$(uname -s)" == "Darwin" ]] && command -v brew >/dev/null 2>&1; then
    if brew --prefix openssl@3 >/dev/null 2>&1; then
      CMAKE_ARGS+=("-DOPENSSL_ROOT_DIR=$(brew --prefix openssl@3)")
    fi
  fi

  cmake "${CMAKE_ARGS[@]}"
  cmake --build "$SRC_REPO_DIR/build" --target install --parallel "$JOBS"
fi

echo "telegram-bot-api installed at: $BIN_PATH"
