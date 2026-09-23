//go:build live

// Live tests for product-extractor-service: they crawl real ad landing
// pages through a running crawl4ai-service (headless Chromium) and extract
// products with the real OpenRouter model, so they are excluded from a
// plain `go test ./...` by the `live` build tag. Start crawl4ai-service
// first (docker compose up crawl4ai-service, or uvicorn main:app in
// crawl4ai-service/), then:
//
//	go test -tags live -v -run Live ./cmd/product-extractor-service/internal/worker/
//
// Each landing page is its own subtest, so the output reads as a per-site
// report of which pages this service can crawl and extract from.
//
// Besides the shared variables in internal/livetest:
//
//	CRAWL4AI_BASE_URL      crawl4ai-service address (default http://localhost:8000)
//	PRODUCT_LIVE_URLS      comma-separated URLs to test instead of liveSites;
//	                       each is treated as a product page
//	OPENROUTER_MODEL       model for extraction (default: etc/extractor.yaml's)
package worker

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/hiepnv/crawl-ecomerce-golang/internal/livetest"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/aiproviders"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/crawl4ai"
)

// liveSite is one ad landing page. product says whether a human looking at
// the page would find products with prices on it: extraction must return
// products for those and nothing for the rest.
type liveSite struct {
	name    string
	url     string
	product bool
}

// liveSites are the distinct link_urls of a real fb-ads crawl
// ("sneakers", US, 2026-09-22) — the pages this service actually receives.
var liveSites = []liveSite{
	{"shopify-saterauto", "https://saterauto.com/products/adjustable-double-layer-clear-shoe-slots-organizer", true},
	{"shopify-spaydes", "https://spaydes.co/products/amos-canvas-sneakers", true},
	{"shopify-reshoevn8r", "https://reshoevn8r.com/products/water-stain-repellent-spray-shoe-protective-coating-re8-repel", true},
	{"shopify-tabiousa", "https://tabiousa.com/products/mellow-trim-dry-mesh-quarter-socks", true},
	{"shopify-empress", "https://empressaustralia.com/products/square-tassel-loafer?variant=48117451292927", true},
	{"shopify-collection-customsneakers", "https://customsneakers.store/collections/jurassic-park", true},
	{"amazon-shortlink", "https://link.amazon/B07v0RZ35", true},
	{"drama-storyreel", "https://w2a.storyreel.life/v6/23/fb02.html?shorttv_adid=349306&language=en&utm_source=facebook", false},
	{"drama-shorttv", "https://w2a.shorttv.live/v6/2/fb02.html?shorttv_adid=303070&language=en&utm_source=facebook", false},
	{"google-play-app", "https://play.google.com/store/apps/details?id=com.ss.android.ttmd.video&hl=en_US", false},
	{"event-bigbounce", "https://thebigbounceamerica.com/event/houston/", false},
	{"facebook-event", "https://www.facebook.com/events/1803426810986172/", false},
	{"instagram-profile", "http://instagram.com/taosfootwear", false},
}

func sitesUnderTest() []liveSite {
	env := strings.TrimSpace(livetest.EnvOr("PRODUCT_LIVE_URLS", ""))
	if env == "" {
		return liveSites
	}
	var sites []liveSite
	for _, u := range strings.Split(env, ",") {
		if u = strings.TrimSpace(u); u == "" {
			continue
		}
		host := u
		if p, err := url.Parse(u); err == nil && p.Host != "" {
			host = p.Host
		}
		sites = append(sites, liveSite{name: host, url: u, product: true})
	}
	return sites
}

// liveCrawler returns a client for the running crawl4ai-service, skipping
// the test when it is not reachable.
func liveCrawler(t *testing.T) crawl4ai.Client {
	t.Helper()
	base := strings.TrimRight(livetest.EnvOr("CRAWL4AI_BASE_URL", "http://localhost:8000"), "/")
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Get(base + "/health")
	if err != nil {
		t.Skipf("crawl4ai-service not reachable at %s (%v) — start it first", base, err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Skipf("crawl4ai-service at %s /health returned %d", base, resp.StatusCode)
	}
	// Same timeout as main.go: crawl4ai pages can be slow.
	return crawl4ai.NewHTTPClient(base, &http.Client{Timeout: 2 * time.Minute})
}

func liveProvider(t *testing.T) aiproviders.Provider {
	t.Helper()
	key := livetest.Secret(t, "OPENROUTER_API_KEY")
	model := livetest.EnvOr("OPENROUTER_MODEL", "dots-studio/dots-3-note-preview:free")
	return aiproviders.NewOpenRouter(key, model, "https://openrouter.ai/api/v1", http.DefaultClient,
		"nvidia/nemotron-3-super-120b-a12b:free")
}

// blockMarkers are phrases that show up when a site served a bot wall or
// login gate instead of the real page. crawl4ai still reports success for
// those, so a "successful" crawl has to be checked for them.
var blockMarkers = regexp.MustCompile(`(?i)(just a moment|verify you are human|verifies you are not a bot|performing security verification|checking your browser|attention required|are you a robot|captcha|access denied|enable javascript and cookies|log ?in to continue|sign up to see|you must log in)`)

// TestLive_CrawlSites crawls every landing page through crawl4ai-service
// only (no AI, no quota) and reports, per site, whether usable page
// content came back.
func TestLive_CrawlSites(t *testing.T) {
	crawler := liveCrawler(t)

	for _, site := range sitesUnderTest() {
		t.Run(site.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()

			start := time.Now()
			res, err := crawler.Crawl(ctx, site.url)
			if err != nil {
				t.Fatalf("crawl request failed: %v", err)
			}
			if !res.Success {
				t.Fatalf("crawl4ai reported failure: %s", res.Error)
			}
			t.Logf("crawled in %s, markdown %d chars", time.Since(start).Round(time.Millisecond), len([]rune(res.Markdown)))
			livetest.Save(t, "page_"+site.name, "md", []byte(res.Markdown))

			// A real product page carries far more text than a bot wall.
			if n := len([]rune(strings.TrimSpace(res.Markdown))); n < 300 {
				t.Errorf("markdown is only %d chars — page likely did not render", n)
			}
			if m := blockMarkers.FindString(res.Markdown); m != "" {
				t.Errorf("page looks blocked or login-gated (found %q)", m)
			}
		})
	}
}

// TestLive_ExtractProducts runs the service's own crawl+extract step
// (extractProducts, the part of extractOne before the DB insert) against
// every landing page with the real OpenRouter model.
func TestLive_ExtractProducts(t *testing.T) {
	c := &Consumer{Crawler: liveCrawler(t), Provider: liveProvider(t)}
	iso4217 := regexp.MustCompile(`^[A-Z]{3}$`)
	report := map[string]any{}

	for _, site := range sitesUnderTest() {
		t.Run(site.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
			defer cancel()

			products, err := c.extractProducts(ctx, site.url)
			if err != nil {
				report[site.url] = map[string]any{"site": site.name, "error": err.Error()}
				t.Fatalf("extractProducts: %v", err)
			}
			report[site.url] = map[string]any{"site": site.name, "products": products}

			for i, p := range products {
				t.Logf("  %d. %s — %.2f %s (sku %s)", i+1, p.ProductName, p.Price, p.Currency, p.SKU)
				if strings.TrimSpace(p.ProductName) == "" {
					t.Errorf("product %d has an empty name", i+1)
				}
				if p.Price <= 0 {
					t.Errorf("product %q has non-positive price %v", p.ProductName, p.Price)
				}
				if !iso4217.MatchString(p.Currency) {
					t.Errorf("product %q currency %q is not an ISO 4217 code", p.ProductName, p.Currency)
				}
				if strings.TrimSpace(p.SKU) == "" {
					t.Errorf("product %q has an empty sku", p.ProductName)
				}
			}

			switch {
			case site.product && len(products) == 0:
				t.Error("product page, but no products were extracted")
			case !site.product && len(products) > 0:
				t.Errorf("not a product page, but %d products were extracted", len(products))
			default:
				t.Logf("%d products extracted", len(products))
			}
		})
	}
	livetest.SaveJSON(t, "products_extracted", report)
}

// TestLive_HandleMessage runs the real worker end to end: it seeds a job
// with one fbads_raw row per landing page, feeds HandleMessage a
// crawl.completed.fbads event, and checks the products rows written. It
// needs the docker compose stack and cleans up the rows it creates.
func TestLive_HandleMessage(t *testing.T) {
	c := &Consumer{Crawler: liveCrawler(t), Provider: liveProvider(t)}
	dsn := livetest.PostgresDSN(t)
	ctx := context.Background()
	c.DB = livetest.DB(t, dsn)

	var jobID string
	if err := c.DB.QueryRowContext(ctx,
		`INSERT INTO jobs (type, status, params) VALUES ('fbads', 'crawled', '{"query":"live-test"}') RETURNING id`,
	).Scan(&jobID); err != nil {
		t.Fatalf("insert job: %v", err)
	}
	t.Cleanup(func() {
		c.DB.ExecContext(ctx, `DELETE FROM products WHERE job_id = $1`, jobID)
		c.DB.ExecContext(ctx, `DELETE FROM fbads_raw WHERE job_id = $1`, jobID)
		c.DB.ExecContext(ctx, `DELETE FROM jobs WHERE id = $1`, jobID)
	})

	sites := sitesUnderTest()
	for i, site := range sites {
		if _, err := c.DB.ExecContext(ctx,
			`INSERT INTO fbads_raw (job_id, ad_archive_id, page_name, link_url, raw) VALUES ($1, $2, $3, $4, '{}')`,
			jobID, "live-test-"+site.name, site.name, site.url,
		); err != nil {
			t.Fatalf("insert fbads_raw %d: %v", i, err)
		}
	}

	msg, _ := json.Marshal(CompletedMessage{JobID: jobID})
	if err := c.HandleMessage(msg); err != nil {
		t.Fatalf("HandleMessage() error = %v", err)
	}

	rows, err := c.DB.QueryContext(ctx, `
		SELECT p.url, COUNT(*), BOOL_AND(p.ad_id = f.id)
		FROM products p JOIN fbads_raw f ON f.job_id = p.job_id AND f.link_url = p.url
		WHERE p.job_id = $1 GROUP BY p.url`, jobID)
	if err != nil {
		t.Fatalf("read products: %v", err)
	}
	defer rows.Close()
	perURL := map[string]int{}
	for rows.Next() {
		var u string
		var n int
		var adLinked bool
		if err := rows.Scan(&u, &n, &adLinked); err != nil {
			t.Fatalf("scan products: %v", err)
		}
		if !adLinked {
			t.Errorf("products for %s are not linked to the fbads_raw row of that URL", u)
		}
		perURL[u] = n
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate products: %v", err)
	}

	for _, site := range sites {
		n := perURL[site.url]
		t.Logf("  %-34s %2d products", site.name, n)
		if site.product && n == 0 {
			t.Errorf("%s: product page, but no products stored", site.name)
		}
		if !site.product && n > 0 {
			t.Errorf("%s: not a product page, but %d products stored", site.name, n)
		}
	}
}
