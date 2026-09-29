# QR login helpers (Taobao / 1688)

Standalone Playwright scripts to complete the QR-code login flow for
Taobao and 1688 headlessly (e.g. on a VPS with no display), by capturing
the `codeContent` string from `qrcode/generate.do` and rendering it into a
QR locally, instead of reading the (cross-origin-tainted) login `<canvas>`.

Run once, interactively, wherever you can view/scan the saved PNG:

```sh
pip install -r requirements.txt
playwright install chromium

python taobao_qr_login.py   # -> taobao_qr.png, taobao_session.json
python 1688_qr_login.py     # -> 1688_qr.png, 1688_session.json
```

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
