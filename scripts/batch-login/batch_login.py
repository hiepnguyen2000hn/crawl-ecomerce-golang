"""
Batch username/password login for CloakBrowser across many of our own
crawler accounts, so logging into each browser profile by hand isn't
needed every time.

For each row in the accounts CSV:
  1. Skip it if a session file already exists in ./sessions/<username>.json
     (unless SKIP_IF_SESSION_EXISTS=false) -- no point re-logging-in.
  2. Otherwise launch a CloakBrowser instance (optionally through that
     account's own proxy), fill the login form using the selectors from
     .env, submit, and wait for a success indicator.
  3. On success, dump context.storage_state() to ./sessions/<username>.json
     so the crawler can reuse the session later via
     browser.new_context(storage_state="sessions/<username>.json")
     instead of logging in again.

Runs accounts concurrently (CONCURRENCY in .env) -- NOT all 100 at once;
pick a concurrency the target site's rate limits / your proxy pool / your
machine's RAM can actually sustain.

Usage:
    cp .env.example .env        # fill in LOGIN_URL + form selectors
    cp accounts.csv.example accounts.csv   # fill in real username,password,proxy
    pip install -r requirements.txt
    python batch_login.py [path/to/accounts.csv]

Output:
    ./sessions/<username>.json        - one storage_state per account
    ./batch_login_output.json         - ok/fail summary (no credentials)
"""
import csv
import json
import os
import re
import sys
import time
from concurrent.futures import ThreadPoolExecutor, as_completed
from pathlib import Path

from cloakbrowser import launch
from dotenv import load_dotenv

load_dotenv()

HERE = Path(__file__).parent
SESSIONS_DIR = HERE / "sessions"
OUTPUT_FILE = HERE / "batch_login_output.json"

LOGIN_URL = os.getenv("LOGIN_URL")
USERNAME_SELECTOR = os.getenv("USERNAME_SELECTOR")
USERNAME_NEXT_SELECTOR = os.getenv("USERNAME_NEXT_SELECTOR") or None
USE_PASSWORD_SELECTOR = os.getenv("USE_PASSWORD_SELECTOR") or None
USE_PASSWORD_WAIT_MS = int(os.getenv("USE_PASSWORD_WAIT_MS", "4000"))
PASSWORD_SELECTOR = os.getenv("PASSWORD_SELECTOR")
SUBMIT_SELECTOR = os.getenv("SUBMIT_SELECTOR")
POST_LOGIN_DISMISS_SELECTOR = os.getenv("POST_LOGIN_DISMISS_SELECTOR") or None
SUCCESS_URL_CONTAINS = os.getenv("SUCCESS_URL_CONTAINS") or None
SUCCESS_SELECTOR = os.getenv("SUCCESS_SELECTOR") or None

HEADLESS = os.getenv("PLAYWRIGHT_HEADLESS", "true").strip().lower() not in ("0", "false", "no")
HUMANIZE = os.getenv("CLOAKBROWSER_HUMANIZE", "true").strip().lower() not in ("0", "false", "no")
CONCURRENCY = int(os.getenv("CONCURRENCY", "5"))
NAV_TIMEOUT_MS = int(os.getenv("NAV_TIMEOUT_MS", "30000"))
LOGIN_CONFIRM_TIMEOUT_SECONDS = int(os.getenv("LOGIN_CONFIRM_TIMEOUT_SECONDS", "20"))
SKIP_IF_SESSION_EXISTS = os.getenv("SKIP_IF_SESSION_EXISTS", "true").strip().lower() not in ("0", "false", "no")
HOLD_BROWSER_OPEN = os.getenv("HOLD_BROWSER_OPEN", "true").strip().lower() not in ("0", "false", "no")

_SAFE_CHARS_RE = re.compile(r"[^A-Za-z0-9_.-]+")


def safe_filename(username: str) -> str:
    return _SAFE_CHARS_RE.sub("_", username.strip()) or "account"


def load_accounts(path: Path) -> list[dict]:
    with path.open(encoding="utf-8-sig", newline="") as f:
        reader = csv.DictReader(f)
        missing = {"username", "password"} - set(reader.fieldnames or [])
        if missing:
            raise ValueError(f"accounts CSV missing required column(s): {sorted(missing)}")
        return [row for row in reader if row.get("username")]


def login_one(account: dict, index: int, total: int) -> dict:
    username = account["username"].strip()
    result = {"slot": index, "username": username, "ok": False}
    session_path = SESSIONS_DIR / f"{safe_filename(username)}.json"

    if SKIP_IF_SESSION_EXISTS and session_path.exists():
        result["ok"] = True
        result["skipped"] = True
        result["session_file"] = str(session_path)
        return result

    proxy = (account.get("proxy") or "").strip() or None
    browser = None
    try:
        browser = launch(headless=HEADLESS, humanize=HUMANIZE, proxy=proxy)
        context = browser.new_context()
        page = context.new_page()

        page.goto(LOGIN_URL, wait_until="domcontentloaded", timeout=NAV_TIMEOUT_MS)
        page.fill(USERNAME_SELECTOR, username, timeout=NAV_TIMEOUT_MS)

        # Two-step forms (email page -> Next -> separate password page, e.g.
        # Microsoft/Google-style login) click an intermediate button before
        # the password field even exists in the DOM.
        if USERNAME_NEXT_SELECTOR:
            page.click(USERNAME_NEXT_SELECTOR, timeout=NAV_TIMEOUT_MS)

        # Microsoft sometimes interposes a "Verify your email" / pick-a-method
        # screen instead of going straight to the password field (it offers
        # "Send code" by default). If that screen shows up, click "Use your
        # password" to fall back to the normal password flow. Best-effort --
        # fine if the password field was already there instead.
        if USE_PASSWORD_SELECTOR:
            try:
                page.click(USE_PASSWORD_SELECTOR, timeout=USE_PASSWORD_WAIT_MS)
            except Exception:
                pass

        page.fill(PASSWORD_SELECTOR, account["password"], timeout=NAV_TIMEOUT_MS)
        page.click(SUBMIT_SELECTOR, timeout=NAV_TIMEOUT_MS)

        # Some providers show a post-login interstitial ("Stay signed in?")
        # that isn't part of the success condition but blocks the redirect
        # until dismissed. Best-effort only -- fine if it never appears.
        if POST_LOGIN_DISMISS_SELECTOR:
            try:
                page.click(POST_LOGIN_DISMISS_SELECTOR, timeout=5000)
            except Exception:
                pass

        deadline = time.time() + LOGIN_CONFIRM_TIMEOUT_SECONDS
        confirmed = False
        while time.time() < deadline:
            if SUCCESS_URL_CONTAINS and SUCCESS_URL_CONTAINS in page.url:
                confirmed = True
                break
            if SUCCESS_SELECTOR:
                try:
                    if page.is_visible(SUCCESS_SELECTOR):
                        confirmed = True
                        break
                except Exception:
                    pass
            # Microsoft reuses the same submit button (id="idSIButton9") for
            # extra interstitials after the password step (e.g. "Help us
            # protect your account" -> Next). If it's showing again, click
            # through it instead of sitting there until the deadline.
            if SUBMIT_SELECTOR:
                try:
                    if page.is_visible(SUBMIT_SELECTOR):
                        page.click(SUBMIT_SELECTOR, timeout=2000)
                except Exception:
                    pass
            page.wait_for_timeout(500)

        if not confirmed:
            result["error"] = f"no success indicator within {LOGIN_CONFIRM_TIMEOUT_SECONDS}s (final url: {page.url})"
            return result

        SESSIONS_DIR.mkdir(exist_ok=True)
        context.storage_state(path=str(session_path))
        result["ok"] = True
        result["session_file"] = str(session_path)
    except Exception as e:
        result["error"] = str(e)
        print(f"[!] slot {index}/{total} ({username}) failed: {e}", file=sys.stderr)
    finally:
        # In a headed run, leave the browser open regardless of outcome --
        # don't auto-close just because a success/failure indicator was hit.
        # It only closes once you press Enter here (or you kill the
        # terminal/script, e.g. Ctrl+C). Doesn't apply headless -- nothing
        # to look at there.
        if browser is not None:
            if not HEADLESS and HOLD_BROWSER_OPEN:
                status = "OK" if result["ok"] else "FAILED"
                try:
                    input(f"[?] slot {index}/{total} ({username}) {status} -- browser left open, "
                          f"press Enter here to close it (or Ctrl+C to leave it and stop the script): ")
                except (EOFError, KeyboardInterrupt):
                    print(f"\n[+] slot {index}/{total} ({username}): leaving browser open, not closing it.",
                          file=sys.stderr)
                    return result
            browser.close()

    return result


def main() -> int:
    if not all([LOGIN_URL, USERNAME_SELECTOR, PASSWORD_SELECTOR, SUBMIT_SELECTOR]):
        print("[!] Missing LOGIN_URL/USERNAME_SELECTOR/PASSWORD_SELECTOR/SUBMIT_SELECTOR in .env "
              "(cp .env.example .env and fill in the target site's form).", file=sys.stderr)
        return 1
    if not SUCCESS_URL_CONTAINS and not SUCCESS_SELECTOR:
        print("[!] Set SUCCESS_URL_CONTAINS or SUCCESS_SELECTOR in .env so the script can tell "
              "a login actually succeeded.", file=sys.stderr)
        return 1

    accounts_file = Path(sys.argv[1]) if len(sys.argv) > 1 else HERE / "accounts.csv"
    if not accounts_file.exists():
        print(f"[!] Accounts file not found: {accounts_file}", file=sys.stderr)
        return 1

    accounts = load_accounts(accounts_file)
    if not accounts:
        print(f"[!] No accounts parsed from {accounts_file}", file=sys.stderr)
        return 1

    SESSIONS_DIR.mkdir(exist_ok=True)
    print(f"[+] Logging in {len(accounts)} account(s), {CONCURRENCY} at a time", file=sys.stderr)

    results = []
    with ThreadPoolExecutor(max_workers=CONCURRENCY) as pool:
        futures = {
            pool.submit(login_one, acc, i, len(accounts)): i
            for i, acc in enumerate(accounts, start=1)
        }
        for fut in as_completed(futures):
            r = fut.result()
            results.append(r)
            status = "OK" if r["ok"] else "FAILED"
            tag = " (skipped, session exists)" if r.get("skipped") else ""
            print(f"    [{r['slot']}/{len(accounts)}] {r['username']}: {status}{tag}", file=sys.stderr)

    results.sort(key=lambda r: r["slot"])
    with OUTPUT_FILE.open("w", encoding="utf-8") as f:
        json.dump(results, f, ensure_ascii=False, indent=2)

    ok_count = sum(1 for r in results if r["ok"])
    print(f"[+] {ok_count}/{len(results)} accounts logged in -> sessions in {SESSIONS_DIR}, "
          f"summary in {OUTPUT_FILE}", file=sys.stderr)

    return 0 if ok_count == len(results) else 1


if __name__ == "__main__":
    sys.exit(main())
