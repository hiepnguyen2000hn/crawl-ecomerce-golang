"""
Full QR-login flow for Taobao, built on the network-layer finding: the QR
image is just a rendering of a `codeContent` URL returned by
`qrCode/generate.do`. Instead of reading the canvas (blocked by taint) or
screenshotting it, this:

  1. Opens the login page and captures `qrCode/generate.do`'s response to
     get `codeContent` — the raw string encoded into the QR.
  2. Renders that string into a QR code locally (own image, no canvas
     involved at all).
  3. Watches the page's own periodic `qrCode/query.do` polling calls (the
     page does this on its own timer) and reports status transitions until
     the user has scanned + confirmed on their phone, or it times out.
  4. On success, dumps `context.storage_state()` to a JSON file so future
     headless runs can reuse the session without logging in again.

Usage:
    python taobao_qr_login.py

Output:
    ./taobao_qr.png            - QR you scan with the Taobao app
    ./taobao_session.json      - storage_state, written only after login succeeds
"""
import json
import sys
import time

import qrcode
from playwright.sync_api import sync_playwright

LOGIN_URL = "https://login.taobao.com/havanaone/login/login.htm?bizName=taobao"
POLL_TIMEOUT_SECONDS = 120  # Taobao QR tokens are typically valid ~3 min


def main() -> int:
    with sync_playwright() as p:
        browser = p.chromium.launch(headless=True)
        context = browser.new_context(
            viewport={"width": 1280, "height": 900},
            user_agent=(
                "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 "
                "(KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
            ),
            locale="zh-CN",
        )
        page = context.new_page()

        state = {"code_content": None, "status": None, "logged_in": False}

        def on_response(resp):
            url = resp.url
            if "qrCode/generate.do" in url and state["code_content"] is None:
                try:
                    data = resp.json()
                    state["code_content"] = data["content"]["data"]["codeContent"]
                    print(f"[+] Got codeContent: {state['code_content']}", file=sys.stderr)
                except Exception as e:
                    print(f"[!] Failed to parse generate.do response: {e}", file=sys.stderr)

            elif "qrCode/query.do" in url:
                try:
                    data = resp.json()
                    status = data["content"]["data"].get("qrCodeStatus")
                    if status != state["status"]:
                        title = data["content"]["data"].get("titleMsg", "")
                        print(f"[status] {state['status']} -> {status} ({title})", file=sys.stderr)
                        state["status"] = status
                    # Statuses observed/expected: NEW -> SCANED -> CONFIRMED
                    # (exact confirmed-name may vary; treat anything that
                    # isn't NEW/SCANED as terminal-success, and watch stderr
                    # to confirm the real string Taobao uses).
                    if status and status not in ("NEW", "SCANED"):
                        state["logged_in"] = True
                except Exception as e:
                    print(f"[!] Failed to parse query.do response: {e}", file=sys.stderr)

        page.on("response", on_response)

        print("Navigating to login page...", file=sys.stderr)
        try:
            page.goto(LOGIN_URL, wait_until="domcontentloaded", timeout=30000)
        except Exception as e:
            print(f"goto warning: {e}", file=sys.stderr)

        # Wait for generate.do to fire and give us codeContent.
        deadline = time.time() + 15
        while state["code_content"] is None and time.time() < deadline:
            page.wait_for_timeout(300)

        if state["code_content"] is None:
            print("[!] Never received codeContent — page structure may have changed.", file=sys.stderr)
            browser.close()
            return 1

        # Render our own QR from the raw string — no canvas/taint involved.
        img = qrcode.make(state["code_content"])
        img.save("taobao_qr.png")
        print("Saved QR to scan: taobao_qr.png", file=sys.stderr)
        print("Scan it with the Taobao app now. Waiting for confirmation...", file=sys.stderr)

        # The page polls qrCode/query.do on its own timer; we just watch
        # the responses our listener already captures.
        deadline = time.time() + POLL_TIMEOUT_SECONDS
        while not state["logged_in"] and time.time() < deadline:
            page.wait_for_timeout(1000)

        if not state["logged_in"]:
            print(f"[!] Timed out after {POLL_TIMEOUT_SECONDS}s without confirmation.", file=sys.stderr)
            browser.close()
            return 1

        print("[+] Login confirmed. Waiting a moment for redirect/cookies to settle...", file=sys.stderr)
        page.wait_for_timeout(3000)

        context.storage_state(path="taobao_session.json")
        print("Saved session: taobao_session.json", file=sys.stderr)
        print("Final URL:", page.url, file=sys.stderr)

        browser.close()
    return 0


if __name__ == "__main__":
    sys.exit(main())
