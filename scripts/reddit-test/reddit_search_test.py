"""
One-off test: crawl Reddit's search HTML page (anonymous, no login) with
CloakBrowser headless and dump extracted post data to a JSON file.

This mirrors the curl request analyzed earlier (www.reddit.com/search/?q=...)
but drives a real headless Chromium so client-side rendered content
(post cards are streamed in via JS chunks, not present in the raw HTML)
is fully loaded before scraping.

Usage:
    python reddit_search_test.py "váy đẹp"

Output:
    ./reddit_search_test_output.json
"""
import json
import sys
import urllib.parse

from cloakbrowser import launch

DEFAULT_QUERY = "váy đẹp"
OUTPUT_FILE = "reddit_search_test_output.json"


def main() -> int:
    query = sys.argv[1] if len(sys.argv) > 1 else DEFAULT_QUERY
    url = f"https://www.reddit.com/search/?q={urllib.parse.quote(query)}"

    browser = launch(headless=True)
    try:
        page = browser.new_page(locale="vi-VN")
        print(f"Navigating to {url} ...", file=sys.stderr)
        page.goto(url, wait_until="domcontentloaded", timeout=30000)

        try:
            page.wait_for_selector('a[data-testid="post-title"]', timeout=15000)
        except Exception as e:
            print(f"[!] No post cards appeared: {e}", file=sys.stderr)

        # Let the streamed JS chunks finish hydrating post cards.
        page.wait_for_timeout(2000)

        blocked = "js_challenge=1" in page.url

        posts = page.evaluate(
            """
() => {
    const cards = document.querySelectorAll('a[data-testid="post-title"]');
    return Array.from(cards).map(a => {
        const unit = a.closest('[data-testid="search-post-unit"]') || a.closest('[data-testid="search-sdui-post"]');
        const communityEl = unit ? unit.querySelector('[data-testid="search-community-title"], [data-testid="search-author"]') : null;
        return {
            title: a.getAttribute('aria-label') || a.textContent.trim(),
            permalink: a.getAttribute('href'),
            community_text: communityEl ? communityEl.textContent.trim() : null,
        };
    });
}
"""
        )

        for p in posts:
            if p.get("permalink"):
                p["url"] = "https://www.reddit.com" + p["permalink"]

        result = {
            "query": query,
            "source_url": url,
            "final_url": page.url,
            "blocked_by_js_challenge": blocked,
            "post_count": len(posts),
            "posts": posts,
        }

        with open(OUTPUT_FILE, "w", encoding="utf-8") as f:
            json.dump(result, f, ensure_ascii=False, indent=2)

        if blocked:
            print(f"[!] BLOCKED: Reddit served a js_challenge gate, 0 posts extracted -> {OUTPUT_FILE}", file=sys.stderr)
        else:
            print(f"[+] Extracted {len(posts)} posts -> {OUTPUT_FILE}", file=sys.stderr)
    finally:
        browser.close()
    return 0


if __name__ == "__main__":
    sys.exit(main())
