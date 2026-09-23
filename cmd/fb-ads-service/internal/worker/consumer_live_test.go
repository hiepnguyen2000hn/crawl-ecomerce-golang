//go:build live

// Live tests for fb-ads-service: they crawl the Facebook Ads Library
// through the real Apify actor (igolaizola~facebook-ad-library-scraper)
// and spend Apify credit per ad returned, so they are excluded from a
// plain `go test ./...` by the `live` build tag. Run them explicitly:
//
//	go test -tags live -v -run Live ./cmd/fb-ads-service/internal/worker/
//
// Besides the shared variables in internal/livetest:
//
//	FBADS_LIVE_QUERY      search query (default "sneakers")
//	FBADS_LIVE_COUNTRY    Ads Library country code (default "US")
//	FBADS_LIVE_MAX_ITEMS  ads to request (default 10, kept low to limit cost)
//	FBADS_LIVE_ACTOR_ID   Apify actor (default: the one in etc/fbads.yaml)
package worker

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/hiepnv/crawl-ecomerce-golang/internal/livetest"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/apify"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/rabbitmq"
)

type liveFbadsParams struct {
	Query, Country, ActorID string
	MaxItems                int
}

func liveParams(t *testing.T) liveFbadsParams {
	t.Helper()
	n, err := strconv.Atoi(livetest.EnvOr("FBADS_LIVE_MAX_ITEMS", "10"))
	if err != nil || n <= 0 {
		t.Fatalf("FBADS_LIVE_MAX_ITEMS must be a positive integer")
	}
	return liveFbadsParams{
		Query:    livetest.EnvOr("FBADS_LIVE_QUERY", "sneakers"),
		Country:  livetest.EnvOr("FBADS_LIVE_COUNTRY", "US"),
		ActorID:  livetest.EnvOr("FBADS_LIVE_ACTOR_ID", "igolaizola~facebook-ad-library-scraper"),
		MaxItems: n,
	}
}

func liveApify(t *testing.T, actorID string) *apify.HTTPClient {
	t.Helper()
	token := livetest.Secret(t, "APIFY_API_TOKEN")
	// Same timeout as svc.NewServiceContext: a synchronous actor run takes
	// tens of seconds to minutes.
	return apify.NewHTTPClient(token, actorID, "https://api.apify.com/v2", &http.Client{Timeout: 5 * time.Minute})
}

// checkLiveAds asserts the fields the rest of the pipeline depends on and
// logs a summary of what the crawl actually covered.
func checkLiveAds(t *testing.T, ads []apify.Ad, maxItems int) {
	t.Helper()
	if len(ads) == 0 {
		t.Fatal("Apify returned no ads")
	}
	if len(ads) > maxItems {
		t.Errorf("got %d ads, more than max_items=%d", len(ads), maxItems)
	}

	seenIDs := map[string]bool{}
	pages := map[string]int{}
	domains := map[string]int{}
	bodies := map[string]bool{}
	withLink, withDates := 0, 0
	for i, ad := range ads {
		if ad.AdArchiveID == "" {
			t.Errorf("ad %d has empty ad_archive_id", i)
		} else if seenIDs[ad.AdArchiveID] {
			t.Errorf("duplicate ad_archive_id %s", ad.AdArchiveID)
		}
		seenIDs[ad.AdArchiveID] = true
		if ad.PageID == "" || ad.PageName == "" {
			t.Errorf("ad %s is missing page_id/page_name", ad.AdArchiveID)
		}
		if !json.Valid(ad.Raw) {
			t.Errorf("ad %s raw is not valid JSON (it is stored in a JSONB column)", ad.AdArchiveID)
		}
		if !ad.StartDate.IsZero() && !ad.EndDate.IsZero() && ad.EndDate.Before(ad.StartDate) {
			t.Errorf("ad %s ends (%s) before it starts (%s)", ad.AdArchiveID, ad.EndDate, ad.StartDate)
		}
		if !ad.StartDate.IsZero() {
			withDates++
		}
		pages[ad.PageName]++
		bodies[ad.Body] = true
		if ad.LinkURL != "" {
			withLink++
			if u, err := url.Parse(ad.LinkURL); err != nil || u.Host == "" {
				t.Errorf("ad %s link_url %q is not an absolute URL", ad.AdArchiveID, ad.LinkURL)
			} else {
				domains[u.Host]++
			}
		}
	}
	// product-extractor-service only has work to do for ads with a link_url.
	if withLink == 0 {
		t.Error("no ad has a link_url — product-extractor-service would have nothing to crawl")
	}

	t.Logf("ads=%d pages=%d distinct_bodies=%d with_link_url=%d with_start_date=%d",
		len(ads), len(pages), len(bodies), withLink, withDates)
	logCounts(t, "landing domain", domains)
	logCounts(t, "page", pages)

	// The actor's raw item carries more than the client maps (e.g. reach or
	// spend where the Ads Library exposes it); list its keys so gaps are
	// visible without opening the saved file.
	var first map[string]json.RawMessage
	if json.Unmarshal(ads[0].Raw, &first) == nil {
		keys := make([]string, 0, len(first))
		for k := range first {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		t.Logf("raw item keys: %v", keys)
	}
}

func logCounts(t *testing.T, label string, counts map[string]int) {
	t.Helper()
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return counts[keys[i]] > counts[keys[j]] })
	for _, k := range keys {
		t.Logf("  %-15s %3d  %s", label, counts[k], k)
	}
}

func rawItems(ads []apify.Ad) []json.RawMessage {
	out := make([]json.RawMessage, len(ads))
	for i, ad := range ads {
		out[i] = ad.Raw
	}
	return out
}

// TestLive_FetchAds calls the real Apify actor with the same client and
// parameters HandleMessage builds, and checks the ads that come back.
func TestLive_FetchAds(t *testing.T) {
	p := liveParams(t)
	client := liveApify(t, p.ActorID)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	result, err := client.FetchAds(ctx, apify.AdParams{Query: p.Query, Country: p.Country, MaxItems: p.MaxItems})
	if err != nil {
		t.Fatalf("FetchAds(%q, %q) error = %v", p.Query, p.Country, err)
	}
	checkLiveAds(t, result.Ads, p.MaxItems)
	livetest.SaveJSON(t, "fbads_"+p.Query+"_"+p.Country, rawItems(result.Ads))
}

// TestLive_HandleMessage runs the real worker end to end: real Apify, real
// Postgres (fbads_raw + jobs) and a real RabbitMQ publish. It needs the
// docker compose stack up and cleans up the rows it creates.
func TestLive_HandleMessage(t *testing.T) {
	p := liveParams(t)
	client := liveApify(t, p.ActorID)
	dsn, amqpURL := livetest.Stack(t)
	ctx := context.Background()
	conn := livetest.DB(t, dsn)

	params, _ := json.Marshal(map[string]any{"query": p.Query, "country": p.Country, "max_items": p.MaxItems})
	var jobID string
	if err := conn.QueryRowContext(ctx,
		`INSERT INTO jobs (type, status, params) VALUES ('fbads', 'pending', $1) RETURNING id`, params,
	).Scan(&jobID); err != nil {
		t.Fatalf("insert job: %v", err)
	}
	t.Cleanup(func() {
		conn.ExecContext(ctx, `DELETE FROM fbads_raw WHERE job_id = $1`, jobID)
		conn.ExecContext(ctx, `DELETE FROM jobs WHERE id = $1`, jobID)
	})

	publisher, err := rabbitmq.NewPublisher(rabbitmq.Config{URL: amqpURL, Exchange: livetest.Exchange})
	if err != nil {
		t.Fatalf("connect rabbitmq publisher: %v", err)
	}
	t.Cleanup(func() { publisher.Close() })
	awaitEvent := livetest.CaptureEvent(t, amqpURL, "crawl.completed.fbads")

	c := &Consumer{DB: conn, Apify: client, Publisher: publisher}
	msg, _ := json.Marshal(JobMessage{JobID: jobID, Query: p.Query, Country: p.Country, MaxItems: p.MaxItems})
	if err := c.HandleMessage(msg); err != nil {
		t.Fatalf("HandleMessage() error = %v", err)
	}

	var status string
	if err := conn.QueryRowContext(ctx, `SELECT status FROM jobs WHERE id = $1`, jobID).Scan(&status); err != nil {
		t.Fatalf("read job status: %v", err)
	}
	if status != "crawled" {
		t.Errorf("jobs.status = %q, want crawled", status)
	}

	rows, err := conn.QueryContext(ctx, `
		SELECT ad_archive_id, COALESCE(page_id, ''), COALESCE(page_name, ''), COALESCE(body, ''),
		       COALESCE(link_url, ''), start_date, end_date, raw
		FROM fbads_raw WHERE job_id = $1`, jobID)
	if err != nil {
		t.Fatalf("read fbads_raw: %v", err)
	}
	defer rows.Close()
	var ads []apify.Ad
	for rows.Next() {
		var ad apify.Ad
		var start, end *time.Time
		if err := rows.Scan(&ad.AdArchiveID, &ad.PageID, &ad.PageName, &ad.Body, &ad.LinkURL, &start, &end, &ad.Raw); err != nil {
			t.Fatalf("scan fbads_raw: %v", err)
		}
		if start != nil {
			ad.StartDate = *start
		}
		if end != nil {
			ad.EndDate = *end
		}
		ads = append(ads, ad)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate fbads_raw: %v", err)
	}
	checkLiveAds(t, ads, p.MaxItems)

	var event CompletedMessage
	if err := json.Unmarshal(awaitEvent(5*time.Second), &event); err != nil {
		t.Fatalf("decode completion event: %v", err)
	}
	if event.JobID != jobID {
		t.Errorf("completion event job_id = %q, want %q", event.JobID, jobID)
	}

	livetest.SaveJSON(t, "fbads_"+p.Query+"_"+p.Country, rawItems(ads))
	t.Logf("job %s crawled: %d ads stored in fbads_raw", jobID, len(ads))
}
