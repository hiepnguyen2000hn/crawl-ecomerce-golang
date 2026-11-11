package kalodata

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"
)

// Live end-to-end test against the real Kalodata API. It needs a Postgres test
// DB (for the cookie registry) and at least one real Kalodata Cookie header.
// Opt-in:
//
//	docker compose up -d postgres && docker compose run --rm migrate
//	KALODATA_TEST_DSN='postgres://crawl:crawl@127.0.0.1:5433/crawl?sslmode=disable' \
//	KALODATA_COOKIES="$(cat one_account_cookie_header.txt)" \
//	go test ./pkg/kalodata/ -run TestLive_Kalodata -v -timeout 5m
//
// Confirms the product/video endpoints respond AND that the assumed field paths
// parse (any empty normalized field with non-empty raw means mapProducts/
// mapVideos needs fixing).
func TestLive_Kalodata(t *testing.T) {
	cookies := ParseCookies(os.Getenv("KALODATA_COOKIES"))
	if len(cookies) == 0 {
		t.Skip("KALODATA_COOKIES not set; skipping the live Kalodata test")
	}
	db := testDB(t) // needs KALODATA_TEST_DSN, else skips
	reg := NewCookieRegistry(db, 168*time.Hour, 10, 10)
	if err := reg.SyncCookies(context.Background(), cookies); err != nil {
		t.Fatalf("SyncCookies: %v", err)
	}
	client := NewHTTPClient("", "", reg, 3)

	country := os.Getenv("KALODATA_COUNTRY")
	if country == "" {
		country = "VN"
	}
	day, err := client.AllLastDay(context.Background(), country)
	if err != nil {
		t.Fatalf("AllLastDay: %v", err)
	}
	t.Logf("allLastDay(%s) = %q", country, day)

	products, err := client.SearchProducts(context.Background(), ProductParams{
		Country: country, StartDate: day, EndDate: day,
		PageNo: 1, PageSize: 5, SortField: "revenue", SortType: "DESC",
	})
	if err != nil {
		t.Fatalf("SearchProducts: %v", err)
	}
	t.Logf("products: %d", len(products))
	for i, p := range products {
		t.Logf("  [%d] id=%q title=%q revenue=%.1f sales=%d rating=%.2f rawBytes=%d",
			i+1, p.ProductID, p.Title, p.Revenue, p.Sales, p.Rating, len(p.Raw))
	}
	if len(products) > 0 {
		var top map[string]json.RawMessage
		_ = json.Unmarshal(products[0].Raw, &top)
		keys := make([]string, 0, len(top))
		for k := range top {
			keys = append(keys, k)
		}
		t.Logf("product[0] fields: %v", keys)
	}

	videos, err := client.SearchVideos(context.Background(), VideoParams{
		Country: country, StartDate: day, EndDate: day,
		PageNo: 1, PageSize: 5, SortField: "revenue", SortType: "DESC",
	})
	if err != nil {
		t.Fatalf("SearchVideos: %v", err)
	}
	t.Logf("videos: %d", len(videos))
	for i, v := range videos {
		t.Logf("  [%d] id=%q handle=%q title=%q views=%d rawBytes=%d", i+1, v.VideoID, v.Handle, v.Title, v.Views, len(v.Raw))
	}
}
