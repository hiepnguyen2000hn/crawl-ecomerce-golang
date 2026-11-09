#!/usr/bin/env python3
"""
Allegro listing crawler powered by Camoufox (os="linux", humanize=True) + YOLO26 (ONNX) captcha solver.
Invoked as a subprocess by `cmd/allegro-service` or directly via CLI.

Extracts full product records from Allegro search/listing pages:
  - product_id, offer_id, title, url, image_url
  - brand (Marka / Producent / parsed from title), model (Model / Kod producenta / parsed from title)
  - condition (Stan), seller (Sprzedawca / logotype)
  - price, price_with_delivery, currency
  - rating, review_count, bought_recently, super_seller, smart, delivery, sponsored
  - specs (JSON map of all <dl><dt>/<dd> technical attributes on the card: processor, RAM, disk, screen, etc.)
  - position, page

Exit codes (contract with Go `crawler.Subprocess`):
  0  OK          offers found, printed as JSON on stdout
  2  CHALLENGED  blocked by DataDome after solver retries
  3  EMPTY       real page, zero cards
  4  USAGE       bad arguments
"""
from __future__ import annotations

import argparse
import json
import os
import platform
import random
import re
import sys
import time
from pathlib import Path
from typing import Any
from urllib.parse import parse_qs, unquote, urlencode, urlparse, urlunparse

# Import YOLO26 + Camoufox solver from scripts/camoufox-yolo-captcha
REPO_ROOT = Path(__file__).resolve().parents[3]
SOLVER_DIR = REPO_ROOT / "scripts" / "camoufox-yolo-captcha"
if str(SOLVER_DIR) not in sys.path:
    sys.path.insert(0, str(SOLVER_DIR))

import cv2  # noqa: E402
import numpy as np  # noqa: E402
from camoufox.sync_api import Camoufox  # noqa: E402
from captcha_solver import YOLO26CaptchaDetector, drag_slider  # noqa: E402

BLOCK_MARKERS = (
    "captcha-delivery.com",
    "DataDome Device Check",
    "geo.captcha-delivery",
    "allegrocaptcha.com",
)

# Webshare-style proxy line: "host:port:user:pass" with an optional leading index.
_PROXY_LINE_RE = re.compile(
    r"^\s*(?:\d+\s+)?(?P<host>[^:\s]+):(?P<port>\d+):(?P<user>[^:\s]+):(?P<pw>[^:\s]+)\s*$"
)
MIN_LISTING_BYTES = 200_000
EXIT_OK, EXIT_CHALLENGED, EXIT_EMPTY, EXIT_USAGE = 0, 2, 3, 4

CONSENT_SELECTORS = (
    '[data-role="accept-consent"]',
    'button[data-role="accept-consent"]',
    'button:has-text("OK, zgadzam się")',
    'button:has-text("Przejdź do serwisu")',
)

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

  const KNOWN_BRANDS = [
    'Apple', 'Lenovo', 'HP', 'Dell', 'ASUS', 'Acer', 'MSI', 'Samsung', 'Huawei',
    'Xiaomi', 'Microsoft', 'LG', 'Gigabyte', 'Razer', 'Toshiba', 'Fujitsu',
    'Medion', 'Chuwi', 'Getac', 'Panasonic', 'Sony', 'Honor', 'Realme', 'Oppo',
    'Vivo', 'Motorola', 'Nokia', 'Garmin', 'Bose', 'JBL', 'Sony', 'Philips',
    'Bosch', 'Makita', 'DeWalt', 'Dyson', 'Karcher', 'Logitech', 'SteelSeries'
  ];

  const parseBrandAndModel = (title, specs) => {
    let brand = specs['Marka'] || specs['Producent'] || null;
    let model = specs['Model'] || specs['Kod producenta'] || specs['Seria'] || null;

    if (title) {
      // Remove leading generic category words in Polish
      const stripped = title
        .replace(/^(Laptop\s+biznesowy|Laptop\s+gamingowy|Laptop\s+2w1|Laptop\s+dotykowy|Laptop|Ultrabook|Notebook|Smartfon|Telefon|Tablet|Monitor|Komputer)\s+/i, '')
        .trim();

      if (!brand) {
        for (const kb of KNOWN_BRANDS) {
          const re = new RegExp('\\b' + kb + '\\b', 'i');
          if (re.test(stripped) || re.test(title)) {
            brand = kb;
            break;
          }
        }
      }
      if (!brand) {
        const firstToken = stripped.split(/\s+/)[0];
        if (firstToken && /^[A-Za-zÀ-ž0-9-]{2,20}$/.test(firstToken) && !/^\d/.test(firstToken)) {
          brand = firstToken;
        }
      }

      if (!model && brand) {
        const idx = stripped.toLowerCase().indexOf(brand.toLowerCase());
        if (idx !== -1) {
          const afterBrand = stripped.slice(idx + brand.length).trim();
          // Capture model tokens up to screen size (e.g. 15,6" / 14") or CPU/RAM specs
          const mMatch = afterBrand.match(/^([A-Za-z0-9][A-Za-z0-9\s.-]{1,28}?)(?=\s+(?:\d{1,2}[,.]\d|\d{1,2}\s*["']|Intel|AMD|Ryzen|Core|Celeron|Pentium|Apple\s+M|\d+\s*GB|FHD|IPS|SSD|Windows|dotykowy))/i);
          if (mMatch && clean(mMatch[1])) {
            model = clean(mMatch[1]);
          } else {
            const words = afterBrand.split(/\s+/).slice(0, 3).join(' ');
            if (words) model = words;
          }
        }
      }
    }
    return { brand, model };
  };

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
    const imgs = Array.from(card.querySelectorAll('img'));
    const productImg = imgs.find((im) => im.src && /allegroimg\.com\/s\d+\//.test(im.src)) || imgs[0] || null;
    const imageUrl = productImg ? productImg.src : null;
    const title = clean(h ? h.innerText : '') || (productImg ? clean(productImg.alt) : null) || null;

    // Extract all <dl><dt>...<dd>... parameter pairs on the card
    const specs = {};
    for (const dl of card.querySelectorAll('dl')) {
      const dts = Array.from(dl.querySelectorAll('dt')).map((e) => clean(e.innerText));
      const dds = Array.from(dl.querySelectorAll('dd')).map((e) => clean(e.innerText));
      for (let j = 0; j < Math.min(dts.length, dds.length); j++) {
        if (dts[j] && dds[j]) {
          specs[dts[j]] = dds[j];
        }
      }
    }

    const { brand, model } = parseBrandAndModel(title, specs);
    const condition = specs['Stan'] || null;

    // Seller logotype if present
    const sellerImg = imgs.find((im) => im.src && im.src.includes('seller-extras') && im.alt);
    const seller = sellerImg ? clean(sellerImg.alt) : null;

    const priceLabel = aria(card, /aktualna cena/);
    const withDelivery = Array.from(card.querySelectorAll('p, span, div'))
      .map((e) => clean(e.textContent))
      .find((t) => /^\d[\d\s.,]*\s*zł\s+z dostawą$/.test(t)) || null;

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
      brand,
      model,
      condition,
      seller,
      url,
      image_url: imageUrl,
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
      specs,
    };
  });
}
"""


def select_top_comments(comments: list[dict[str, Any]], max_n: int) -> list[dict[str, Any]]:
    """Rank reviews by helpful/vote count (missing -> 0), stable, cap at max_n,
    and stamp 1-based position. Pure, unit-testable (no browser)."""
    def helpful_key(c: dict[str, Any]) -> float:
        hc = c.get("helpful_count")
        return float(hc) if isinstance(hc, (int, float)) else 0.0

    ranked = sorted(comments, key=helpful_key, reverse=True)
    out: list[dict[str, Any]] = []
    for i, c in enumerate(ranked[: max(0, max_n)]):
        item = dict(c)
        item["position"] = i + 1
        out.append(item)
    return out


def comment_targets(offers: list[dict[str, Any]], max_products: int) -> list[dict[str, Any]]:
    """Pick which offers get their comments scraped: those with a product url,
    capped at max_products. Bounds the comment phase so it cannot run away and
    jeopardise the already-collected listing data (best-effort guarantee)."""
    if max_products <= 0:
        return []
    out: list[dict[str, Any]] = []
    for o in offers:
        if o.get("url"):
            out.append(o)
            if len(out) >= max_products:
                break
    return out


# DOM extraction for product reviews (opinie). Heuristic/resilient: anchors on
# star-rating labels ("x na 5") and climbs to the smallest block that also holds
# review text. Selectors confirmed/refined during the implementation spike.
COMMENTS_JS = r"""
() => {
  const clean = (s) => (s || '').replace(/\s+/g, ' ').trim();
  const intOf = (s) => { const m = (s || '').match(/\d+/); return m ? parseInt(m[0], 10) : null; };

  const ratingEls = Array.from(document.querySelectorAll('[aria-label*="na 5"]'));
  const seen = new Set();
  const reviews = [];
  for (const rEl of ratingEls) {
    let node = rEl;
    for (let hops = 0; hops < 6 && node && node !== document.body; hops++) {
      node = node.parentElement;
      if (!node) break;
      const txt = clean(node.innerText);
      if (txt.length > 40 && txt.length < 2000) break;
    }
    if (!node || seen.has(node)) continue;
    seen.add(node);

    const label = rEl.getAttribute('aria-label') || '';
    const rm = label.match(/([\d,]+)\s*na 5/);
    const rating = rm ? parseFloat(rm[1].replace(',', '.')) : null;

    const full = clean(node.innerText);

    // Skip aggregate rating / CTA widgets that also carry a "x na 5" label but
    // are not individual reviews.
    if (/ocen produktu|oceny produktu|Napisz opini|wszystkie opinie|średnia ocena/i.test(full)
        && !/Treść recenzji|potwierdzona zakupem/i.test(full)) {
      continue;
    }

    const confirmed = /potwierdzona zakupem/i.test(full);

    // Strip Allegro UI labels/badges so content holds just the review body.
    let content = full;
    content = content.replace(/^.*?Treść recenzji\s*/is, '');
    content = content.replace(/\s*(Zalety|Plusy|Wady|Minusy)\b.*$/is, '');
    content = content.replace(/NAJBARDZIEJ POMOCNA/gi, '');
    content = content.replace(/Opinia potwierdzona zakupem/gi, '');
    content = clean(content) || full || null;

    // Helpful votes: Allegro renders "Czy ta opinia była pomocna?" with Tak/Nie
    // buttons carrying counts. Try the combined label first, then a "Tak (n)".
    let helpful = null;
    const texts = Array.from(node.querySelectorAll('button, span, a')).map((e) => clean(e.innerText));
    const combined = texts.find((t) => /(przydatn|pomocn)/i.test(t) && /\d/.test(t));
    if (combined) {
      helpful = intOf(combined);
    } else {
      const tak = texts.find((t) => /^Tak\b/i.test(t) && /\d/.test(t));
      if (tak) helpful = intOf(tak);
    }

    const dm = full.match(/\b(\d{1,2}\s+\p{L}+\s+\d{4}|\d{2}\.\d{2}\.\d{4})\b/u);

    const prosEl = Array.from(node.querySelectorAll('*'))
      .find((e) => /^(Zalety|Plusy)/i.test(clean(e.innerText)));
    const consEl = Array.from(node.querySelectorAll('*'))
      .find((e) => /^(Wady|Minusy)/i.test(clean(e.innerText)));

    const strong = node.querySelector('strong, h3, h4, [class*="author"], [class*="user"]');
    const author = strong ? clean(strong.innerText) : null;

    reviews.push({
      author: author || null,
      rating,
      content,
      review_date: dm ? dm[1] : null,
      confirmed_purchase: confirmed,
      helpful_count: helpful,
      pros: prosEl ? clean(prosEl.innerText) : null,
      cons: consEl ? clean(consEl.innerText) : null,
    });
  }
  return reviews;
}
"""

MORE_REVIEWS_SELECTORS = (
    'button:has-text("Pokaż więcej")',
    'button:has-text("Pokaż kolejne")',
    'button:has-text("więcej opinii")',
    'a:has-text("wszystkie opinie")',
)


def extract_comments_from_product(
    page: Any, product_url: str, max_comments: int, detector: YOLO26CaptchaDetector
) -> list[dict[str, Any]]:
    """Open a product page, settle/solve DataDome, scroll to reviews, and extract
    up to max_comments top reviews. Best-effort: any failure returns []."""
    try:
        try:
            page.goto(product_url, wait_until="commit", timeout=60000)
        except Exception as e:
            print(f"(comment nav warning: {str(e).splitlines()[0][:80]})", file=sys.stderr)

        html, _ = settle_or_solve(page, detector)
        if is_blocked(html):
            # Blocked product page: bail early instead of burning the scroll/click
            # budget, so one challenged product doesn't eat the comment phase.
            return []
        # Reviews lazy-load near the bottom of the product page; scroll down in
        # steps with waits to trigger loading before extracting.
        for _ in range(14):
            page.mouse.wheel(0, 1400)
            page.wait_for_timeout(450)
        page.wait_for_timeout(1500)

        raw: list[dict[str, Any]] = []
        for _ in range(3):
            clicked = False
            for sel in MORE_REVIEWS_SELECTORS:
                try:
                    loc = page.locator(sel)
                    if loc.count():
                        loc.first.click(timeout=2500)
                        page.wait_for_timeout(1200)
                        clicked = True
                        break
                except Exception:
                    continue
            raw = page.evaluate(COMMENTS_JS)
            if len(raw) >= max_comments or not clicked:
                break

        return select_top_comments(raw, max_comments)
    except Exception as e:
        print(f"(comment extract error for {product_url}: {str(e).splitlines()[0][:100]})", file=sys.stderr)
        return []


def host_os_name() -> str:
    sys_name = platform.system().lower()
    if "linux" in sys_name:
        return "linux"
    if "darwin" in sys_name:
        return "macos"
    return "windows"


def parse_proxy_line(line: str) -> str | None:
    """Convert a Webshare "host:port:user:pass" line (optional leading index)
    into a "http://user:pass@host:port" URL. Returns None for blank/bad lines."""
    m = _PROXY_LINE_RE.match(line or "")
    if not m:
        return None
    return f"http://{m['user']}:{m['pw']}@{m['host']}:{m['port']}"


def load_proxies(path: str) -> list[str]:
    """Load a Webshare-style proxy list file into proxy URLs, skipping bad
    lines. Missing/unreadable file -> empty list (crawl proceeds direct)."""
    try:
        text = Path(path).read_text(encoding="utf-8")
    except OSError:
        return []
    out: list[str] = []
    for line in text.splitlines():
        if not line.strip():
            continue
        url = parse_proxy_line(line)
        if url:
            out.append(url)
    return out


WEBSHARE_API_URL = "https://proxy.webshare.io/api/v2/proxy/list/?mode=direct&page=1&page_size=100"


def webshare_proxies_from_json(data: dict[str, Any]) -> list[str]:
    """Parse a Webshare `/api/v2/proxy/list/` JSON response into proxy URLs,
    skipping entries flagged invalid or missing any field. Pure/testable."""
    out: list[str] = []
    for r in data.get("results", []) or []:
        if r.get("valid") is False:
            continue
        host = r.get("proxy_address")
        port = r.get("port")
        user = r.get("username")
        pw = r.get("password")
        if host and port and user and pw:
            out.append(f"http://{user}:{pw}@{host}:{port}")
    return out


def fetch_webshare_proxies(api_key: str, url: str = WEBSHARE_API_URL, timeout: int = 20) -> list[str]:
    """Fetch the proxy pool from the Webshare API using an API key. Network
    failure -> empty list (caller falls back to file/single/direct)."""
    import urllib.error
    import urllib.request

    req = urllib.request.Request(url, headers={"Authorization": f"Token {api_key}"})
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            data = json.loads(resp.read().decode("utf-8"))
    except (urllib.error.URLError, OSError, ValueError) as e:
        print(f"(webshare api error: {str(e)[:120]})", file=sys.stderr)
        return []
    return webshare_proxies_from_json(data)


def build_proxy(proxy_url: str) -> dict[str, Any]:
    """Build a Playwright/Camoufox proxy dict from a proxy URL.

    Firefox ignores credentials embedded in the server URL, so any user:pass
    must be split out into separate username/password fields. Webshare-style
    endpoints (http://user:pass@p.webshare.io:80 or socks5://...) are supported;
    a bare host:port is assumed to be an HTTP proxy.
    """
    if "://" not in proxy_url:
        proxy_url = "http://" + proxy_url
    parsed = urlparse(proxy_url)
    server = f"{parsed.scheme}://{parsed.hostname}"
    if parsed.port:
        server += f":{parsed.port}"
    proxy: dict[str, Any] = {"server": server}
    if parsed.username:
        proxy["username"] = unquote(parsed.username)
    if parsed.password:
        proxy["password"] = unquote(parsed.password)
    return proxy


def is_blocked(html: str) -> bool:
    return any(m in html for m in BLOCK_MARKERS)


def is_slider_captcha_challenge(html: str, page: Any) -> bool:
    if "t=bv" in html or "'t':'bv'" in html:
        return False
    if "geo.captcha-delivery.com/captcha/" in html or "'rt':'c'" in html:
        return True
    for frame in page.frames:
        if "captcha-delivery.com/captcha/" in frame.url and "t=bv" not in frame.url:
            return True
    return False


def solve_datadome_captcha(page: Any, detector: YOLO26CaptchaDetector, max_attempts: int = 3) -> dict[str, Any] | None:
    for attempt in range(1, max_attempts + 1):
        page.wait_for_timeout(1800)

        target_frame = None
        for frame in page.frames:
            if "captcha-delivery.com" in frame.url:
                target_frame = frame
                break
        ctx = target_frame if target_frame is not None else page

        try:
            dom_info = ctx.evaluate(
                r"""() => {
                const bodyText = (document.body && document.body.innerText) || '';
                if (bodyText.includes('You have been blocked')) {
                    return { hardBlocked: true };
                }
                const canvas = document.querySelector('canvas, #captcha-bg, .captcha-bg, [id*="puzzle"], [class*="puzzle"]');
                const canvasRect = canvas ? canvas.getBoundingClientRect() : null;

                const allEls = Array.from(document.querySelectorAll('*'));
                let handle = document.querySelector('#slider-handle, .sliderTarget, .slider');
                let track = document.querySelector('.sliderContainer, .slider-track, #captcha-container');

                if (!handle) {
                    for (const el of allEls) {
                        const r = el.getBoundingClientRect();
                        if (r.width >= 36 && r.width <= 90 && r.height >= 28 && r.height <= 65 && r.left > 10 && r.left < window.innerWidth / 2) {
                            const bg = window.getComputedStyle(el).backgroundColor || '';
                            if (bg.includes('255, 90, 0') || bg.includes('255, 80') || bg.includes('245') || el.querySelector('svg, i')) {
                                handle = el;
                                break;
                            }
                        }
                    }
                }

                if (handle && !track) {
                    let p = handle.parentElement;
                    while (p && p !== document.body) {
                        const pr = p.getBoundingClientRect();
                        if (pr.width >= handle.getBoundingClientRect().width * 2.5 && pr.width <= 500) {
                            track = p;
                            break;
                        }
                        p = p.parentElement;
                    }
                }

                return {
                    hardBlocked: false,
                    hasCanvas: !!(canvasRect && canvasRect.width > 100 && canvasRect.height > 80),
                    canvasRect: canvasRect ? {x: canvasRect.x, y: canvasRect.y, width: canvasRect.width, height: canvasRect.height} : null,
                    handleRect: handle ? (() => { const r = handle.getBoundingClientRect(); return {x: r.x, y: r.y, width: r.width, height: r.height}; })() : null,
                    trackRect: track ? (() => { const r = track.getBoundingClientRect(); return {x: r.x, y: r.y, width: r.width, height: r.height}; })() : null,
                };
            }"""
            )
        except Exception as e:
            print(f"[!] Frame DOM inspect error (attempt {attempt}): {e}", file=sys.stderr)
            continue

        if dom_info.get("hardBlocked"):
            return {"solved": False, "reason": "hard_blocked_by_datadome"}

        frame_offset_x = 0.0
        frame_offset_y = 0.0
        if target_frame is not None:
            try:
                iframe_el = page.locator('iframe[src*="captcha-delivery.com"]').first
                ib = iframe_el.bounding_box()
                if ib:
                    frame_offset_x = ib["x"]
                    frame_offset_y = ib["y"]
            except Exception:
                pass

        handle_rect = dom_info.get("handleRect")
        track_rect = dom_info.get("trackRect")
        canvas_rect = dom_info.get("canvasRect")

        if not handle_rect:
            continue

        start_x = frame_offset_x + handle_rect["x"] + handle_rect["width"] / 2.0
        start_y = frame_offset_y + handle_rect["y"] + handle_rect["height"] / 2.0

        screenshot_bytes = page.screenshot()
        detection = detector.detect_from_bytes(screenshot_bytes)

        if dom_info.get("hasCanvas") and canvas_rect:
            clip_bytes = page.screenshot(
                clip={
                    "x": frame_offset_x + canvas_rect["x"],
                    "y": frame_offset_y + canvas_rect["y"],
                    "width": canvas_rect["width"],
                    "height": canvas_rect["height"],
                }
            )
            detection = detector.detect_from_bytes(clip_bytes)
            nparr = np.frombuffer(clip_bytes, np.uint8)
            img = cv2.imdecode(nparr, cv2.IMREAD_COLOR)
            img_w = img.shape[1] if img is not None else canvas_rect["width"]
            scale_x = canvas_rect["width"] / float(img_w)
            gap_center_css_x = detection.x_center * scale_x
            piece_init_offset_x = max(0.0, (handle_rect["x"] + handle_rect["width"] / 2.0) - canvas_rect["x"])
            distance_x = gap_center_css_x - piece_init_offset_x
            mode = f"puzzle_{detection.method}"
        elif track_rect:
            distance_x = (track_rect["x"] + track_rect["width"] - handle_rect["width"] / 2.0) - (
                handle_rect["x"] + handle_rect["width"] / 2.0
            )
            mode = f"track_{detection.method}"
        else:
            distance_x = 210.0
            mode = f"default_{detection.method}"

        drag_ms = drag_slider(
            page=page,
            start_x=start_x,
            start_y=start_y,
            distance_x=distance_x,
            distance_y=0.0,
            overshoot_px=2.0,
            hold_delay_ms=110,
        )

        page.wait_for_timeout(3500)
        try:
            html_after = page.content()
        except Exception:
            html_after = ""

        if not is_blocked(html_after) and len(html_after) >= MIN_LISTING_BYTES:
            return {
                "solved": True,
                "attempt": attempt,
                "distance_x": round(distance_x, 2),
                "drag_ms": round(drag_ms, 2),
                "mode": mode,
                "inference_ms": round(detection.inference_ms, 2),
            }

    return {"solved": False, "attempts": max_attempts}


def settle_or_solve(page: Any, detector: YOLO26CaptchaDetector, retries: int = 10) -> tuple[str, dict[str, Any] | None]:
    html = ""
    captcha_meta = None
    for _ in range(retries):
        page.wait_for_timeout(2000)
        try:
            html = page.content()
        except Exception:
            continue
        if not is_blocked(html) and len(html) >= MIN_LISTING_BYTES:
            return html, captcha_meta
        if is_slider_captcha_challenge(html, page):
            print("[!] Interactive DataDome slider captcha detected! Solving with YOLO26 + Camoufox...", file=sys.stderr)
            captcha_meta = solve_datadome_captcha(page, detector)
            try:
                html = page.content()
            except Exception:
                html = ""
            if not is_blocked(html) and len(html) >= MIN_LISTING_BYTES:
                return html, captcha_meta
    return html, captcha_meta


def dismiss_consent(page: Any) -> None:
    for sel in CONSENT_SELECTORS:
        try:
            loc = page.locator(sel)
            if loc.count():
                loc.first.click(timeout=3000)
                page.wait_for_timeout(1200)
                return
        except Exception:
            continue


def lazy_scroll(page: Any, steps: int = 6) -> None:
    for _ in range(steps):
        page.mouse.wheel(0, 1300)
        page.wait_for_timeout(350)


def build_page_url(base_url: str, page_no: int) -> str:
    if page_no <= 1:
        return base_url
    parsed = urlparse(base_url)
    qs = parse_qs(parsed.query, keep_blank_values=True)
    qs["p"] = [str(page_no)]
    new_query = urlencode(qs, doseq=True)
    return urlunparse(parsed._replace(query=new_query))


def crawl_once(
    camoufox_kwargs: dict[str, Any], target_url: str, args: Any, detector: YOLO26CaptchaDetector
) -> tuple[list[dict[str, Any]], int, bool]:
    """One crawl attempt with a single browser/proxy context: fetch listing
    page(s), extract offers, then (optionally) per-product comments. Returns
    (offers, last_html_bytes, blocked). `blocked` is True if DataDome challenged
    the listing with no offers collected, so the caller can retry a new proxy."""
    all_offers: list[dict[str, Any]] = []
    last_html_bytes = 0
    blocked = False

    with Camoufox(**camoufox_kwargs) as browser:
        page = browser.new_page()
        for page_no in range(1, max(1, args.pages) + 1):
            page_url = build_page_url(target_url, page_no)
            try:
                page.goto(page_url, wait_until="commit", timeout=60000)
            except Exception as e:
                print(f"(navigation commit warning: {str(e).splitlines()[0][:80]})", file=sys.stderr)

            html, _ = settle_or_solve(page, detector)
            last_html_bytes = len(html)

            if is_blocked(html):
                print(f"blocked by DataDome on page {page_no} ({len(html)} bytes)", file=sys.stderr)
                blocked = True
                if not all_offers:
                    return all_offers, last_html_bytes, blocked
                break

            if page_no == 1:
                dismiss_consent(page)
            lazy_scroll(page)

            offers = page.evaluate(EXTRACT_JS)
            for o in offers:
                o["page"] = page_no
            all_offers.extend(offers)

            if args.max_items > 0 and len(all_offers) >= args.max_items:
                all_offers = all_offers[: args.max_items]
                break

        # Per-product comments: reuse the DataDome-warmed context. Best-effort and
        # BOUNDED — capped to --max-comment-products and a wall-clock budget — so
        # the comment phase can never run past the Go subprocess timeout and lose
        # the already-collected listing data. Every offer gets a comments list.
        if args.with_comments and all_offers:
            for o in all_offers:
                o.setdefault("comments", [])
            targets = comment_targets(all_offers, args.max_comment_products)
            started = time.monotonic()
            for o in targets:
                if (args.comment_budget_seconds > 0
                        and time.monotonic() - started > args.comment_budget_seconds):
                    print("(comment budget exhausted; remaining offers left without comments)",
                          file=sys.stderr)
                    break
                o["comments"] = extract_comments_from_product(
                    page, o["url"], args.max_comments, detector
                )

    return all_offers, last_html_bytes, blocked


def main() -> int:
    ap = argparse.ArgumentParser(description="Fetch Allegro listing URL(s) with Camoufox + YOLO26")
    ap.add_argument("url", nargs="?", help="Allegro listing URL (e.g. https://allegro.pl/listing?string=laptop)")
    ap.add_argument("--keyword", default="", help="Search keyword if URL is not provided")
    ap.add_argument("--pages", type=int, default=1, help="Number of pages to crawl (default: 1)")
    ap.add_argument("--max-items", type=int, default=0, help="Max offers to return (0 = unlimited)")
    ap.add_argument("--profile", default="", help="Optional profile directory")
    ap.add_argument("--proxy", default="", help="Optional single proxy URL")
    ap.add_argument("--proxy-file", default="", help="Webshare-style proxy list file for random rotation")
    ap.add_argument("--proxy-api-key", default=os.getenv("WEBSHARE_API_KEY", ""),
                    help="Webshare API key to fetch the proxy pool (default: env WEBSHARE_API_KEY)")
    ap.add_argument("--proxy-retries", type=int, default=5,
                    help="Max proxies to try from the pool on block (default: 5)")
    ap.add_argument("--headed", action="store_true", help="Show browser window (default is headless=True)")
    ap.add_argument("--with-comments", action="store_true", help="Also scrape top product reviews per offer")
    ap.add_argument("--max-comments", type=int, default=5, help="Max comments per product (default: 5)")
    ap.add_argument("--max-comment-products", type=int, default=10,
                    help="Max products to scrape comments for per run (default: 10)")
    ap.add_argument("--comment-budget-seconds", type=int, default=150,
                    help="Wall-clock budget for the whole comment phase (default: 150)")
    args = ap.parse_args()

    target_url = args.url
    if not target_url and args.keyword:
        target_url = f"https://allegro.pl/listing?{urlencode({'string': args.keyword})}"
    if not target_url:
        print("error: either url or --keyword is required", file=sys.stderr)
        return EXIT_USAGE

    detector = YOLO26CaptchaDetector()
    target_os = host_os_name()

    headless_env = os.getenv("PLAYWRIGHT_HEADLESS", "true").strip().lower()
    headless = False if args.headed else (headless_env not in ("0", "false", "no"))

    base_kwargs: dict[str, Any] = {
        "headless": headless,
        "os": target_os,
        "humanize": True,
    }

    # Build the proxy pool for random rotation + retry-on-block. Source priority:
    # Webshare API key -> proxy file -> single --proxy -> direct. Each source only
    # kicks in if the previous yielded nothing.
    proxies: list[str] = []
    if args.proxy_api_key:
        proxies = fetch_webshare_proxies(args.proxy_api_key)
        if not proxies:
            print("(webshare api returned no proxies; falling back)", file=sys.stderr)
    if not proxies and args.proxy_file:
        proxies = load_proxies(args.proxy_file)
        if not proxies:
            print(f"(proxy file '{args.proxy_file}' empty/unreadable; falling back)", file=sys.stderr)
    if not proxies and args.proxy:
        proxies = [args.proxy]

    if proxies:
        random.shuffle(proxies)
        attempts: list[str | None] = proxies[: max(1, args.proxy_retries)]
    else:
        attempts = [None]

    all_offers: list[dict[str, Any]] = []
    last_html_bytes = 0
    any_blocked = False

    for i, proxy in enumerate(attempts, 1):
        kwargs = dict(base_kwargs)
        if proxy:
            kwargs["proxy"] = build_proxy(proxy)
            kwargs["geoip"] = True
        offers, last_html_bytes, blocked = crawl_once(kwargs, target_url, args, detector)
        any_blocked = any_blocked or blocked
        if offers:
            all_offers = offers
            break
        label = "proxy" if proxy else "direct"
        state = "blocked" if blocked else "empty"
        tail = "retrying" if i < len(attempts) else "giving up"
        print(f"(attempt {i}/{len(attempts)} via {label} -> {state}; {tail})", file=sys.stderr)

    if not all_offers:
        if any_blocked:
            print(f"blocked by DataDome after {len(attempts)} attempt(s)", file=sys.stderr)
            return EXIT_CHALLENGED
        print(f"no product cards found on {last_html_bytes}-byte page", file=sys.stderr)
        return EXIT_EMPTY

    json.dump(
        {"url": target_url, "bytes": last_html_bytes, "offers": all_offers},
        sys.stdout,
        ensure_ascii=False,
    )
    sys.stdout.write("\n")
    return EXIT_OK


if __name__ == "__main__":
    sys.exit(main())
