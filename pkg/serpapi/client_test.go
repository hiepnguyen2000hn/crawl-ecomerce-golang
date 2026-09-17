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
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"interest_over_time":{"timeline_data":[{"date":"Sep 1","values":[{"value":"42","extracted_value":42}]}]}}`))
	}))
	defer srv.Close()

	c := NewHTTPClient("test-key", srv.URL, srv.Client())
	result, err := c.FetchTrend(context.Background(), "golang", "")
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
	if _, err := c.FetchTrend(context.Background(), "golang", "VN"); err != nil {
		t.Fatalf("FetchTrend() error = %v", err)
	}
}
