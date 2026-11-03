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
import sys
from pathlib import Path
from typing import Any
from urllib.parse import parse_qs, urlencode, urlparse, urlunparse

# Import YOLO26 + Camoufox solver from scripts/camoufox-yolo-captcha
REPO_ROOT = Path(__file__).resolve().parents[3]
SOLVER_DIR = REPO_ROOT / "scripts" / "camoufox-yolo-captcha"
if str(SOLVER_DIR) not in sys.path:
    sys.path.insert(0, str(SOLVER_DIR))

import cv2  # noqa: E402
import numpy as np  # noqa: E402
from camoufox.sync_api import Camoufox  # noqa: E402
from captcha_solver import YOLO26CaptchaDetector, drag_slider  # noqa: E402

BLOCK_MARKERS = ("captcha-delivery.com", "DataDome Device Check", "geo.captcha-delivery")
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


def host_os_name() -> str:
    sys_name = platform.system().lower()
    if "linux" in sys_name:
        return "linux"
    if "darwin" in sys_name:
        return "macos"
    return "windows"


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


def main() -> int:
    ap = argparse.ArgumentParser(description="Fetch Allegro listing URL(s) with Camoufox + YOLO26")
    ap.add_argument("url", nargs="?", help="Allegro listing URL (e.g. https://allegro.pl/listing?string=laptop)")
    ap.add_argument("--keyword", default="", help="Search keyword if URL is not provided")
    ap.add_argument("--pages", type=int, default=1, help="Number of pages to crawl (default: 1)")
    ap.add_argument("--max-items", type=int, default=0, help="Max offers to return (0 = unlimited)")
    ap.add_argument("--profile", default="", help="Optional profile directory")
    ap.add_argument("--proxy", default="", help="Optional proxy URL")
    ap.add_argument("--headed", action="store_true", help="Show browser window (default is headless=True)")
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

    camoufox_kwargs: dict[str, Any] = {
        "headless": headless,
        "os": target_os,
        "humanize": True,
    }
    if args.proxy:
        camoufox_kwargs["proxy"] = {"server": args.proxy}
        camoufox_kwargs["geoip"] = True

    all_offers: list[dict[str, Any]] = []
    last_html_bytes = 0

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
                if not all_offers:
                    return EXIT_CHALLENGED
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

    if not all_offers:
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
