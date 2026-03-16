#!/usr/bin/env bash
set -euo pipefail

LIGHTSAIL_HOST="${LIGHTSAIL_HOST:-}"
LIGHTSAIL_USER="${LIGHTSAIL_USER:-ubuntu}"
TARGET_ARCH="${TARGET_ARCH:-amd64}"
LIGHTSAIL_SSH_KEY="${LIGHTSAIL_SSH_KEY:-}"
REMOTE_TMP="/tmp/prolinkrobot-lightsail.tar.gz"
REMOTE_DIR="/tmp/prolinkrobot-lightsail-$$"

if [[ -z "$LIGHTSAIL_HOST" ]]; then
  echo "LIGHTSAIL_HOST is required"
  exit 1
fi

ARCHIVE_PATH="$(TARGET_ARCH="$TARGET_ARCH" ./scripts/package_lightsail.sh)"

SSH_ARGS=()
if [[ -n "$LIGHTSAIL_SSH_KEY" ]]; then
  SSH_ARGS=(-i "$LIGHTSAIL_SSH_KEY")
fi

echo "Uploading $ARCHIVE_PATH to $LIGHTSAIL_USER@$LIGHTSAIL_HOST"
scp "${SSH_ARGS[@]}" "$ARCHIVE_PATH" "$LIGHTSAIL_USER@$LIGHTSAIL_HOST:$REMOTE_TMP"

ssh "${SSH_ARGS[@]}" "$LIGHTSAIL_USER@$LIGHTSAIL_HOST" "set -euo pipefail; rm -rf '$REMOTE_DIR'; mkdir -p '$REMOTE_DIR'; tar -xzf '$REMOTE_TMP' -C '$REMOTE_DIR'; sudo APP_USER='$LIGHTSAIL_USER' bash '$REMOTE_DIR/deploy/lightsail/install-server.sh' '$REMOTE_DIR'; rm -f '$REMOTE_TMP'"

echo "Deployment completed."
