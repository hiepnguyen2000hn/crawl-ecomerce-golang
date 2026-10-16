package apify

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// newTestClient wires a client against a stub server with a single-slot
// token pool, so these tests exercise the same Acquire/release path as
// production rather than a bypass that only they use.
func newTestClient(t *testing.T, baseURL string) *HTTPClient {
	t.Helper()
	db := testDB(t)
	s := newStore(t, db, []string{"test-token"}, 5)
	return NewHTTPClient("igolaizola~facebook-ad-library-scraper", NewRunner(baseURL, s, time.Minute))
}

func TestHTTPClient_FetchAds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("token") != "test-token" {
			t.Errorf("query param token = %q, want %q", r.URL.Query().Get("token"), "test-token")
		}
		if r.URL.Path != "/acts/igolaizola~facebook-ad-library-scraper/run-sync-get-dataset-items" {
			t.Errorf("path = %q, unexpected", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[
			{
				"ad_archive_id": "123",
				"page_id": "456",
				"page_name": "Acme",
				"is_active": true,
				"start_date": 1700000000,
				"end_date": 1701000000,
				"snapshot": {
					"body": {"text": "Buy now"},
					"title": "Acme Sale",
					"cta_text": "Shop Now",
					"cta_type": "SHOP_NOW",
					"link_url": "https://acme.example/sale"
				}
			}
		]`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	result, err := c.FetchAds(context.Background(), AdParams{Query: "acme", Country: "US", MaxItems: 5})
	if err != nil {
		t.Fatalf("FetchAds() error = %v", err)
	}
	if len(result.Ads) != 1 {
		t.Fatalf("len(Ads) = %d, want 1", len(result.Ads))
	}
	ad := result.Ads[0]
	if ad.AdArchiveID != "123" || ad.PageName != "Acme" || !ad.IsActive {
		t.Errorf("ad top-level fields wrong: %+v", ad)
	}
	if ad.Body != "Buy now" || ad.Title != "Acme Sale" || ad.CtaText != "Shop Now" || ad.CtaType != "SHOP_NOW" || ad.LinkURL != "https://acme.example/sale" {
		t.Errorf("ad snapshot fields wrong: %+v", ad)
	}
	wantStart := time.Unix(1700000000, 0).UTC()
	if !ad.StartDate.Equal(wantStart) {
		t.Errorf("StartDate = %v, want %v", ad.StartDate, wantStart)
	}
}

func TestHTTPClient_FetchAds_Status201(t *testing.T) {
	// The real Apify run-sync-get-dataset-items endpoint can answer with
	// 201 Created (not just 200 OK) on a successful run; a strict ==200
	// check causes a false failure on a request that actually succeeded.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`[{"ad_archive_id":"1","page_id":"2","page_name":"Acme","is_active":true,"start_date":1700000000,"end_date":1701000000,"snapshot":{"body":{"text":"hi"},"title":"t","cta_text":"c","cta_type":"ct","link_url":"u"}}]`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	result, err := c.FetchAds(context.Background(), AdParams{Query: "acme", Country: "US", MaxItems: 5})
	if err != nil {
		t.Fatalf("FetchAds() error = %v, want nil for 201 Created", err)
	}
	if len(result.Ads) != 1 {
		t.Fatalf("len(Ads) = %d, want 1", len(result.Ads))
	}
}

func TestHTTPClient_FetchAds_SkipsExhaustedTokenOnTheNextJob(t *testing.T) {
	// The old transport retried the *same* request on the next token. That is
	// wrong under slot accounting: the retry would run on an account whose
	// concurrency budget was never charged. Instead a credential failure
	// parks the token, and later jobs' Acquire simply do not pick it.
	//
	// Which account a given job lands on is deliberately not deterministic --
	// the allocator breaks ties at random so 10 jobs do not all pile onto the
	// lowest-id accounts -- so this asserts the invariant rather than an order.
	var mu sync.Mutex
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.URL.Query().Get("token")
		mu.Lock()
		seen = append(seen, token)
		mu.Unlock()
		if token == "spent" {
			w.WriteHeader(http.StatusPaymentRequired)
			w.Write([]byte(`{"error":{"type":"monthly-usage-hard-limit-exceeded"}}`))
			return
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"query":"acme"`) {
			t.Errorf("request body = %q, want the actor input preserved", body)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[{"ad_archive_id":"1","page_id":"2","page_name":"Acme","is_active":true,"start_date":1700000000,"end_date":1701000000,"snapshot":{"body":{"text":"hi"},"title":"t","cta_text":"c","cta_type":"ct","link_url":"u"}}]`))
	}))
	defer srv.Close()

	db := testDB(t)
	slots := newStore(t, db, []string{"spent", "funded"}, 5)
	c := NewHTTPClient("igolaizola~facebook-ad-library-scraper", NewRunner(srv.URL, slots, time.Minute))

	var spentHits, successes int
	for i := 0; i < 6; i++ {
		result, err := c.FetchAds(context.Background(), AdParams{Query: "acme", MaxItems: 5})
		if err != nil {
			spentHits++
			continue
		}
		successes++
		if len(result.Ads) != 1 {
			t.Fatalf("job %d: len(Ads) = %d, want 1", i, len(result.Ads))
		}
	}

	// The spent account may be picked once. After that it is parked, so it
	// must never be reached again no matter how many jobs run.
	if spentHits > 1 {
		t.Errorf("the spent account was used %d times, want at most 1 before it is parked", spentHits)
	}
	if successes == 0 {
		t.Error("no job reached the funded account")
	}

	mu.Lock()
	defer mu.Unlock()
	for i, tok := range seen {
		if tok == "spent" && i != 0 && seen[i-1] == "spent" {
			t.Errorf("token sequence %v retried a parked account", seen)
		}
	}
}

func TestHTTPClient_FetchAds_NullDates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[
			{
				"ad_archive_id": "789",
				"page_id": "456",
				"page_name": "Acme",
				"is_active": false,
				"start_date": null,
				"end_date": null,
				"snapshot": {
					"body": {"text": "No dates"},
					"title": "Acme Sale",
					"cta_text": "Shop Now",
					"cta_type": "SHOP_NOW",
					"link_url": "https://acme.example/sale"
				}
			}
		]`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	result, err := c.FetchAds(context.Background(), AdParams{Query: "acme", Country: "US", MaxItems: 5})
	if err != nil {
		t.Fatalf("FetchAds() error = %v, want nil (real Apify responses can have null start_date/end_date)", err)
	}
	if len(result.Ads) != 1 {
		t.Fatalf("len(Ads) = %d, want 1", len(result.Ads))
	}
	ad := result.Ads[0]
	if !ad.StartDate.IsZero() {
		t.Errorf("StartDate = %v, want zero value for null input", ad.StartDate)
	}
	if !ad.EndDate.IsZero() {
		t.Errorf("EndDate = %v, want zero value for null input", ad.EndDate)
	}
}
