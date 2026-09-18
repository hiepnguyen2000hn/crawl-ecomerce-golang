package serpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPClient_FetchTrend(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("q") != "golang" {
			t.Errorf("query param q = %q, want %q", r.URL.Query().Get("q"), "golang")
		}
		if r.URL.Query().Get("api_key") != "test-key" {
			t.Errorf("query param api_key = %q, want %q", r.URL.Query().Get("api_key"), "test-key")
		}
		if r.URL.Query().Get("date") != "today 12-m" {
			t.Errorf("query param date = %q, want %q", r.URL.Query().Get("date"), "today 12-m")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"interest_over_time":{"timeline_data":[{"date":"Sep 1","values":[{"value":"42","extracted_value":42}]}]}}`))
	}))
	defer srv.Close()

	c := NewHTTPClient("test-key", srv.URL, srv.Client())
	result, err := c.FetchTrend(context.Background(), "golang", "", "")
	if err != nil {
		t.Fatalf("FetchTrend() error = %v", err)
	}
	if result.Keyword != "golang" {
		t.Errorf("Keyword = %q, want %q", result.Keyword, "golang")
	}
	if len(result.Points) != 1 || result.Points[0].Value != 42 || result.Points[0].Date != "Sep 1" {
		t.Errorf("Points = %+v, want one point {Date: Sep 1, Value: 42}", result.Points)
	}
}

func TestHTTPClient_FetchTrend_PassesGeo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("geo") != "VN" {
			t.Errorf("query param geo = %q, want %q", r.URL.Query().Get("geo"), "VN")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"interest_over_time":{"timeline_data":[]}}`))
	}))
	defer srv.Close()

	c := NewHTTPClient("test-key", srv.URL, srv.Client())
	if _, err := c.FetchTrend(context.Background(), "golang", "VN", ""); err != nil {
		t.Fatalf("FetchTrend() error = %v", err)
	}
}

func TestHTTPClient_FetchTrend_PassesDateRange(t *testing.T) {
	// Google Trends has no "24 months ago to today" preset, so callers
	// needing more than 12 months (e.g. a year-over-year comparison) pass a
	// custom "YYYY-MM-DD YYYY-MM-DD" range instead of a preset like
	// "today 12-m" — the client just forwards whatever it's given.
	const customRange = "2024-09-18 2026-09-18"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("date") != customRange {
			t.Errorf("query param date = %q, want %q", r.URL.Query().Get("date"), customRange)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"interest_over_time":{"timeline_data":[]}}`))
	}))
	defer srv.Close()

	c := NewHTTPClient("test-key", srv.URL, srv.Client())
	if _, err := c.FetchTrend(context.Background(), "golang", "", customRange); err != nil {
		t.Fatalf("FetchTrend() error = %v", err)
	}
}
