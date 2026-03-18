#!/usr/bin/env bash
set -euo pipefail

APP_NAME="prolinkrobot"
APP_USER="${APP_USER:-ubuntu}"
APP_DIR="${APP_DIR:-/opt/prolinkrobot}"
SERVICE_NAME="${SERVICE_NAME:-prolinkrobot}"
LOCAL_API_SERVICE_NAME="${LOCAL_API_SERVICE_NAME:-prolinkrobot-telegram-bot-api}"
SRC_DIR="${1:-$(pwd)}"

RELEASES_DIR="$APP_DIR/releases"
SHARED_DIR="$APP_DIR/shared"
CURRENT_LINK="$APP_DIR/current"
RELEASE_NAME="$(date +%Y%m%d%H%M%S)"
RELEASE_DIR="$RELEASES_DIR/$RELEASE_NAME"

load_env_file() {
  local env_file="$1"
  [[ -f "$env_file" ]] || return 0

  while IFS= read -r raw_line || [[ -n "$raw_line" ]]; do
    local line="${raw_line%$'\r'}"
    line="${line#"${line%%[![:space:]]*}"}"
    line="${line%"${line##*[![:space:]]}"}"
    [[ -z "$line" || "${line:0:1}" == "#" ]] && continue
    [[ "$line" == export\ * ]] && line="${line#export }"
    [[ "$line" != *=* ]] && continue

    local key="${line%%=*}"
    local value="${line#*=}"
    key="${key%"${key##*[![:space:]]}"}"
    key="${key#"${key%%[![:space:]]*}"}"
    if [[ -z "$key" ]]; then
      continue
    fi
    if [[ ${#value} -ge 2 ]]; then
      if [[ "${value:0:1}" == '"' && "${value: -1}" == '"' ]] || [[ "${value:0:1}" == "'" && "${value: -1}" == "'" ]]; then
        value="${value:1:${#value}-2}"
      fi
    fi
    printf -v "$key" '%s' "$value"
    export "$key"
  done < "$env_file"
}

if [[ ! -f "$SRC_DIR/prolinkrobot" ]]; then
  echo "Binary not found in $SRC_DIR"
  exit 1
fi

apt-get update -y
DEBIAN_FRONTEND=noninteractive apt-get install -y ca-certificates tzdata

if [[ "${INSTALL_BROWSER_REFRESH_DEPS:-0}" == "1" ]]; then
  DEBIAN_FRONTEND=noninteractive apt-get install -y google-chrome-stable
fi

id -u "$APP_USER" >/dev/null 2>&1 || useradd --system --create-home --shell /usr/sbin/nologin "$APP_USER"

mkdir -p "$RELEASES_DIR" "$SHARED_DIR"
mkdir -p "$RELEASE_DIR"
install -m 0755 "$SRC_DIR/prolinkrobot" "$RELEASE_DIR/prolinkrobot"
mkdir -p "$RELEASE_DIR/deploy/lightsail"
install -m 0755 "$SRC_DIR/deploy/lightsail/run-telegram-bot-api.sh" "$RELEASE_DIR/deploy/lightsail/run-telegram-bot-api.sh"

if [[ -f "$SRC_DIR/.env.example" && ! -f "$SHARED_DIR/.env" ]]; then
  install -m 0600 "$SRC_DIR/.env.example" "$SHARED_DIR/.env"
fi

if [[ -f "$SRC_DIR/freepik_cookies.example.json" && ! -f "$SHARED_DIR/freepik_cookies.json" ]]; then
  install -m 0600 "$SRC_DIR/freepik_cookies.example.json" "$SHARED_DIR/freepik_cookies.json"
fi

load_env_file "$SHARED_DIR/.env"

ln -sfn "$RELEASE_DIR" "$CURRENT_LINK"

sed \
  -e "s|__APP_USER__|$APP_USER|g" \
  -e "s|__APP_DIR__|$APP_DIR|g" \
  "$SRC_DIR/deploy/lightsail/prolinkrobot.service" > "/etc/systemd/system/$SERVICE_NAME.service"

sed \
  -e "s|__APP_USER__|$APP_USER|g" \
  -e "s|__APP_DIR__|$APP_DIR|g" \
  "$SRC_DIR/deploy/lightsail/prolinkrobot-telegram-bot-api.service" > "/etc/systemd/system/$LOCAL_API_SERVICE_NAME.service"

chown -R "$APP_USER":"$APP_USER" "$APP_DIR"
chmod 0600 "$SHARED_DIR/.env" 2>/dev/null || true
chmod 0600 "$SHARED_DIR/freepik_cookies.json" 2>/dev/null || true

systemctl daemon-reload
if [[ "${TELEGRAM_LOCAL_API_ENABLED:-0}" == "1" ]]; then
  APP_USER="$APP_USER" APP_DIR="$APP_DIR" "$SRC_DIR/deploy/lightsail/install-telegram-bot-api.sh" "$SRC_DIR"
else
  systemctl disable --now "$LOCAL_API_SERVICE_NAME" 2>/dev/null || true
fi
systemctl enable "$SERVICE_NAME"
systemctl restart "$SERVICE_NAME"
systemctl --no-pager --full status "$SERVICE_NAME" | sed -n '1,20p'

echo
echo "Shared env file: $SHARED_DIR/.env"
echo "Shared cookies file: $SHARED_DIR/freepik_cookies.json"
echo "Logs: journalctl -u $SERVICE_NAME -f"
