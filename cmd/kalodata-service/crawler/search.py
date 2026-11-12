#!/usr/bin/env python3
"""
Kalodata keyword search via a real browser login (Camoufox).

Keyword search (`/product/searchList`) needs a fresh full login session — an
injected SESSION cookie alone returns 401. So this logs in with email+password
through the account's proxy, then calls the search API from inside the page
(reusing the browser's live session + Cloudflare token) and prints the JSON.

Invoked as a subprocess by cmd/kalodata-service for jobs that carry a keyword.

Exit codes (contract with the Go worker):
  0  OK          JSON {"data":[...]} printed on stdout
  2  LOGIN_FAIL  could not log in (bad creds / OTP required / blocked)
  4  USAGE       bad arguments
"""
from __future__ import annotations

import argparse
import json
import sys
import time
from urllib.parse import urlparse

from camoufox.sync_api import Camoufox  # noqa: E402

EXIT_OK, EXIT_LOGIN_FAIL, EXIT_USAGE = 0, 2, 4


def build_proxy(raw: str):
    if not raw:
        return None
    p = urlparse(raw if "://" in raw else "http://" + raw)
    proxy = {"server": f"{p.scheme}://{p.hostname}:{p.port}"}
    if p.username:
        proxy["username"] = p.username
    if p.password:
        proxy["password"] = p.password
    return proxy


def do_login(page, email: str, password: str) -> bool:
    page.goto("https://www.kalodata.com/login", wait_until="domcontentloaded", timeout=60000)
    page.wait_for_timeout(4000)

    btn = page.query_selector(".tiktok_btn:has-text('Email'), .tiktok_btn:has-text('E-mail'), div:has-text('E-mail')")
    if btn and btn.is_visible():
        btn.click()
        time.sleep(2)

    inp = (page.query_selector("input#register_email")
           or page.query_selector("input[placeholder*='Email']")
           or page.query_selector("input[placeholder*='E-mail']")
           or page.query_selector("input[type='text']"))
    if not inp:
        print("login: email input not found", file=sys.stderr)
        return False
    inp.fill(email)
    time.sleep(0.6)

    pw = page.query_selector("input[type='password'], input[placeholder*='Password']")
    if not pw or not pw.is_visible():
        sw = page.query_selector("span:has-text('password'), a:has-text('password'), div:has-text('Log in with password')")
        if sw:
            sw.click()
            time.sleep(1.5)
        pw = page.query_selector("input[type='password'], input[placeholder*='Password']")
    if not pw:
        print("login: password field not available (OTP-only account?)", file=sys.stderr)
        return False
    pw.fill(password)
    time.sleep(0.5)

    cb = page.query_selector("input[type='checkbox']")
    if cb and not cb.is_checked():
        try:
            cb.check()
        except Exception:
            pass

    sb = (page.query_selector(".submit-btn")
          or page.query_selector("button:has-text('Log in')")
          or page.query_selector("button:has-text('Đăng nhập')"))
    if sb:
        sb.click()
    page.wait_for_timeout(6000)
    # Logged in when we leave /login (lands on /explore or dashboard).
    return "/login" not in page.url


SEARCH_JS = r"""
async (p) => {
  const r = await fetch('/product/searchList', {
    method: 'POST',
    headers: {'content-type': 'application/json', 'country': p.country, 'currency': p.currency, 'language': 'en-US'},
    body: JSON.stringify(p.payload),
  });
  const text = await r.text();
  return {status: r.status, body: text};
}
"""


def main() -> int:
    ap = argparse.ArgumentParser(description="Kalodata keyword search via browser login")
    ap.add_argument("--email", required=True)
    ap.add_argument("--password", required=True)
    ap.add_argument("--proxy", default="")
    ap.add_argument("--country", default="VN")
    ap.add_argument("--currency", default="USD")
    ap.add_argument("--query", required=True)
    ap.add_argument("--start-date", default="")
    ap.add_argument("--end-date", default="")
    ap.add_argument("--pages", type=int, default=1)
    ap.add_argument("--page-size", type=int, default=10)
    ap.add_argument("--headed", action="store_true")
    args = ap.parse_args()

    if not args.query:
        print("error: --query is required", file=sys.stderr)
        return EXIT_USAGE

    camoufox_kwargs = {"headless": not args.headed, "os": "linux", "humanize": True}
    proxy = build_proxy(args.proxy)
    if proxy:
        camoufox_kwargs["proxy"] = proxy
        camoufox_kwargs["geoip"] = True

    all_items = []
    with Camoufox(**camoufox_kwargs) as b:
        page = b.new_page()
        if not do_login(page, args.email, args.password):
            return EXIT_LOGIN_FAIL

        page.goto("https://www.kalodata.com/product", wait_until="domcontentloaded", timeout=60000)
        page.wait_for_timeout(4000)

        for page_no in range(1, max(1, args.pages) + 1):
            payload = {
                "country": args.country.upper(),
                "startDate": args.start_date,
                "endDate": args.end_date,
                "cateIds": [],
                "showCateIds": [],
                "pageNo": page_no,
                "pageSize": args.page_size,
                "query": args.query,
            }
            res = page.evaluate(SEARCH_JS, {"country": args.country.upper(), "currency": args.currency, "payload": payload})
            if res["status"] != 200 or not res["body"].strip():
                print(f"searchList page {page_no}: status {res['status']}, {len(res['body'])} bytes", file=sys.stderr)
                break
            try:
                data = json.loads(res["body"]).get("data") or []
            except Exception as e:
                print(f"searchList page {page_no}: decode error {e}", file=sys.stderr)
                break
            if not data:
                break
            all_items.extend(data)
            page.wait_for_timeout(1200)

    json.dump({"data": all_items}, sys.stdout, ensure_ascii=False)
    sys.stdout.write("\n")
    return EXIT_OK


if __name__ == "__main__":
    sys.exit(main())
