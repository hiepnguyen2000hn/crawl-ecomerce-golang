package kalodata

import (
	"context"
	"os"
	"testing"
	"time"
)

// Live test of the browser keyword-search path: Go BrowserSearcher -> search.py
// -> Camoufox login -> /product/searchList -> parse. Opt-in:
//
//	KALODATA_SEARCH_EMAIL=... KALODATA_SEARCH_PASSWORD=... KALODATA_SEARCH_PROXY=... \
//	go test ./pkg/kalodata/ -run TestLive_KalodataSearch -v -timeout 6m
func TestLive_KalodataSearch(t *testing.T) {
	email := os.Getenv("KALODATA_SEARCH_EMAIL")
	password := os.Getenv("KALODATA_SEARCH_PASSWORD")
	if email == "" || password == "" {
		t.Skip("KALODATA_SEARCH_EMAIL/PASSWORD not set; skipping live keyword-search test")
	}
	python := os.Getenv("KALODATA_PYTHON")
	if python == "" {
		python = "/home/hiepnv/code/crawl-ecomerce-golang/scripts/camoufox-yolo-captcha/.venv/bin/python"
	}
	script := os.Getenv("KALODATA_SEARCH_SCRIPT")
	if script == "" {
		script = "/home/hiepnv/code/crawl-ecomerce-golang/cmd/kalodata-service/crawler/search.py"
	}
	acc := Account{Email: email, Password: password, Proxy: os.Getenv("KALODATA_SEARCH_PROXY")}
	s := NewBrowserSearcher(python, script, "", "USD", []Account{acc})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	products, err := s.Search(ctx, ProductParams{Country: "VN", PageNo: 1, PageSize: 5, StartDate: "2026-10-13", EndDate: "2026-11-11"}, "serum")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	t.Logf("keyword 'serum' -> %d products", len(products))
	for i, p := range products {
		t.Logf("  [%d] id=%q title=%q revenue=%q sales=%d rating=%.1f rawBytes=%d",
			i+1, p.ProductID, p.Title, p.Revenue, p.Sales, p.Rating, len(p.Raw))
	}
	if len(products) == 0 {
		t.Fatal("no products from keyword search")
	}
}
