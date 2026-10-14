package apify

import (
	"context"
	"os"

	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Live end-to-end test against the real Apify API and the real Facebook Ads
// Library actor: five keywords fired at once through the same client the
// fb-ads-service worker uses, to see the slot pool do its job on real runs.
//
// It costs real Apify credit, so it is opt-in:
//
//	docker compose up -d postgres && docker compose run --rm migrate
//	APIFY_SLOTS_TEST_DSN='postgres://crawl:crawl@127.0.0.1:5433/crawl?sslmode=disable' \
//	APIFY_LIVE_TOKEN="$APIFY_API_TOKEN" \
//	go test ./pkg/apify/ -run TestLive_FbAds -v -timeout 20m
//
// Pass several comma-separated tokens in APIFY_LIVE_TOKEN to watch the runs
// spread across accounts instead of queueing on one.
var liveKeywords = []string{
	"quần dài",
	"quần đùi dài",
	"quần bò",
	"quần cộc",
	"quần bó",
}

func TestLive_FbAdsFiveKeywords(t *testing.T) {
	raw := os.Getenv("APIFY_LIVE_TOKEN")
	if raw == "" {
		t.Skip("APIFY_LIVE_TOKEN not set; skipping the live Apify test (it spends real credit)")
	}
	tokens := ParseTokens(raw)
	if len(tokens) == 0 {
		t.Fatal("APIFY_LIVE_TOKEN parsed to zero usable tokens")
	}

	db := testDB(t)
	slots := NewSlotStore(db, tokens, "live-test", 10*time.Minute)
	if err := slots.SyncTokens(context.Background(), 5); err != nil {
		t.Fatalf("SyncTokens: %v", err)
	}
	t.Logf("pool: %d account(s) x 5 = %d concurrent slots", len(tokens), len(tokens)*5)

	client := NewHTTPClient("", NewRunner("https://api.apify.com/v2", slots, 10*time.Minute))

	type outcome struct {
		keyword string
		ads     int
		elapsed time.Duration
		err     error
	}
	results := make([]outcome, len(liveKeywords))

	// peak tracks how many runs were genuinely in flight together: the whole
	// claim being tested is that these five overlap rather than queue.
	var inflight, peak int64

	started := time.Now()
	var wg sync.WaitGroup
	for i, kw := range liveKeywords {
		wg.Add(1)
		go func(i int, kw string) {
			defer wg.Done()

			n := atomic.AddInt64(&inflight, 1)
			for {
				p := atomic.LoadInt64(&peak)
				if n <= p || atomic.CompareAndSwapInt64(&peak, p, n) {
					break
				}
			}
			defer atomic.AddInt64(&inflight, -1)

			t0 := time.Now()
			res, err := client.FetchAds(context.Background(), AdParams{
				JobID:    "live-" + kw,
				Query:    kw,
				Country:  "VN",
				MaxItems: 10,
			})
			results[i] = outcome{keyword: kw, ads: len(res.Ads), elapsed: time.Since(t0), err: err}
		}(i, kw)
	}
	wg.Wait()
	total := time.Since(started)

	var ok, failed int
	var slowest time.Duration
	for _, r := range results {
		if r.err != nil {
			failed++
			t.Logf("  %-14s FAILED after %-6s %v", r.keyword, r.elapsed.Round(time.Second), r.err)
			continue
		}
		ok++
		if r.elapsed > slowest {
			slowest = r.elapsed
		}
		t.Logf("  %-14s %3d ads in %s", r.keyword, r.ads, r.elapsed.Round(time.Second))
	}
	t.Logf("wall clock %s, peak concurrent runs %d, ok %d, failed %d",
		total.Round(time.Second), atomic.LoadInt64(&peak), ok, failed)

	// Report what the pool did with the tokens, so an exhausted account is
	// visible here rather than as a mystery failure.
	rows, err := db.Query(`SELECT token_index, label, exhausted_until FROM apify_tokens ORDER BY token_index`)
	if err != nil {
		t.Fatalf("query tokens: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var idx int
		var label string
		var until *time.Time
		if err := rows.Scan(&idx, &label, &until); err != nil {
			t.Fatalf("scan: %v", err)
		}
		state := "live"
		if until != nil && until.After(time.Now()) {
			state = "PARKED until " + until.Format(time.TimeOnly)
		}
		t.Logf("token %d (%s): %s", idx, label, state)
	}

	if failed > 0 {
		t.Fatalf("%d/%d keywords failed", failed, len(liveKeywords))
	}

	// Sequential execution would make the total roughly the sum of the parts.
	// Overlapping makes it close to the slowest single run instead.
	if total > slowest*2 {
		t.Errorf("wall clock %s vs slowest run %s -- the runs queued instead of overlapping",
			total.Round(time.Second), slowest.Round(time.Second))
	}

	// No lease may survive the run, or the pool shrinks permanently.
	var leaked int
	if err := db.QueryRow(`SELECT count(*) FROM apify_leases`).Scan(&leaked); err != nil {
		t.Fatalf("count leases: %v", err)
	}
	if leaked != 0 {
		t.Errorf("%d lease(s) left behind, want 0", leaked)
	}
}
