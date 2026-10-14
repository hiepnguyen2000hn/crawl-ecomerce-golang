"""
Crawl allegro.pl search listings.

Opens `https://allegro.pl/listing?string=<keyword>` page by page and extracts
one record per product card:

    product_id, offer_id, title, url, price, price_with_delivery, currency,
    rating, review_count, bought_recently, super_seller, smart, delivery,
    sponsored, position, page

## Which browser engine, and why it matters

Allegro is behind DataDome. Measured on this machine (Hanoi residential IP),
against `https://allegro.pl/`:

    Chrome 149 HEADED, blank profile, navigator.webdriver = TRUE  -> PASSES
    Chrome 149 HEADLESS, otherwise identical                      -> BLOCKED
    CloakBrowser stealth Chromium 146, every stealth knob on      -> BLOCKED

That is not a typo. Undisguised automation gets through and the stealth
browser does not, so ENGINE defaults to `chrome` -- and PLAYWRIGHT_HEADLESS
must be false, which is the one hard requirement here. On a server without a
display, run the whole thing under `xvfb-run`.

CloakBrowser's free tier ships Chromium **146** and spoofs a Windows UA while
running on Linux; DataDome scores a three-major-versions-stale Chrome claiming
the wrong OS worse than it scores an honest, current, obviously-automated one.

ENGINE=cloakbrowser is still supported (proxy rotation, humanized input,
per-account sessions) -- just expect to need a newer binary (`cloakbrowser
login` unlocks the free v152) before Allegro lets it through.

Usage:
    python3 -m venv .venv && ./.venv/bin/pip install -r requirements.txt
    ./.venv/bin/playwright install chrome      # only if Chrome isn't installed
    cp .env.example .env

    python allegro_crawl.py laptop                 # 1 page of "laptop"
    python allegro_crawl.py "fotel gamingowy" -p 3 # 3 pages
    python allegro_crawl.py laptop -o laptops.json
    python allegro_crawl.py --open                 # 1 visible browser, stays open
    python allegro_crawl.py --solve                # save a session by hand

Output:
    ./allegro-<query>-<timestamp>.json  - {"query", "pages_fetched", "blocked", "offers": [...]}
    ./debug/                      - HTML + screenshot of any page we could not parse
"""
import argparse
import json
import os
import re
import sys
from contextlib import contextmanager
from datetime import datetime
from pathlib import Path
from urllib.parse import urlencode

from dotenv import load_dotenv

HERE = Path(__file__).parent
load_dotenv(HERE / ".env")

LISTING_URL = "https://allegro.pl/listing"
DEBUG_DIR = HERE / "debug"

ENGINE = (os.getenv("ENGINE") or "chrome").strip().lower()
HEADLESS = os.getenv("PLAYWRIGHT_HEADLESS", "true").strip().lower() not in ("0", "false", "no")
HUMANIZE = os.getenv("CLOAKBROWSER_HUMANIZE", "true").strip().lower() not in ("0", "false", "no")
PROXY = os.getenv("PROXY") or None
SESSION_FILE = HERE / (os.getenv("SESSION_FILE") or "session.json")
# Empty by default on purpose: spoofing pl-PL/Europe/Warsaw while egressing
# from a Vietnamese IP is a fingerprint contradiction, not a disguise. Only
# set these when PROXY actually exits in Poland.
LOCALE = os.getenv("LOCALE") or None
TIMEZONE = os.getenv("TIMEZONE") or None
NAV_TIMEOUT_MS = int(os.getenv("NAV_TIMEOUT_MS", "60000"))
SETTLE_MS = int(os.getenv("SETTLE_MS", "3000"))
PAGE_DELAY_MS = int(os.getenv("PAGE_DELAY_MS", "2500"))
LOAD_RETRIES = int(os.getenv("LOAD_RETRIES", "10"))

CONSENT_SELECTORS = (
    '[data-role="accept-consent"]',
    'button[data-role="accept-consent"]',
    'button:has-text("OK, zgadzam się")',
    'button:has-text("Przejdź do serwisu")',
)

# DataDome serves a ~1.5KB shell whose only real content is an iframe pointing
# at *.captcha-delivery.com -- that string is the reliable tell.
BLOCK_MARKERS = ("captcha-delivery.com", "DataDome Device Check", "geo.captcha-delivery")

# A fully rendered listing is ~2.9MB; anything near the 1.5KB shell is a block
# or a half-loaded page.
MIN_LISTING_BYTES = 200_000

# Every field below comes off an aria-label rather than a class name. Allegro
# ships hashed CSS classes that churn constantly, but the accessibility labels
# are stable -- "899,00 zł aktualna cena", "4,54 na 5, 13 ocen produktu".
EXTRACT_JS = r"""
() => {
  const clean = (s) => (s || '').replace(/\s+/g, ' ').trim();
  const money = (s) => {
    if (!s) return null;
    const m = clean(s).match(/(\d[\d  .]*,\d{2}|\d[\d  .]*)\s*(?:zł|PLN)/);
    if (!m) return null;
    const v = parseFloat(m[1].replace(/[  .]/g, '').replace(',', '.'));
    return Number.isFinite(v) ? v : null;
  };
  const aria = (card, re) =>
    Array.from(card.querySelectorAll('[aria-label]'))
      .map((e) => e.getAttribute('aria-label'))
      .find((l) => re.test(l)) || null;

  const cards = Array.from(document.querySelectorAll('article'))
    .filter((a) => a.querySelector('a[href*="/produkt/"], a[href*="/oferta/"]'));

  return cards.map((card, i) => {
    const link = card.querySelector('a[href*="/produkt/"], a[href*="/oferta/"]');
    const raw = link ? link.href : null;
    let url = null, productId = null, offerId = null;
    if (raw) {
      const u = new URL(raw);
      offerId = u.searchParams.get('offerId');
      url = u.origin + u.pathname;
      const idm = u.pathname.match(/([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})$/);
      productId = idm ? idm[1] : null;
    }

    const h = card.querySelector('h2, h3');
    const img = card.querySelector('img');
    const title = clean(h ? h.innerText : '') || (img ? clean(img.alt) : null) || null;

    const priceLabel = aria(card, /aktualna cena/);
    const withDelivery = Array.from(card.querySelectorAll('p'))
      .map((e) => clean(e.textContent))
      .find((t) => /z dostawą/.test(t)) || null;

    const ratingLabel = aria(card, /na 5/);
    const rm = ratingLabel ? ratingLabel.match(/([\d,]+)\s*na 5,\s*(\d+)\s*ocen/) : null;

    const boughtLabel = aria(card, /kupiło ostatnio/);
    const bm = boughtLabel ? boughtLabel.match(/(\d+)/) : null;

    const deliveryLabel = aria(card, /^dostawa/);

    return {
      position: i + 1,
      product_id: productId,
      offer_id: offerId,
      title,
      url,
      price: money(priceLabel),
      price_with_delivery: money(withDelivery),
      currency: priceLabel ? 'PLN' : null,
      rating: rm ? parseFloat(rm[1].replace(',', '.')) : null,
      review_count: rm ? parseInt(rm[2], 10) : null,
      bought_recently: bm ? parseInt(bm[1], 10) : null,
      super_seller: !!aria(card, /Super Sprzedawca/),
      smart: !!aria(card, /Allegro Smart/),
      delivery: deliveryLabel ? clean(deliveryLabel.replace(/\s*zobacz szczegóły\s*$/, '')) : null,
      sponsored: !!aria(card, /Promowane|Reklama/),
    };
  });
}
"""


@contextmanager
def browser_session(headless: bool, use_proxy: bool = True):
    """Yield a started browser for whichever ENGINE is configured.

    Both engines expose Playwright's `new_page` / `new_context`, so everything
    downstream of here is engine-agnostic.
    """
    if ENGINE == "cloakbrowser":
        from cloakbrowser import launch

        kwargs = dict(headless=headless, humanize=HUMANIZE)
        if LOCALE:
            kwargs["locale"] = LOCALE
        if TIMEZONE:
            kwargs["timezone"] = TIMEZONE
        if PROXY and use_proxy:
            kwargs["proxy"] = PROXY
            kwargs["geoip"] = True  # keep the fingerprint's geo on the proxy's exit
        browser = launch(**kwargs)
        try:
            yield browser
        finally:
            browser.close()
        return

    from playwright.sync_api import sync_playwright

    with sync_playwright() as pw:
        kwargs = dict(channel="chrome", headless=headless)
        if PROXY and use_proxy:
            kwargs["proxy"] = {"server": PROXY}
        browser = pw.chromium.launch(**kwargs)
        try:
            yield browser
        finally:
            browser.close()


def new_context(browser):
    """Reuse a saved session if we have one -- that's what carries a cleared
    DataDome cookie across runs."""
    opts = {}
    if LOCALE:
        opts["locale"] = LOCALE
    if TIMEZONE:
        opts["timezone_id"] = TIMEZONE
    if SESSION_FILE.exists():
        print(f"[+] Reusing session {SESSION_FILE.name}", file=sys.stderr)
        opts["storage_state"] = str(SESSION_FILE)
    return browser.new_context(**opts)


def goto(page, url: str) -> None:
    """Navigate without insisting on a load event.

    DataDome interrupts the first navigation to bounce us through
    `?dd_referrer=`, which aborts `domcontentloaded` and raises. The redirect
    is the normal path in, not a failure, so commit and let settle() wait.
    """
    try:
        page.goto(url, wait_until="commit", timeout=NAV_TIMEOUT_MS)
    except Exception as e:
        print(f"    (navigation interrupted: {str(e).splitlines()[0][:70]})", file=sys.stderr)


def settle(page, min_bytes: int = MIN_LISTING_BYTES) -> str:
    """Wait for the page to stop moving and return its HTML.

    DataDome bounces you through `?dd_referrer=` on the way in, so `content()`
    raises "page is navigating" for the first few seconds, and its shell HTML
    is also what you see *mid-redirect*. Seeing the shell once therefore means
    nothing -- only still seeing it after every retry is a real block.
    """
    html = ""
    for _ in range(LOAD_RETRIES):
        page.wait_for_timeout(SETTLE_MS)
        try:
            html = page.content()
        except Exception:
            continue  # "page is navigating": DataDome is bouncing us through
        if not is_blocked(html) and len(html) >= min_bytes:
            return html
    return html


def dismiss_consent(page) -> None:
    for sel in CONSENT_SELECTORS:
        try:
            loc = page.locator(sel)
            if loc.count():
                loc.first.click(timeout=3000)
                page.wait_for_timeout(1500)
                return
        except Exception:
            continue


def lazy_scroll(page, steps: int = 8) -> None:
    """Cards below the fold don't render until scrolled into view."""
    for _ in range(steps):
        page.mouse.wheel(0, 1300)
        page.wait_for_timeout(400)


def is_blocked(html: str) -> bool:
    return any(m in html for m in BLOCK_MARKERS)


def dump_debug(page, tag: str) -> None:
    DEBUG_DIR.mkdir(exist_ok=True)
    try:
        (DEBUG_DIR / f"{tag}.html").write_text(page.content(), encoding="utf-8")
        page.screenshot(path=str(DEBUG_DIR / f"{tag}.png"))
    except Exception:
        pass
    print(f"[!] Saved debug/{tag}.html (+ .png)", file=sys.stderr)


def default_output(query: str) -> Path:
    """Timestamped per query so a later blocked run can't clobber a good one.

    An earlier version wrote to one fixed path and a follow-up run destroyed a
    successful 122-record crawl. Don't do that again.
    """
    slug = re.sub(r"[^a-z0-9]+", "-", query.lower()).strip("-") or "query"
    stamp = datetime.now().strftime("%Y%m%d-%H%M%S")
    return HERE / f"allegro-{slug}-{stamp}.json"


def listing_url(query: str, page_no: int) -> str:
    params = {"string": query}
    if page_no > 1:
        params["p"] = page_no
    return f"{LISTING_URL}?{urlencode(params)}"


def crawl(query: str, pages: int) -> dict:
    result = {
        "engine": ENGINE,
        "query": query,
        "pages_requested": pages,
        "pages_fetched": 0,
        "blocked": False,
        "offers": [],
    }
    with browser_session(HEADLESS) as browser:
        ctx = new_context(browser)
        page = ctx.new_page()

        for page_no in range(1, pages + 1):
            url = listing_url(query, page_no)
            print(f"[+] page {page_no}/{pages}: {url}", file=sys.stderr)
            goto(page, url)
            html = settle(page)

            if is_blocked(html):
                result["blocked"] = True
                result["block_reason"] = (
                    f"DataDome blocked engine={ENGINE}. See debug/blocked-p{page_no}.png. "
                    "ENGINE=chrome is the combination measured to get through; on "
                    "cloakbrowser, run `cloakbrowser login` for the newer v152 binary."
                )
                dump_debug(page, f"blocked-p{page_no}")
                break

            if page_no == 1:
                dismiss_consent(page)
            lazy_scroll(page)

            offers = page.evaluate(EXTRACT_JS)
            if not offers:
                dump_debug(page, f"empty-p{page_no}")
            for o in offers:
                o["page"] = page_no
            result["offers"].extend(offers)
            result["pages_fetched"] = page_no
            print(f"    -> {len(offers)} offers", file=sys.stderr)

            if page_no < pages:
                page.wait_for_timeout(PAGE_DELAY_MS)
    return result


def solve_session(query: str = "laptop") -> int:
    """Open a real window, let the human clear the captcha, then prove the
    session actually works on a listing page before persisting it.

    Saving a storage_state that still carries a challenge is worse than not
    saving one, because every later run would silently reuse it -- hence the
    verification step instead of trusting the Enter keypress.
    """
    with browser_session(headless=False) as browser:
        ctx = new_context(browser)
        page = ctx.new_page()
        goto(page, "https://allegro.pl/")
        settle(page, min_bytes=1)  # let the challenge render before prompting

        print(
            "\n[>] Solve the captcha in the browser window, accept the cookie\n"
            "    banner, and wait until you can see the normal Allegro page.\n"
            "    Then come back here and press Enter.",
            file=sys.stderr,
        )
        input()

        print(f"[+] Verifying on a real listing (string={query})...", file=sys.stderr)
        goto(page, listing_url(query, 1))
        html = settle(page)
        dismiss_consent(page)
        lazy_scroll(page, steps=3)
        cards = len(page.evaluate(EXTRACT_JS))

        if is_blocked(html) or cards == 0:
            dump_debug(page, "solve-failed")
            print(
                f"[!] Still challenged (blocked={is_blocked(html)}, cards={cards}). "
                "Session NOT saved -- nothing was overwritten.",
                file=sys.stderr,
            )
            return 1

        ctx.storage_state(path=str(SESSION_FILE))
        print(f"[+] Verified: {cards} cards parsed. Session saved to {SESSION_FILE.name}", file=sys.stderr)
        print("[+] Later runs reuse it automatically. Delete the file to start over.", file=sys.stderr)
        return 0


def open_once(url: str) -> int:
    """One visible browser, no proxy, straight to `url`, left open so you can
    see with your own eyes what Allegro serves. Closes only on Enter."""
    with browser_session(headless=False, use_proxy=False) as browser:
        page = new_context(browser).new_page()
        print(f"[+] engine={ENGINE}, 1 browser, no proxy -> {url}", file=sys.stderr)
        goto(page, url)
        html = settle(page, min_bytes=MIN_LISTING_BYTES)

        blocked = is_blocked(html)
        print(f"\n    url     : {page.url}", file=sys.stderr)
        print(f"    title   : {page.title()!r}", file=sys.stderr)
        print(f"    html    : {len(html)} bytes", file=sys.stderr)
        print(f"    cards   : {page.locator('article').count()}", file=sys.stderr)
        print(f"    blocked : {blocked}", file=sys.stderr)
        dump_debug(page, "open")

        print("\n[>] Browser stays open. Press Enter here to close it.", file=sys.stderr)
        input()
        return 2 if blocked else 0


def main() -> int:
    ap = argparse.ArgumentParser(description="Crawl allegro.pl listings")
    ap.add_argument("query", nargs="?", help="search keyword, e.g. laptop")
    ap.add_argument("-p", "--pages", type=int, default=1, help="how many listing pages (default 1)")
    ap.add_argument("-o", "--out", default=None, help="output JSON path (default: allegro-<query>-<timestamp>.json)")
    ap.add_argument("--solve", action="store_true", help="headed run: clear the bot check once and save the session")
    ap.add_argument(
        "--open",
        nargs="?",
        const="https://allegro.pl/",
        metavar="URL",
        help="open ONE visible browser (no proxy) at URL and leave it open until you press Enter",
    )
    args = ap.parse_args()

    if args.open:
        return open_once(args.open)
    if args.solve:
        return solve_session(args.query or "laptop")
    if not args.query:
        ap.error("query is required (or pass --open / --solve)")

    result = crawl(args.query, args.pages)
    out = Path(args.out) if args.out else default_output(args.query)
    out.write_text(json.dumps(result, ensure_ascii=False, indent=2), encoding="utf-8")
    args.out = str(out)

    n = len(result["offers"])
    print(f"\n[+] {n} offers from {result['pages_fetched']} page(s) -> {args.out}", file=sys.stderr)
    for o in result["offers"][:10]:
        print(f"    {o['price']} zł | {(o['title'] or '')[:62]}", file=sys.stderr)
    if result["blocked"]:
        print(f"\n[!] BLOCKED: {result['block_reason']}", file=sys.stderr)
        return 2
    return 0 if n else 1


if __name__ == "__main__":
    sys.exit(main())
