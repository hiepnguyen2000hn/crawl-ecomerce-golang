package aiproviders

import (
	"context"
	"encoding/json"
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

func TestOpenRouter_CompleteJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		rf, ok := body["response_format"].(map[string]interface{})
		if !ok {
			t.Fatalf("request body missing response_format, got: %+v", body)
		}
		if rf["type"] != "json_schema" {
			t.Errorf("response_format.type = %v, want json_schema", rf["type"])
		}
		js, ok := rf["json_schema"].(map[string]interface{})
		if !ok {
			t.Fatalf("response_format.json_schema missing, got: %+v", rf)
		}
		if js["name"] != "test_schema" {
			t.Errorf("json_schema.name = %v, want test_schema", js["name"])
		}
		if js["strict"] != true {
			t.Errorf("json_schema.strict = %v, want true", js["strict"])
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"model":"openrouter/test-model","choices":[{"message":{"content":"{\"products\":[{\"product_name\":\"Widget\",\"price\":9.99,\"currency\":\"USD\",\"sku\":\"W-1\"}]}"}}]}`))
	}))
	defer srv.Close()

	p := NewOpenRouter("test-key", "openrouter/test-model", srv.URL, srv.Client())
	schema := json.RawMessage(`{"type":"object","properties":{"products":{"type":"array"}},"required":["products"]}`)
	result, err := p.CompleteJSON(context.Background(), "extract products", "test_schema", schema)
	if err != nil {
		t.Fatalf("CompleteJSON() error = %v", err)
	}

	var parsed struct {
		Products []struct {
			ProductName string  `json:"product_name"`
			Price       float64 `json:"price"`
			Currency    string  `json:"currency"`
			SKU         string  `json:"sku"`
		} `json:"products"`
	}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("unmarshal CompleteJSON() result: %v", err)
	}
	if len(parsed.Products) != 1 || parsed.Products[0].ProductName != "Widget" || parsed.Products[0].SKU != "W-1" {
		t.Errorf("parsed products = %+v, unexpected", parsed.Products)
	}
}
