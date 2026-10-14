# Allegro listing crawler

Opens `https://allegro.pl/listing?string=<keyword>` and extracts one record
per product card:

```json
{
  "position": 2,
  "product_id": "880fe6be-3992-4532-97f2-8fd88c55889a",
  "offer_id": "17118603300",
  "title": "Laptop Lenovo V15 G4 15,6 \" AMD Ryzen 5 16 GB / 512 GB czarny",
  "url": "https://allegro.pl/produkt/laptop-lenovo-v15-g4-...-880fe6be-...",
  "price": 2399,
  "price_with_delivery": 2409.49,
  "currency": "PLN",
  "rating": 4.83,
  "review_count": 111,
  "bought_recently": 28,
  "super_seller": true,
  "smart": true,
  "delivery": "dostawa jutro – kup do 14:00",
  "sponsored": true,
  "page": 1
}
```

## Setup

```sh
python3 -m venv .venv && ./.venv/bin/pip install -r requirements.txt
cp .env.example .env
```

Needs Google Chrome installed (it drives the real one). If it isn't:
`./.venv/bin/playwright install chrome`.

## Run

```sh
./.venv/bin/python allegro_crawl.py laptop                  # 1 page
./.venv/bin/python allegro_crawl.py "fotel gamingowy" -p 3  # 3 pages
./.venv/bin/python allegro_crawl.py laptop -o laptops.json
./.venv/bin/python allegro_crawl.py --open                  # 1 visible browser, stays open
```

Output: `allegro_crawl_output.json`. Exit `0` = offers found, `1` = nothing
parsed, `2` = blocked.

Last verified run: **122 offers across 2 pages**, `blocked: false`.
Fill rates — `title`/`url`/`price` 122/122, `product_id`/`offer_id` 120/122,
`delivery` 120/122, `price_with_delivery` 94/122, `rating`/`review_count`
85/122, `bought_recently` 74/122. The sparse ones are genuinely absent on
cards for products with no reviews or no delivery estimate, not parse misses.

`sponsored` comes out at ~96%, which looks like a bug but isn't — the raw
HTML carries 58 `aria-label="Promowane …"` across 65 cards. Allegro really
does mark almost every search result as promoted.

## ENGINE: why this drives real Chrome, not CloakBrowser

Allegro is behind DataDome. Measured on this machine (Hanoi residential IP,
`14.232.214.101`), against `https://allegro.pl/`:

| Engine | Mode | Stealth | `navigator.webdriver` | Result |
|---|---|---|---|---|
| Google Chrome 149, blank profile | headed | none | **true** | **passes** |
| Google Chrome 149, blank profile | headless | none | true | blocked |
| CloakBrowser Chromium 146 | either | all knobs on | false | blocked |

Undisguised automation gets through; the stealth browser does not. So
`ENGINE=chrome` is the default.

**`PLAYWRIGHT_HEADLESS=false` is mandatory.** Same machine, same IP, same
Chrome binary — headed returns the listing, headless returns the DataDome
shell. On a server with no display, wrap the command in `xvfb-run`.

Two things were wrong with the CloakBrowser attempt, and the first was
self-inflicted:

1. **A fingerprint contradiction.** The first version of this script
   hardcoded `locale=pl-PL` + `timezone=Europe/Warsaw` while egressing from
   a Vietnamese IP. That is not a disguise, it's a mismatch. `LOCALE` and
   `TIMEZONE` are now empty by default — only set them when `PROXY` actually
   exits in Poland.
2. **A stale binary.** CloakBrowser's free tier ships Chromium **146**
   (`~/.cloakbrowser/chromium-146.../chrome`) and spoofs a Windows UA while
   running on Linux. Real Chrome here is **149**. Three majors behind is a
   tiny, conspicuous slice of real traffic, and you can't fix it by editing
   the UA string — DataDome feature-detects the engine. `cloakbrowser login`
   unlocks the free v152 binary; that's the thing to try before using
   `ENGINE=cloakbrowser` against Allegro.

`ENGINE=cloakbrowser` is still wired up and is the better tool when you need
per-account sessions, proxy rotation, or humanized input — it just loses to
plain Chrome on this particular target today.

## Two non-obvious load behaviours

Both of these looked like blocks at first and cost a debugging cycle:

- **DataDome bounces the first navigation through `?dd_referrer=`.** That
  aborts `wait_until="domcontentloaded"` and raises. `goto()` uses
  `wait_until="commit"` and swallows the error on purpose.
- **Its shell HTML is also what you see mid-redirect.** Seeing
  `captcha-delivery` once means nothing; `settle()` only concludes "blocked"
  if the shell is still there after every retry. An earlier version bailed on
  the first sighting and reported a false block.

## Notes

- `session.json`, `debug/`, `.env`, `.venv/` and the output JSON are
  gitignored.
- Fields are read off `aria-label`s (`"899,00 zł aktualna cena"`,
  `"4,54 na 5, 13 ocen produktu"`), not CSS classes — Allegro's class names
  are hashed and churn, the accessibility labels don't.
- `PAGE_DELAY_MS` (default 2.5s) throttles between pages. Don't zero it.
