# QR login helpers (Taobao / 1688)

Standalone [CloakBrowser](https://github.com/CloakHQ/cloakbrowser) (stealth
Chromium, drop-in Playwright replacement) scripts to complete the QR-code
login flow for Taobao and 1688 headlessly (e.g. on a VPS with no display),
by capturing the `codeContent` string from `qrcode/generate.do` and
rendering it into a QR locally, instead of reading the
(cross-origin-tainted) login `<canvas>`.

Run once, interactively, wherever you can view/scan the saved PNG:

```sh
pip install -r requirements.txt
cp .env.example .env   # PLAYWRIGHT_HEADLESS, CLOAKBROWSER_HUMANIZE, CLOAKBROWSER_PROXY

python taobao_qr_login.py   # -> taobao_qr.png, taobao_session.json
python 1688_qr_login.py     # -> 1688_qr.png, 1688_session.json
```

Config (`.env`, loaded via `python-dotenv`):
- `PLAYWRIGHT_HEADLESS` — `true`/`1` (default, no display needed — scan the
  saved PNG) or `false`/`0` (opens a visible Chromium window, useful for
  debugging the click/popup logic locally).
- `CLOAKBROWSER_HUMANIZE` — `true`/`1` (default) enables Bezier-curve mouse
  movement and timing jitter on clicks; set `false`/`0` to disable.
- `CLOAKBROWSER_PROXY` — optional proxy URL
  (`http://user:pass@host:port` or `socks5://host:port`).

Note: the fixed `user_agent` override that used to be passed to
`new_context()` was dropped — CloakBrowser generates a self-consistent
fingerprint (UA + client hints + canvas/WebGL/font signals) as a whole;
overriding just the UA string would desync it from the rest and make
detection easier, not harder.

Scan the saved PNG with the Taobao/1688 app. Once confirmed, the script
dumps `storage_state` to a JSON file — reuse it in headless crawl runs via
`browser.new_context(storage_state="taobao_session.json")` instead of
logging in again.

Notes:
- QR tokens expire after a few minutes (`POLL_TIMEOUT_SECONDS = 120`).
- `1688_qr_login.py`'s login click uses real mouse coordinates rather than
  Playwright's `.click()` API — the site's login link is a custom
  `<workbench-i18n>` web component that doesn't respond to synthetic
  clicks, and 1688 opens the login flow inside an `<iframe>` modal rather
  than navigating.
- The `qrCodeStatus` value used to detect a confirmed login (anything
  other than `NEW`/`SCANED`) was inferred, not observed end-to-end against
  a real scan — verify against the real value on first run and adjust if
  needed.
