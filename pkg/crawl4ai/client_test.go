package crawl4ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPClient_Crawl(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/crawl" {
			t.Errorf("path = %q, want /crawl", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		var body struct {
			URL string `json:"url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if body.URL != "https://example.com/product/1" {
			t.Errorf("url = %q, unexpected", body.URL)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"success":true,"markdown":"# Product\nPrice: $19.99"}`))
	}))
	defer srv.Close()

	c := NewHTTPClient(srv.URL, srv.Client())
	result, err := c.Crawl(context.Background(), "https://example.com/product/1")
	if err != nil {
		t.Fatalf("Crawl() error = %v", err)
	}
	if !result.Success {
		t.Errorf("Success = false, want true")
	}
	if result.Markdown != "# Product\nPrice: $19.99" {
		t.Errorf("Markdown = %q, unexpected", result.Markdown)
	}
}

func TestHTTPClient_Crawl_ServiceReportsFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"success":false,"error":"timeout fetching page"}`))
	}))
	defer srv.Close()

	c := NewHTTPClient(srv.URL, srv.Client())
	result, err := c.Crawl(context.Background(), "https://example.com/dead-link")
	if err != nil {
		t.Fatalf("Crawl() error = %v, want nil (service-level failure is reported via CrawlResult.Success, not a Go error)", err)
	}
	if result.Success {
		t.Errorf("Success = true, want false")
	}
	if result.Error != "timeout fetching page" {
		t.Errorf("Error = %q, unexpected", result.Error)
	}
}
