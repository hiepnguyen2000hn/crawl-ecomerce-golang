"""
Open N CloakBrowser instances concurrently, each bound to a different proxy
from a Webshare-style proxy list file, and confirm the browser's egress IP
actually matches the proxy (via https://ifconfig.me/ip). If a proxy fails to
connect, falls back to a direct (no-proxy) launch for that slot so the run
still produces a result per browser instead of hard-failing.

Proxy file format (one per line, tab- or whitespace-separated index optional):
    host:port:username:password

Usage:
    python multi_browser_proxy_test.py [path/to/proxies.txt] [count]

    path/to/proxies.txt  defaults to "Webshare 100 proxies.txt" in this dir
                          (or pass an absolute path, e.g. to ~/Downloads)
    count                 how many browsers to open concurrently (default 3)

Env:
    PLAYWRIGHT_HEADLESS   true/1 (default) runs headless; false/0 opens real
                          visible Chromium windows (needs a DISPLAY) and
                          keeps each one open for HEADED_HOLD_SECONDS so you
                          can see it before it closes.

Output:
    ./multi_browser_proxy_test_output.json
"""
import json
import os
import re
import sys
from concurrent.futures import ThreadPoolExecutor, as_completed
from pathlib import Path

from cloakbrowser import launch

IP_CHECK_URL = "https://whatismyipaddress.com/"
IP_PATTERN = re.compile(r"IPv4:.*?(\d{1,3}(?:\.\d{1,3}){3})", re.DOTALL)
NAV_TIMEOUT_MS = 20000
DEFAULT_COUNT = 3
OUTPUT_FILE = "multi_browser_proxy_test_output.json"
HEADLESS = os.getenv("PLAYWRIGHT_HEADLESS", "true").strip().lower() not in ("0", "false", "no")
HEADED_HOLD_SECONDS = int(os.getenv("HEADED_HOLD_SECONDS", "30"))

PROXY_LINE_RE = re.compile(
    r"^\s*(?:\d+\s+)?(?P<host>[^:\s]+):(?P<port>\d+):(?P<user>[^:\s]+):(?P<pass>[^:\s]+)\s*$"
)


def load_proxies(path: Path) -> list[dict]:
    proxies = []
    for line in path.read_text(encoding="utf-8").splitlines():
        if not line.strip():
            continue
        m = PROXY_LINE_RE.match(line)
        if not m:
            print(f"[!] Skipping unparseable line: {line!r}", file=sys.stderr)
            continue
        proxies.append(m.groupdict())
    return proxies


def proxy_url(p: dict) -> str:
    return f"http://{p['user']}:{p['pass']}@{p['host']}:{p['port']}"


def check_one(index: int, proxy: dict) -> dict:
    """Launch a browser against `proxy`; on any failure, retry once with no
    proxy (direct connection) so the slot still reports a result."""
    label = f"{proxy['host']}:{proxy['port']}"
    result = {"slot": index, "proxy": label, "mode": "proxy", "ok": False}

    for attempt_mode, proxy_arg in (("proxy", proxy_url(proxy)), ("direct", None)):
        browser = None
        try:
            browser = launch(headless=HEADLESS, proxy=proxy_arg)
            page = browser.new_page()
            page.goto(IP_CHECK_URL, wait_until="domcontentloaded", timeout=NAV_TIMEOUT_MS)
            page.wait_for_timeout(1500)  # let the IPv4 widget finish its lookup
            body_text = page.evaluate("() => document.body.innerText")
            m = IP_PATTERN.search(body_text)
            seen_ip = m.group(1) if m else None

            result["mode"] = attempt_mode
            result["seen_ip"] = seen_ip
            result["ok"] = True
            if attempt_mode == "proxy":
                result["ip_matches_proxy_host"] = seen_ip == proxy["host"]
            else:
                result["fallback_reason"] = result.get("error")
                result.pop("error", None)

            if not HEADLESS:
                page.wait_for_timeout(HEADED_HOLD_SECONDS * 1000)
            break
        except Exception as e:
            result["error"] = f"{attempt_mode} attempt failed: {e}"
            print(f"[!] slot {index} ({label}) {attempt_mode} attempt failed: {e}", file=sys.stderr)
            continue
        finally:
            if browser is not None:
                browser.close()

    return result


def main() -> int:
    proxy_file = Path(sys.argv[1]) if len(sys.argv) > 1 else Path(__file__).parent / "Webshare 100 proxies.txt"
    count = int(sys.argv[2]) if len(sys.argv) > 2 else DEFAULT_COUNT

    if not proxy_file.exists():
        print(f"[!] Proxy file not found: {proxy_file}", file=sys.stderr)
        return 1

    proxies = load_proxies(proxy_file)
    if not proxies:
        print(f"[!] No valid proxy lines parsed from {proxy_file}", file=sys.stderr)
        return 1

    selected = proxies[:count]
    print(f"[+] Opening {len(selected)} browser(s) against {proxy_file.name}", file=sys.stderr)

    results = []
    with ThreadPoolExecutor(max_workers=len(selected)) as pool:
        futures = {pool.submit(check_one, i, p): i for i, p in enumerate(selected, start=1)}
        for fut in as_completed(futures):
            results.append(fut.result())

    results.sort(key=lambda r: r["slot"])

    with open(OUTPUT_FILE, "w", encoding="utf-8") as f:
        json.dump(results, f, ensure_ascii=False, indent=2)

    ok_count = sum(1 for r in results if r["ok"])
    print(f"[+] {ok_count}/{len(results)} browsers reached {IP_CHECK_URL} -> {OUTPUT_FILE}", file=sys.stderr)
    for r in results:
        status = "OK" if r["ok"] else "FAILED"
        print(f"    slot {r['slot']} [{r['mode']}] {r['proxy']}: {status} seen_ip={r.get('seen_ip')}", file=sys.stderr)

    return 0 if ok_count == len(results) else 1


if __name__ == "__main__":
    sys.exit(main())
