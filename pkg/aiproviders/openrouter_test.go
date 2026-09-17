package aiproviders

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestOpenRouter_CompleteJSON_FallsBackOnEmptyChoices(t *testing.T) {
	var modelsSeen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		model, _ := body["model"].(string)
		modelsSeen = append(modelsSeen, model)

		w.Header().Set("Content-Type", "application/json")
		if model == "primary-model" {
			// Simulate a transient "no choices" response (HTTP 200, empty
			// choices array) — the failure mode observed against a real
			// free-tier model on a long prompt.
			w.Write([]byte(`{"model":"primary-model","choices":[]}`))
			return
		}
		w.Write([]byte(`{"model":"fallback-model","choices":[{"message":{"content":"{\"products\":[]}"}}]}`))
	}))
	defer srv.Close()

	p := NewOpenRouter("test-key", "primary-model", srv.URL, srv.Client(), "fallback-model")
	schema := json.RawMessage(`{"type":"object","properties":{"products":{"type":"array"}},"required":["products"]}`)
	result, err := p.CompleteJSON(context.Background(), "extract products", "test_schema", schema)
	if err != nil {
		t.Fatalf("CompleteJSON() error = %v, want nil after falling back", err)
	}
	if len(modelsSeen) != 2 || modelsSeen[0] != "primary-model" || modelsSeen[1] != "fallback-model" {
		t.Errorf("modelsSeen = %v, want [primary-model fallback-model]", modelsSeen)
	}
	if string(result) != `{"products":[]}` {
		t.Errorf("result = %s, unexpected", result)
	}
}

func TestOpenRouter_CompleteJSON_ReturnsErrorAfterExhaustingRetries(t *testing.T) {
	var callCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"model":"openrouter/test-model","choices":[]}`))
	}))
	defer srv.Close()

	p := NewOpenRouter("test-key", "openrouter/test-model", srv.URL, srv.Client())
	schema := json.RawMessage(`{"type":"object","properties":{"products":{"type":"array"}},"required":["products"]}`)
	_, err := p.CompleteJSON(context.Background(), "extract products", "test_schema", schema)
	if err == nil {
		t.Fatal("expected error when the only model returns no choices, got nil")
	}
	if callCount != 1 {
		t.Errorf("callCount = %d, want 1 (single model, no fallback configured)", callCount)
	}
}

func TestOpenRouter_CompleteJSON_FallsBackToNextModelOnError(t *testing.T) {
	var modelsSeen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		model, _ := body["model"].(string)
		modelsSeen = append(modelsSeen, model)

		w.Header().Set("Content-Type", "application/json")
		if model == "primary-model" {
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error":"overloaded"}`))
			return
		}
		w.Write([]byte(`{"model":"fallback-model","choices":[{"message":{"content":"{\"products\":[]}"}}]}`))
	}))
	defer srv.Close()

	p := NewOpenRouter("test-key", "primary-model", srv.URL, srv.Client(), "fallback-model")
	schema := json.RawMessage(`{"type":"object","properties":{"products":{"type":"array"}},"required":["products"]}`)
	result, err := p.CompleteJSON(context.Background(), "extract products", "test_schema", schema)
	if err != nil {
		t.Fatalf("CompleteJSON() error = %v, want nil after falling back", err)
	}
	if string(result) != `{"products":[]}` {
		t.Errorf("result = %s, unexpected", result)
	}
	if len(modelsSeen) != 2 || modelsSeen[0] != "primary-model" || modelsSeen[1] != "fallback-model" {
		t.Errorf("modelsSeen = %v, want [primary-model fallback-model]", modelsSeen)
	}
}

func TestOpenRouter_CompleteJSON_ErrorIncludesResponseBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":{"message":"model does not support structured outputs"}}`))
	}))
	defer srv.Close()

	p := NewOpenRouter("test-key", "primary-model", srv.URL, srv.Client())
	schema := json.RawMessage(`{"type":"object","properties":{"products":{"type":"array"}},"required":["products"]}`)
	_, err := p.CompleteJSON(context.Background(), "extract products", "test_schema", schema)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "model does not support structured outputs") {
		t.Errorf("error = %v, want it to include the response body", err)
	}
}
