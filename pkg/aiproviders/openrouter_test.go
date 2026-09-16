package aiproviders

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenRouter_Complete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("missing/incorrect Authorization header: %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"model":"openrouter/test-model","choices":[{"message":{"content":"summary text"}}]}`))
	}))
	defer srv.Close()

	p := NewOpenRouter("test-key", "openrouter/test-model", srv.URL, srv.Client())
	result, err := p.Complete(context.Background(), "summarize this trend")
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if result.Content != "summary text" {
		t.Errorf("Content = %q, want %q", result.Content, "summary text")
	}
	if result.Model != "openrouter/test-model" {
		t.Errorf("Model = %q, want %q", result.Model, "openrouter/test-model")
	}
}
