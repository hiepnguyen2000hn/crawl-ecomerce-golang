"""
QR-login flow for 1688.com — same underlying mechanism as
taobao_qr_login.py (login.taobao.com's qrcode/generate.do + query.do APIs),
with exactly one extra piece specific to 1688:

  1688's login isn't a page you can navigate to directly. The homepage
  shows a "已切换至海外站点" popup that must be dismissed first, then the
  "登录" link (a <workbench-i18n> web component, not a plain <a>) opens
  login.taobao.com **inside an <iframe> modal** rather than navigating —
  Playwright's `.click()` API silently no-ops on it (likely due to the
  overlay/shadow-DOM combo), so a real mouse click at its bounding-box
  coordinates is used instead. Everything after that (codeContent capture,
  QR render, status poll, storage_state dump) is identical to Taobao —
  Page.on("response") already covers responses from iframes belonging to
  the page, so no special iframe-handling code is needed for that part.

Usage:
    python 1688_qr_login.py

Output:
    ./1688_qr.png            - QR you scan with the Taobao/1688 app
    ./1688_session.json      - storage_state, written only after login succeeds
"""
import os
import sys
import time

import qrcode
from cloakbrowser import launch
from dotenv import load_dotenv

load_dotenv()

HOME_URL = "https://www.1688.com/"
POLL_TIMEOUT_SECONDS = 120
HEADLESS = os.getenv("PLAYWRIGHT_HEADLESS", "true").strip().lower() not in ("0", "false", "no")
HUMANIZE = os.getenv("CLOAKBROWSER_HUMANIZE", "true").strip().lower() not in ("0", "false", "no")
PROXY = os.getenv("CLOAKBROWSER_PROXY") or None


def main() -> int:
    browser = launch(headless=HEADLESS, humanize=HUMANIZE, proxy=PROXY)
    try:
        context = browser.new_context(
            viewport={"width": 1280, "height": 900},
            locale="zh-CN",
        )
        page = context.new_page()

        state = {"code_content": None, "status": None, "logged_in": False}

        def on_response(resp):
            url_lower = resp.url.lower()
            if "qrcode/generate.do" in url_lower and state["code_content"] is None:
                try:
                    data = resp.json()
                    state["code_content"] = data["content"]["data"]["codeContent"]
                    print(f"[+] Got codeContent: {state['code_content']}", file=sys.stderr)
                except Exception as e:
                    print(f"[!] Failed to parse generate.do response: {e}", file=sys.stderr)

            elif "qrcode/query.do" in url_lower:
                try:
                    data = resp.json()
                    status = data["content"]["data"].get("qrCodeStatus")
                    if status != state["status"]:
                        print(f"[status] {state['status']} -> {status}", file=sys.stderr)
                        state["status"] = status
                    if status and status not in ("NEW", "SCANED"):
                        state["logged_in"] = True
                except Exception as e:
                    print(f"[!] Failed to parse query.do response: {e}", file=sys.stderr)

        # page.on("response") already fires for responses from every frame
        # (including the login iframe below), so this one listener is
        # enough — no need to attach anything to the iframe separately.
        page.on("response", on_response)

        # --- 1688-specific step starts here ---
        print(f"Navigating to {HOME_URL} ...", file=sys.stderr)
        try:
            page.goto(HOME_URL, wait_until="domcontentloaded", timeout=30000)
        except Exception as e:
            print(f"goto warning: {e}", file=sys.stderr)
        page.wait_for_timeout(4000)

        # Dismiss the "已切换至海外站点" (switched to overseas site) popup if
        # present — it overlaps the login link and blocks clicks.
        ok_btn = page.query_selector("text=OK")
        if ok_btn:
            print("Dismissing site-switch popup...", file=sys.stderr)
            ok_btn.click(timeout=3000, force=True)
            page.wait_for_timeout(1000)

        # The login link is a <workbench-i18n class="userSignInText">登录
        # </workbench-i18n> web component. Playwright's normal .click()
        # silently does nothing on it (custom-element/shadow-DOM event
        # handling quirk) — a real mouse click at its screen coordinates
        # works reliably instead.
        login_link = page.get_by_text("登录", exact=True).first
        try:
            box = login_link.bounding_box(timeout=5000)
        except Exception as e:
            print(f"[!] Could not locate login link — site markup may have changed: {e}", file=sys.stderr)
            return 1
        if box is None:
            print("[!] Login link has no bounding box (not visible).", file=sys.stderr)
            return 1

        cx, cy = box["x"] + box["width"] / 2, box["y"] + box["height"] / 2
        page.mouse.move(cx, cy)
        page.mouse.click(cx, cy)
        print("Clicked login link via mouse coordinates.", file=sys.stderr)
        # --- 1688-specific step ends here; rest is identical to Taobao ---

        deadline = time.time() + 15
        while state["code_content"] is None and time.time() < deadline:
            page.wait_for_timeout(300)

        if state["code_content"] is None:
            print("[!] Never received codeContent — page structure may have changed.", file=sys.stderr)
            return 1

        img = qrcode.make(state["code_content"])
        img.save("1688_qr.png")
        print("Saved QR to scan: 1688_qr.png", file=sys.stderr)
        print("Scan it with the Taobao/1688 app now. Waiting for confirmation...", file=sys.stderr)

        deadline = time.time() + POLL_TIMEOUT_SECONDS
        while not state["logged_in"] and time.time() < deadline:
            page.wait_for_timeout(1000)

        if not state["logged_in"]:
            print(f"[!] Timed out after {POLL_TIMEOUT_SECONDS}s without confirmation.", file=sys.stderr)
            return 1

        print("[+] Login confirmed. Waiting a moment for redirect/cookies to settle...", file=sys.stderr)
        page.wait_for_timeout(3000)

        context.storage_state(path="1688_session.json")
        print("Saved session: 1688_session.json", file=sys.stderr)
        print("Final URL:", page.url, file=sys.stderr)
    finally:
        browser.close()
    return 0


if __name__ == "__main__":
    sys.exit(main())
