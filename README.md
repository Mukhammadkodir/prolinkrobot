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
2. `deploy/lightsail/install-server.sh`
3. `scripts/package_lightsail.sh`
4. `scripts/deploy_lightsail.sh`

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

### Service commands

```bash
sudo systemctl restart prolinkrobot
sudo systemctl status prolinkrobot
sudo journalctl -u prolinkrobot -f
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
