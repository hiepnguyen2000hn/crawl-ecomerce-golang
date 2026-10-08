"""
Minimal smoke test: open ONE CloakBrowser instance, navigate to the
Outlook/Microsoft login page, and leave the browser open so you can take
over manually (enter credentials, solve a captcha/OTP, inspect selectors,
etc). This script never closes the browser itself -- you close the window
yourself when you're done, or Ctrl+C the script (which still leaves the
already-open window alone; it just stops the Python process).

Usage:
    python test_open_outlook_login.py [proxy_url]

    proxy_url   optional, e.g. http://user:pass@host:port

Env:
    PLAYWRIGHT_HEADLESS       default false -- must be headed since the
                              whole point is to hand off to a human.
    CLOAKBROWSER_HUMANIZE     default true.
    OUTLOOK_LOGIN_URL         default https://login.microsoftonline.com/
"""
import os
import sys

from cloakbrowser import launch

LOGIN_URL = os.getenv("OUTLOOK_LOGIN_URL", "https://login.microsoftonline.com/")
HEADLESS = os.getenv("PLAYWRIGHT_HEADLESS", "false").strip().lower() not in ("0", "false", "no")
HUMANIZE = os.getenv("CLOAKBROWSER_HUMANIZE", "true").strip().lower() not in ("0", "false", "no")
NAV_TIMEOUT_MS = int(os.getenv("NAV_TIMEOUT_MS", "30000"))


def main() -> int:
    proxy = sys.argv[1] if len(sys.argv) > 1 else None

    print(f"[+] Launching CloakBrowser (headless={HEADLESS}, humanize={HUMANIZE}, proxy={proxy or 'direct'})",
          file=sys.stderr)
    browser = launch(headless=HEADLESS, humanize=HUMANIZE, proxy=proxy)
    page = browser.new_page()

    print(f"[+] Navigating to {LOGIN_URL}", file=sys.stderr)
    page.goto(LOGIN_URL, wait_until="domcontentloaded", timeout=NAV_TIMEOUT_MS)

    print("[+] Browser is open at the login page and will stay open.", file=sys.stderr)
    print("[+] Take over manually now. Press Ctrl+C here to stop this script", file=sys.stderr)
    print("    (the browser window itself will remain open).", file=sys.stderr)

    try:
        while True:
            page.wait_for_timeout(60_000)
    except KeyboardInterrupt:
        print("\n[+] Script stopped; browser window left open as requested.", file=sys.stderr)

    return 0


if __name__ == "__main__":
    sys.exit(main())
