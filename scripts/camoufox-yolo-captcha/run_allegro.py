#!/usr/bin/env python3
"""
Full Allegro crawl using Camoufox (os="linux", humanize=True) + YOLO26 (ONNX) captcha solver.

Why `os="linux"` is critical on this Linux machine:
- Without `os="linux"`, Camoufox defaults to random OS choice (`["windows", "macos", "linux"]`).
- When it spoofs Windows/macOS (`Win32` / Direct3D11) while TCP/IP + X11 display metrics come from Linux,
  DataDome's interstitial check (`geo.captcha-delivery.com/interstitial/`) detects the OS/kernel mismatch
  and immediately bans the session ("You have been blocked").
- Also, DataDome runs a silent 4-6s JS device check (`rt: 'i'` / `/interstitial/`) before redirecting
  to the real 3.8MB Allegro page. Only if DataDome serves `rt: 'c'` (`/captcha/`) should the YOLO26 +
  Camoufox slider solver interact with the page.

Usage:
    ./.venv/bin/python run_allegro.py "laptop" -p 1
"""
from __future__ import annotations

import argparse
import json
import os
import platform
import re
import sys
from datetime import datetime
from pathlib import Path
from typing import Any
from urllib.parse import urlencode

import cv2
import numpy as np
from camoufox.sync_api import Camoufox

from captcha_solver import YOLO26CaptchaDetector, drag_slider

HERE = Path(__file__).parent
DEBUG_DIR = HERE / "debug"
LISTING_URL = "https://allegro.pl/listing"
BLOCK_MARKERS = ("captcha-delivery.com", "DataDome Device Check", "geo.captcha-delivery")
MIN_LISTING_BYTES = 200_000

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
    'Vivo', 'Motorola', 'Nokia', 'Garmin', 'Bose', 'JBL', 'Philips',
    'Bosch', 'Makita', 'DeWalt', 'Dyson', 'Karcher', 'Logitech', 'SteelSeries'
  ];

  const parseBrandAndModel = (title, specs) => {
    let brand = specs['Marka'] || specs['Producent'] || null;
    let model = specs['Model'] || specs['Kod producenta'] || specs['Seria'] || null;

    if (title) {
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
    """
    Distinguish DataDome's silent interstitial check (`geo.captcha-delivery.com/interstitial/`, `rt:'i'`)
    from an actual interactive slider captcha (`geo.captcha-delivery.com/captcha/`, `rt:'c'`).
    Wait out interstitials passively; only solve when `/captcha/` is present and not hard-blocked (`t=bv`).
    """
    if "t=bv" in html or "'t':'bv'" in html:
        return False
    if "geo.captcha-delivery.com/captcha/" in html or "'rt':'c'" in html:
        return True
    for frame in page.frames:
        if "captcha-delivery.com/captcha/" in frame.url and "t=bv" not in frame.url:
            return True
    return False


def solve_datadome_captcha(page: Any, detector: YOLO26CaptchaDetector, max_attempts: int = 3) -> dict[str, Any] | None:
    """
    Detect and solve DataDome slider inside iframe (`geo.captcha-delivery.com/captcha/`) or main page:
    1. If a puzzle canvas/background image is present, run YOLO26 ONNX inference on it to compute `distance_x`.
    2. If it is the horizontal slide bar ("Slide right to secure your access"), compute exact track distance
       and drag from left to right with Camoufox `humanize=True`.
    """
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
            print(f"    [!] Frame DOM inspect error (attempt {attempt}): {e}", file=sys.stderr)
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
            print(f"    [!] No slider handle found on attempt {attempt}", file=sys.stderr)
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

        print(
            f"    [*] Solving slider captcha (attempt {attempt}): start=({start_x:.1f}, {start_y:.1f}), "
            f"distance_x={distance_x:.1f}px, mode={mode}",
            file=sys.stderr,
        )

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
    """
    Wait for DataDome's silent interstitial (`/interstitial/`, ~4-6s) to redirect to the real listing.
    Only trigger the active YOLO26 + Camoufox slider solver if DataDome serves an interactive `/captcha/`.
    """
    html = ""
    captcha_meta = None
    for i in range(retries):
        page.wait_for_timeout(2000)
        try:
            html = page.content()
        except Exception:
            continue
        if not is_blocked(html) and len(html) >= MIN_LISTING_BYTES:
            return html, captcha_meta
        if is_slider_captcha_challenge(html, page):
            print("    [!] Interactive DataDome slider captcha detected! Solving with YOLO26 + Camoufox...", file=sys.stderr)
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


def listing_url(query: str, page_no: int) -> str:
    params: dict[str, Any] = {"string": query}
    if page_no > 1:
        params["p"] = page_no
    return f"{LISTING_URL}?{urlencode(params)}"


def crawl_allegro(query: str, pages: int, headless: bool = True) -> dict[str, Any]:
    detector = YOLO26CaptchaDetector()
    target_os = host_os_name()
    result: dict[str, Any] = {
        "engine": f"camoufox(os={target_os},headless={headless})+yolo26",
        "query": query,
        "pages_requested": pages,
        "pages_fetched": 0,
        "blocked": False,
        "captcha_meta": None,
        "offers": [],
    }

    with Camoufox(headless=headless, os=target_os, humanize=True) as browser:
        page = browser.new_page()
        for page_no in range(1, pages + 1):
            url = listing_url(query, page_no)
            print(f"[+] Opening page {page_no}/{pages} (os={target_os}, headless={headless}): {url}", file=sys.stderr)
            try:
                page.goto(url, wait_until="commit", timeout=60000)
            except Exception as e:
                print(f"    (navigation commit warning: {str(e).splitlines()[0][:80]})", file=sys.stderr)

            html, c_meta = settle_or_solve(page, detector)
            if c_meta:
                result["captcha_meta"] = c_meta

            if is_blocked(html):
                result["blocked"] = True
                DEBUG_DIR.mkdir(exist_ok=True)
                page.screenshot(path=str(DEBUG_DIR / f"blocked-p{page_no}.png"))
                (DEBUG_DIR / f"blocked-p{page_no}.html").write_text(html, encoding="utf-8")
                print(f"[!] Blocked on page {page_no}. Saved debug/blocked-p{page_no}.png", file=sys.stderr)
                break

            if page_no == 1:
                dismiss_consent(page)
            lazy_scroll(page)

            offers = page.evaluate(EXTRACT_JS)
            for o in offers:
                o["page"] = page_no
            result["offers"].extend(offers)
            result["pages_fetched"] = page_no
            print(f"    -> Extracted {len(offers)} offers from page {page_no}", file=sys.stderr)

    return result


def main() -> int:
    ap = argparse.ArgumentParser(description="Allegro crawler with Camoufox + YOLO26 captcha solver")
    ap.add_argument("query", nargs="?", default="laptop", help="Search keyword (default: laptop)")
    ap.add_argument("-p", "--pages", type=int, default=1, help="Number of pages to crawl (default: 1)")
    ap.add_argument("-o", "--out", default=None, help="Output JSON file path")
    ap.add_argument("--headed", action="store_true", help="Show browser UI window (default: run headless in background)")
    args = ap.parse_args()

    headless_env = os.getenv("PLAYWRIGHT_HEADLESS", "true").strip().lower()
    headless = False if args.headed else (headless_env not in ("0", "false", "no"))

    res = crawl_allegro(args.query, args.pages, headless=headless)
    slug = re.sub(r"[^a-z0-9]+", "-", args.query.lower()).strip("-") or "query"
    stamp = datetime.now().strftime("%Y%m%d-%H%M%S")
    out_path = Path(args.out) if args.out else (HERE / f"allegro-{slug}-{stamp}.json")
    out_path.write_text(json.dumps(res, ensure_ascii=False, indent=2), encoding="utf-8")

    n = len(res["offers"])
    print(f"\n[+] Saved {n} offers from {res['pages_fetched']} page(s) -> {out_path}", file=sys.stderr)
    for o in res["offers"][:10]:
        print(f"    {o['price']} PLN | {(o['title'] or '')[:65]}", file=sys.stderr)
    return 0 if n > 0 else 1


if __name__ == "__main__":
    sys.exit(main())
