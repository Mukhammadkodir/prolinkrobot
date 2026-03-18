# prolinkrobot

Google Sheet dependency removed. Bot now works as:

1. Receive Freepik link in Telegram.
2. Request Freepik download API with your account cookies.
3. Return direct download link.
4. Auto-refresh Freepik `GR_TOKEN` from the cookie file when `GR_REFRESH` is available.
5. Monitor Freepik auth status and notify admins if refresh stops working.

## Setup

1. Copy `.env.example` to `.env` and fill `BOT_TOKEN`.
2. Add cookies one of two ways:
   - `FREEPIK_COOKIE_HEADER` in `.env`, or
   - create `freepik_cookies.json` (use `freepik_cookies.example.json` format).
3. Run:

```bash
go run ./cmd
```

You can also run:

```bash
go run .
```

## Lightsail

Recommended deployment target: Ubuntu Lightsail instance with `systemd`.

What is included:

1. `deploy/lightsail/prolinkrobot.service`
2. `deploy/lightsail/prolinkrobot-telegram-bot-api.service`
3. `deploy/lightsail/install-server.sh`
4. `deploy/lightsail/install-telegram-bot-api.sh`
5. `scripts/package_lightsail.sh`
6. `scripts/deploy_lightsail.sh`

### Build deployment archive locally

Default target is `linux/amd64`:

```bash
./scripts/package_lightsail.sh
```

For Graviton/ARM instance:

```bash
TARGET_ARCH=arm64 ./scripts/package_lightsail.sh
```

### Deploy from local machine

```bash
LIGHTSAIL_HOST=YOUR_SERVER_IP ./scripts/deploy_lightsail.sh
```

Optional:

```bash
LIGHTSAIL_USER=ubuntu TARGET_ARCH=arm64 ./scripts/deploy_lightsail.sh
```

### Manual install on server

1. Upload archive to server.
2. Extract it to a temp directory.
3. Run:

```bash
sudo APP_USER=ubuntu bash deploy/lightsail/install-server.sh .
```

The service will use:

1. Binary: `/opt/prolinkrobot/current/prolinkrobot`
2. Env file: `/opt/prolinkrobot/shared/.env`
3. Cookies file: `/opt/prolinkrobot/shared/freepik_cookies.json`

### Local Telegram Bot API server

Use this when you want cache uploads larger than the default Telegram Bot API limit.

Required env values in `.env`:

```bash
TELEGRAM_LOCAL_API_ENABLED=1
TELEGRAM_LOCAL_API_ID=your_api_id
TELEGRAM_LOCAL_API_HASH=your_api_hash
TELEGRAM_LOCAL_API_BINARY=.local/bin/telegram-bot-api
TELEGRAM_LOCAL_API_PORT=8081
TELEGRAM_LOCAL_API_HTTP_IP=127.0.0.1
TELEGRAM_LOCAL_API_BUILD_JOBS=1
TELEGRAM_LOCAL_API_SWAP_SIZE_MB=2048
```

Install local binary into the repo:

```bash
./scripts/install_local_telegram_bot_api.sh
```

Recommended local run flow:

```bash
./scripts/run_local_telegram_bot_api.sh
```

Open another terminal and run:

```bash
./scripts/run_local_bot.sh
```

Notes:

1. `./scripts/run_local_bot.sh` automatically sets `TELEGRAM_CACHE_API_ENDPOINT=http://127.0.0.1:8081/bot%s/%s` when `TELEGRAM_LOCAL_API_ENABLED=1`.
2. `./scripts/run_local_bot.sh` also defaults `CACHE_MAX_UPLOAD_BYTES` to `2147483648` when `TELEGRAM_LOCAL_API_ENABLED=1`.
3. This keeps user-facing bot traffic on the default Telegram API and sends only cache uploads through the local server.
4. If you prefer a full switch, you can still set `TELEGRAM_API_ENDPOINT=http://127.0.0.1:8081/bot%s/%s` manually.
5. `api_id` and `api_hash` come from [my.telegram.org](https://my.telegram.org).
6. On small servers, keep `TELEGRAM_LOCAL_API_BUILD_JOBS=1` and enable swap to avoid SSH lockups during the first TDLib build.

### Service commands

```bash
sudo systemctl restart prolinkrobot
sudo systemctl status prolinkrobot
sudo journalctl -u prolinkrobot -f
sudo systemctl status prolinkrobot-telegram-bot-api
sudo journalctl -u prolinkrobot-telegram-bot-api -f
```

## Notes

- Required cookie values usually include `GR_REFRESH`, `GR_TOKEN`, and a csrf cookie such as `csrftoken` or `csrf_freepik`.
- When `FREEPIK_COOKIES_FILE` points to a file, the bot reloads that file on every request.
- Refresh order is:
  1. Firebase securetoken refresh when `FREEPIK_FIREBASE_API_KEY` is set
  2. homepage cookie refresh fallback
  3. optional browser refresh fallback when `FREEPIK_BROWSER_REFRESH_ENABLED=true`
- Browser refresh requires a local Chrome/Chromium binary. Recommended env values:
  - `FREEPIK_BROWSER_REFRESH_ENABLED=true`
  - `FREEPIK_BROWSER_PATH=/usr/bin/google-chrome-stable`
  - `FREEPIK_BROWSER_HEADLESS=true`
  - `FREEPIK_BROWSER_TIMEOUT_SECONDS=90`
  - `FREEPIK_BROWSER_WAIT_SECONDS=8`
  - `FREEPIK_BROWSER_USER_DATA_DIR=/opt/prolinkrobot/shared/chrome-profile`
- If Freepik returns 401/403 and auto-refresh cannot recover, refresh cookies from a logged-in browser.
- Admins receive Telegram warnings before `GR_TOKEN` expires. Defaults:
  - check every `5` minutes
  - warning window `20` minutes
  - critical window `5` minutes
  - auto-refresh when expiry is within `15` minutes (`FREEPIK_AUTH_REFRESH_BEFORE_MINUTES`)
