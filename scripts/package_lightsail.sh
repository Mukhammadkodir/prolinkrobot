#!/usr/bin/env bash
set -euo pipefail

APP_NAME="prolinkrobot"
TARGET_OS="linux"
TARGET_ARCH="${TARGET_ARCH:-amd64}"
DIST_DIR="dist/lightsail/${TARGET_OS}-${TARGET_ARCH}"
PACKAGE_ROOT="$DIST_DIR/package"
ARCHIVE_NAME="${APP_NAME}-lightsail-${TARGET_OS}-${TARGET_ARCH}.tar.gz"
ARCHIVE_PATH="$DIST_DIR/$ARCHIVE_NAME"

rm -rf "$PACKAGE_ROOT"
mkdir -p "$PACKAGE_ROOT/deploy/lightsail"

CGO_ENABLED=0 GOOS="$TARGET_OS" GOARCH="$TARGET_ARCH" go build -o "$PACKAGE_ROOT/$APP_NAME" ./cmd
cp deploy/lightsail/prolinkrobot.service "$PACKAGE_ROOT/deploy/lightsail/prolinkrobot.service"
cp deploy/lightsail/prolinkrobot-telegram-bot-api.service "$PACKAGE_ROOT/deploy/lightsail/prolinkrobot-telegram-bot-api.service"
cp deploy/lightsail/install-server.sh "$PACKAGE_ROOT/deploy/lightsail/install-server.sh"
cp deploy/lightsail/install-telegram-bot-api.sh "$PACKAGE_ROOT/deploy/lightsail/install-telegram-bot-api.sh"
cp deploy/lightsail/run-telegram-bot-api.sh "$PACKAGE_ROOT/deploy/lightsail/run-telegram-bot-api.sh"
cp .env.example "$PACKAGE_ROOT/.env.example"
cp freepik_cookies.example.json "$PACKAGE_ROOT/freepik_cookies.example.json"
cp README.md "$PACKAGE_ROOT/README.md"
chmod +x "$PACKAGE_ROOT/deploy/lightsail/install-server.sh"
chmod +x "$PACKAGE_ROOT/deploy/lightsail/install-telegram-bot-api.sh"
chmod +x "$PACKAGE_ROOT/deploy/lightsail/run-telegram-bot-api.sh"

mkdir -p "$DIST_DIR"
tar -C "$PACKAGE_ROOT" -czf "$ARCHIVE_PATH" .

echo "$ARCHIVE_PATH"
