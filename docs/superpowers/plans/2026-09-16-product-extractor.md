# Product Extractor Pipeline Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a product-extraction enrichment pipeline that runs after fb-ads-service crawls: for each distinct `link_url` in a job's `fbads_raw` rows, crawl the destination page with `crawl4ai` (Python, HTML→Markdown), then use OpenRouter's structured output (`response_format: json_schema`) to extract product name/price/currency/SKU into a new `products` table.

**Architecture:** A new Python service (`crawl4ai-service`) exposes a single `POST /crawl` HTTP endpoint wrapping `crawl4ai`'s `AsyncWebCrawler`. A new Go service (`product-extractor-service`), structurally like `ai-service`, binds to the EXISTING `crawl.completed.fbads` routing key (no changes to `fb-ads-service` or `ai-service` — this is purely an additional consumer on an event that already fires), calls `crawl4ai-service` per URL, then calls OpenRouter via an extended `pkg/aiproviders.Provider` interface for structured JSON extraction.

**Tech Stack:** Go 1.25 (existing services), Python 3.12 + FastAPI + `crawl4ai` (new), PostgreSQL via `golang-migrate`, Docker Compose. Reuses `pkg/db`, `pkg/rabbitmq` as-is.

**Spec:** `docs/superpowers/specs/2026-09-16-product-extractor-design.md`

## Global Constraints

- Module path: `github.com/hiepnv/crawl-ecomerce-golang` (existing repo/branch, continue in place).
- Go version: this repo's `go.mod` specifies `go 1.25.0` — Dockerfiles for the new Go service use `golang:1.25-alpine` (established pattern from prior plans).
- RabbitMQ exchange: `crawl` (topic, already declared). This plan adds NO new routing keys — `product-extractor-service` binds to the existing `crawl.completed.fbads` key that `fb-ads-service` already publishes to (verify: `cmd/fb-ads-service/internal/worker/consumer.go` already does `c.Publisher.Publish(ctx, "crawl.completed.fbads", payload)` — do not modify that file in this plan).
- `product-extractor-service` must NEVER modify `jobs.status` — that state machine belongs exclusively to `ai-service`. A crawl4ai/extraction failure for one URL must be logged and skipped, never fail the whole message (so the RabbitMQ delivery should still ack, not nack — this differs from every prior consumer in this repo, which nacks on any handler error).
- Every service that talks to an external API/service must go through a small interface so tests can substitute an `httptest.Server` — never call the real crawl4ai-service or OpenRouter in a unit test.
- Config values come from go-zero `.yaml` config files loaded with `conf.MustLoad(*configFile, &c, conf.UseEnv())` (not bare `conf.MustLoad` — this was a Critical bug in an earlier plan; do not repeat it). `product-extractor-service` has no external API key of its own to guard at startup (it reuses `OPENROUTER_API_KEY`, already guarded by `ai-service`'s own startup check — but add the same guard here too since this is a separate process that also depends on that key being set).
- Two config variants per Go service: host-oriented `.yaml` (`localhost:5433`/`localhost:5673`) and `.docker.yaml` (`postgres:5432`/`rabbitmq:5672`), matching the established split — never mix.
- **`crawl4ai`'s exact Python API surface (e.g. the shape of the object `AsyncWebCrawler().arun()` returns) is not independently verified in this plan** — the implementer of Task 4 MUST run a real crawl against a live URL during that task's verification step and adapt the code to whatever the installed `crawl4ai` version's API actually returns, the same way earlier plans adapted to `goctl`'s actual generated output. Note any such adaptation in that task's report.

---

## File Structure

```
db/migrations/
  000003_products.up.sql
  000003_products.down.sql
pkg/crawl4ai/
  client.go
  client_test.go
pkg/aiproviders/
  provider.go (modify: add CompleteJSON to the Provider interface)
  openrouter.go (modify: implement CompleteJSON)
  openrouter_test.go (modify: add a CompleteJSON test)
crawl4ai-service/
  main.py
  requirements.txt
  Dockerfile
cmd/product-extractor-service/
  etc/extractor.yaml
  etc/extractor.docker.yaml
  Dockerfile
  main.go
  internal/worker/
    consumer.go
    consumer_test.go
docker-compose.yml (modify)
.env.example (unchanged — reuses existing OPENROUTER_API_KEY)
README.md (modify)
```

---

## Task 1: Database migration — `products` table

**Files:**
- Create: `db/migrations/000003_products.up.sql`
- Create: `db/migrations/000003_products.down.sql`

**Interfaces:**
- Produces: table `products` that Task 5 (`product-extractor-service` worker) writes to.

- [ ] **Step 1: Write the up migration**

```sql
-- db/migrations/000003_products.up.sql
CREATE TABLE products (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id UUID NOT NULL REFERENCES jobs(id),
    ad_id UUID REFERENCES fbads_raw(id),
    url TEXT NOT NULL,
    product_name TEXT,
    price NUMERIC,
    currency TEXT,
    sku TEXT,
    raw JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

- [ ] **Step 2: Write the down migration**

```sql
-- db/migrations/000003_products.down.sql
DROP TABLE IF EXISTS products;
```

- [ ] **Step 3: Apply the migration against the running dev Postgres container (host port 5433 in this environment — check `docker compose ps` first; if postgres isn't running, `docker compose up -d postgres` before migrating)**

Run:
```bash
migrate -database "postgres://crawl:crawl@localhost:5433/crawl?sslmode=disable" -path db/migrations up
```
Expected: output ends with `3/u products (X.XXXms)` and no error.

- [ ] **Step 4: Commit**

```bash
git add db/migrations/000003_products.up.sql db/migrations/000003_products.down.sql
git commit -m "feat: add products table migration"
```

---

## Task 2: `pkg/crawl4ai` client

**Files:**
- Create: `pkg/crawl4ai/client.go`
- Test: `pkg/crawl4ai/client_test.go`

**Interfaces:**
- Produces:
  - `crawl4ai.CrawlResult{Markdown string, Success bool, Error string}`
  - `crawl4ai.Client` interface: `Crawl(ctx context.Context, url string) (CrawlResult, error)`
  - `crawl4ai.NewHTTPClient(baseURL string, httpClient *http.Client) *HTTPClient`

- [ ] **Step 1: Write the failing test**

```go
// pkg/crawl4ai/client_test.go
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
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./pkg/crawl4ai/...`
Expected: FAIL — `undefined: NewHTTPClient`

- [ ] **Step 3: Write `pkg/crawl4ai/client.go`**

```go
package crawl4ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// CrawlResult is the crawl4ai-service response for one URL. Success=false
// represents a crawl-level failure (bad URL, timeout, blocked page) reported
// by the service itself, distinct from a Go-level transport/HTTP error.
type CrawlResult struct {
	Markdown string
	Success  bool
	Error    string
}

type Client interface {
	Crawl(ctx context.Context, url string) (CrawlResult, error)
}

type HTTPClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewHTTPClient(baseURL string, httpClient *http.Client) *HTTPClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &HTTPClient{baseURL: baseURL, httpClient: httpClient}
}

type crawlRequest struct {
	URL string `json:"url"`
}

func (c *HTTPClient) Crawl(ctx context.Context, url string) (CrawlResult, error) {
	payload, err := json.Marshal(crawlRequest{URL: url})
	if err != nil {
		return CrawlResult{}, fmt.Errorf("crawl4ai: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/crawl", bytes.NewReader(payload))
	if err != nil {
		return CrawlResult{}, fmt.Errorf("crawl4ai: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return CrawlResult{}, fmt.Errorf("crawl4ai: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return CrawlResult{}, fmt.Errorf("crawl4ai: returned status %d", resp.StatusCode)
	}

	var result CrawlResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return CrawlResult{}, fmt.Errorf("crawl4ai: decode response: %w", err)
	}
	return result, nil
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./pkg/crawl4ai/...`
Expected: PASS (both tests)

- [ ] **Step 5: Commit**

```bash
git add pkg/crawl4ai
git commit -m "feat: add crawl4ai-service HTTP client"
```

---

## Task 3: Extend `pkg/aiproviders` with structured-output `CompleteJSON`

**Files:**
- Modify: `pkg/aiproviders/provider.go`
- Modify: `pkg/aiproviders/openrouter.go`
- Modify: `pkg/aiproviders/openrouter_test.go`

**Interfaces:**
- Produces: `Provider` interface gains `CompleteJSON(ctx context.Context, prompt string, schemaName string, schema json.RawMessage) (json.RawMessage, error)`. `OpenRouter` implements it. This is a strictly additive change — `Complete` is untouched, existing callers (`ai-service`) keep working unmodified.

- [ ] **Step 1: Read the current files to confirm exact starting content**

`pkg/aiproviders/provider.go` currently is:
```go
package aiproviders

import "context"

// Result is a normalized response from any AI provider.
type Result struct {
	Model   string
	Content string
}

// Provider is implemented by each AI backend (OpenRouter, TokenRouter, ...).
type Provider interface {
	Complete(ctx context.Context, prompt string) (Result, error)
}
```

- [ ] **Step 2: Write the failing test in `pkg/aiproviders/openrouter_test.go` (append to the existing file, keep the existing `TestOpenRouter_Complete` test unchanged)**

```go
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
```

Add `"context"` and `"encoding/json"` to the test file's imports if not already present (the existing `TestOpenRouter_Complete` test already imports `"context"`; check `"encoding/json"` and add if missing).

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./pkg/aiproviders/...`
Expected: FAIL — `undefined: CompleteJSON` (or a compile error on the `Provider` interface if you update it before the implementation — either way, confirm a RED state before proceeding)

- [ ] **Step 4: Update `pkg/aiproviders/provider.go`**

```go
package aiproviders

import (
	"context"
	"encoding/json"
)

// Result is a normalized response from any AI provider.
type Result struct {
	Model   string
	Content string
}

// Provider is implemented by each AI backend (OpenRouter, TokenRouter, ...).
type Provider interface {
	Complete(ctx context.Context, prompt string) (Result, error)

	// CompleteJSON asks the model to respond strictly conforming to the
	// given JSON Schema (via the provider's structured-output feature) and
	// returns the raw JSON response content.
	CompleteJSON(ctx context.Context, prompt string, schemaName string, schema json.RawMessage) (json.RawMessage, error)
}
```

- [ ] **Step 5: Implement `CompleteJSON` in `pkg/aiproviders/openrouter.go`**

Add this method to the existing file (the existing `Complete` method, `NewOpenRouter`, `openRouterRequest`/`openRouterResponse` types, and imports all stay as-is — only add the following):

```go
type openRouterJSONRequest struct {
	Model    string `json:"model"`
	Messages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
	ResponseFormat struct {
		Type       string `json:"type"`
		JSONSchema struct {
			Name   string          `json:"name"`
			Strict bool            `json:"strict"`
			Schema json.RawMessage `json:"schema"`
		} `json:"json_schema"`
	} `json:"response_format"`
}

func (o *OpenRouter) CompleteJSON(ctx context.Context, prompt string, schemaName string, schema json.RawMessage) (json.RawMessage, error) {
	reqBody := openRouterJSONRequest{Model: o.model}
	reqBody.Messages = []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}{{Role: "user", Content: prompt}}
	reqBody.ResponseFormat.Type = "json_schema"
	reqBody.ResponseFormat.JSONSchema.Name = schemaName
	reqBody.ResponseFormat.JSONSchema.Strict = true
	reqBody.ResponseFormat.JSONSchema.Schema = schema

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("aiproviders: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("aiproviders: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.apiKey)

	resp, err := o.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("aiproviders: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("aiproviders: openrouter returned status %d", resp.StatusCode)
	}

	var out openRouterResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("aiproviders: decode response: %w", err)
	}
	if len(out.Choices) == 0 {
		return nil, fmt.Errorf("aiproviders: openrouter returned no choices")
	}
	return json.RawMessage(out.Choices[0].Message.Content), nil
}
```

Note this reuses the existing `openRouterResponse` type (unchanged) to decode the response, since the response shape is the same regardless of whether structured output was requested — only the request shape differs.

- [ ] **Step 6: Run the test to verify it passes**

Run: `go test ./pkg/aiproviders/...`
Expected: PASS (both `TestOpenRouter_Complete` and `TestOpenRouter_CompleteJSON`)

- [ ] **Step 7: Build the whole repo to confirm the interface change doesn't break `ai-service`**

Run: `go build ./...`
Expected: succeeds (`ai-service`'s `worker.Consumer.Provider aiproviders.Provider` field only ever calls `.Complete`, never `.CompleteJSON`, so adding a method to the interface doesn't break it as long as `OpenRouter` — the only concrete implementation — implements both).

- [ ] **Step 8: Commit**

```bash
git add pkg/aiproviders
git commit -m "feat: add structured-output CompleteJSON to aiproviders.Provider"
```

---

## Task 4: `crawl4ai-service` (Python)

**Files:**
- Create: `crawl4ai-service/requirements.txt`
- Create: `crawl4ai-service/main.py`
- Create: `crawl4ai-service/Dockerfile`

**Interfaces:**
- Produces: `POST /crawl {"url": "..."}` → `{"success": bool, "markdown": string, "error": string}`, matching exactly what Task 2's `pkg/crawl4ai.Client` expects. `GET /health` → `{"status": "ok"}`.

- [ ] **Step 1: Write `crawl4ai-service/requirements.txt`**

```
crawl4ai==0.6.3
fastapi==0.115.6
uvicorn[standard]==0.34.0
```

- [ ] **Step 2: Write `crawl4ai-service/main.py`**

```python
from fastapi import FastAPI
from pydantic import BaseModel
from crawl4ai import AsyncWebCrawler

app = FastAPI()


class CrawlRequest(BaseModel):
    url: str


class CrawlResponse(BaseModel):
    success: bool
    markdown: str = ""
    error: str = ""


@app.post("/crawl", response_model=CrawlResponse)
async def crawl(req: CrawlRequest) -> CrawlResponse:
    try:
        async with AsyncWebCrawler() as crawler:
            result = await crawler.arun(url=req.url)
            if not result.success:
                return CrawlResponse(success=False, error=str(getattr(result, "error_message", "crawl failed")))
            markdown = result.markdown
            # crawl4ai's `markdown` field has varied in shape across
            # versions (plain string vs. an object with a `.raw_markdown`
            # attribute) — handle both.
            if not isinstance(markdown, str):
                markdown = getattr(markdown, "raw_markdown", str(markdown))
            return CrawlResponse(success=True, markdown=markdown or "")
    except Exception as e:
        return CrawlResponse(success=False, error=str(e))


@app.get("/health")
async def health() -> dict:
    return {"status": "ok"}
```

- [ ] **Step 3: Write `crawl4ai-service/Dockerfile`**

```dockerfile
FROM python:3.12-slim

WORKDIR /app

RUN apt-get update && apt-get install -y --no-install-recommends \
    wget gnupg \
    && rm -rf /var/lib/apt/lists/*

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
RUN crawl4ai-setup

COPY main.py .

EXPOSE 8000
CMD ["uvicorn", "main:app", "--host", "0.0.0.0", "--port", "8000"]
```

`crawl4ai-setup` is `crawl4ai`'s own post-install command that installs the Playwright browser binaries it needs — do not replace it with a manual `playwright install` unless `crawl4ai-setup` is missing from the installed version (check `pip show crawl4ai` / the package's own docs if the build fails here, and adapt).

- [ ] **Step 4: Build the image and verify it actually crawls a real page — this is the step that validates the assumed `crawl4ai` API surface**

Run:
```bash
docker build -t crawl4ai-service-test crawl4ai-service/
docker run -d --name crawl4ai-test -p 8000:8000 crawl4ai-service-test
```
Wait a few seconds for the server to start, then:
```bash
curl -s -X POST http://localhost:8000/crawl -H 'Content-Type: application/json' -d '{"url":"https://example.com"}' | python3 -m json.tool
```
Expected: `{"success": true, "markdown": "...", "error": ""}` with non-empty markdown containing recognizable content from example.com (e.g. "Example Domain"). **If the actual response shape differs from what `main.py` assumes (e.g. `result.success` doesn't exist, or `result.markdown` behaves differently than the `isinstance` fallback handles), fix `main.py` now based on the real behavior you observe** — check `docker logs crawl4ai-test` for a Python traceback if the request errors, adapt the code, rebuild, and re-test until a real crawl succeeds. Note any such adaptation in your report.

Clean up the test container after verifying:
```bash
docker rm -f crawl4ai-test
```

- [ ] **Step 5: Commit**

```bash
git add crawl4ai-service
git commit -m "feat: add crawl4ai-service (Python, HTML to Markdown crawling)"
```

---

## Task 5: `product-extractor-service` (Go worker)

**Files:**
- Create: `cmd/product-extractor-service/etc/extractor.yaml`
- Create: `cmd/product-extractor-service/etc/extractor.docker.yaml`
- Create: `cmd/product-extractor-service/internal/worker/consumer.go`
- Test: `cmd/product-extractor-service/internal/worker/consumer_test.go`
- Create: `cmd/product-extractor-service/main.go`
- Create: `cmd/product-extractor-service/Dockerfile`

**Interfaces:**
- Consumes: `pkg/db.Connect`, `pkg/rabbitmq.Consumer`, `pkg/crawl4ai.Client` (Task 2), `pkg/aiproviders.Provider.CompleteJSON` (Task 3).
- Produces: a RabbitMQ consumer bound to the EXISTING `crawl.completed.fbads` routing key, queue name `product-extractor.fbads.completed` (a new, distinct queue name from `ai-service`'s `ai.fbads.completed` — RabbitMQ topic exchanges fan out to every queue bound to a matching routing key, so both consumers receive every message independently).

- [ ] **Step 1: Write `cmd/product-extractor-service/etc/extractor.yaml` (host dev config)**

```yaml
Postgres:
  DSN: postgres://crawl:crawl@localhost:5433/crawl?sslmode=disable
RabbitMQ:
  URL: amqp://guest:guest@localhost:5673/
  Exchange: crawl
OpenRouter:
  ApiKey: ${OPENROUTER_API_KEY}
  Model: nvidia/nemotron-3-super-120b-a12b:free
  BaseURL: https://openrouter.ai/api/v1
Crawl4ai:
  BaseURL: http://localhost:8000
```

- [ ] **Step 2: Write `cmd/product-extractor-service/etc/extractor.docker.yaml` (containerized config)**

Identical to Step 1 except:
```yaml
Postgres:
  DSN: postgres://crawl:crawl@postgres:5432/crawl?sslmode=disable
RabbitMQ:
  URL: amqp://guest:guest@rabbitmq:5672/
  Exchange: crawl
Crawl4ai:
  BaseURL: http://crawl4ai-service:8000
```

- [ ] **Step 3: Write the consumer worker**

```go
// cmd/product-extractor-service/internal/worker/consumer.go
package worker

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/hiepnv/crawl-ecomerce-golang/pkg/aiproviders"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/crawl4ai"
)

type CompletedMessage struct {
	JobID string `json:"job_id"`
}

const productSchemaJSON = `{
  "type": "object",
  "properties": {
    "products": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "product_name": {"type": "string"},
          "price": {"type": "number"},
          "currency": {"type": "string"},
          "sku": {"type": "string"}
        },
        "required": ["product_name", "price", "currency", "sku"],
        "additionalProperties": false
      }
    }
  },
  "required": ["products"],
  "additionalProperties": false
}`

type extractedProduct struct {
	ProductName string  `json:"product_name"`
	Price       float64 `json:"price"`
	Currency    string  `json:"currency"`
	SKU         string  `json:"sku"`
}

type extractedProducts struct {
	Products []extractedProduct `json:"products"`
}

// Consumer reads crawl.completed.fbads events (the same event ai-service
// consumes) and, for each distinct link_url in the job's crawled ads,
// crawls the destination page and extracts product info via structured
// AI output. It never modifies jobs.status — that belongs to ai-service.
// Per-URL failures (crawl or extraction) are logged and skipped; they
// never fail the whole message, since one broken destination link should
// not block extraction for the job's other URLs.
type Consumer struct {
	DB       *sql.DB
	Crawler  crawl4ai.Client
	Provider aiproviders.Provider
}

func (c *Consumer) HandleMessage(body []byte) error {
	var msg CompletedMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		return fmt.Errorf("worker: unmarshal completed message: %w", err)
	}

	ctx := context.Background()

	rows, err := c.DB.QueryContext(ctx,
		`SELECT DISTINCT link_url, (array_agg(id))[1] FROM fbads_raw WHERE job_id = $1 AND link_url IS NOT NULL AND link_url != '' GROUP BY link_url`,
		msg.JobID,
	)
	if err != nil {
		return fmt.Errorf("worker: load distinct link_urls: %w", err)
	}

	type urlAd struct {
		URL  string
		AdID string
	}
	var urls []urlAd
	for rows.Next() {
		var u urlAd
		if err := rows.Scan(&u.URL, &u.AdID); err != nil {
			rows.Close()
			return fmt.Errorf("worker: scan link_url row: %w", err)
		}
		urls = append(urls, u)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("worker: iterate link_url rows: %w", err)
	}

	for _, u := range urls {
		c.extractOne(ctx, msg.JobID, u.AdID, u.URL)
	}
	return nil
}

// extractOne handles a single URL's crawl+extract+save. Any failure is
// logged and swallowed — see the Consumer doc comment on why this never
// returns an error to HandleMessage's caller.
func (c *Consumer) extractOne(ctx context.Context, jobID, adID, url string) {
	crawlResult, err := c.Crawler.Crawl(ctx, url)
	if err != nil {
		fmt.Println("worker: crawl4ai request failed for", url, ":", err)
		return
	}
	if !crawlResult.Success {
		fmt.Println("worker: crawl4ai reported failure for", url, ":", crawlResult.Error)
		return
	}
	if crawlResult.Markdown == "" {
		fmt.Println("worker: crawl4ai returned empty markdown for", url)
		return
	}

	prompt := fmt.Sprintf("Extract every distinct product mentioned on this page, with its name, price, currency, and SKU if present. If no SKU is given, invent a short stable one from the product name. Page content (Markdown):\n%s", crawlResult.Markdown)
	raw, err := c.Provider.CompleteJSON(ctx, prompt, "product_extraction", []byte(productSchemaJSON))
	if err != nil {
		fmt.Println("worker: AI extraction failed for", url, ":", err)
		return
	}

	var extracted extractedProducts
	if err := json.Unmarshal(raw, &extracted); err != nil {
		fmt.Println("worker: failed to parse extracted products for", url, ":", err)
		return
	}

	for _, p := range extracted.Products {
		productJSON, err := json.Marshal(p)
		if err != nil {
			fmt.Println("worker: failed to marshal product for storage:", err)
			continue
		}
		if _, err := c.DB.ExecContext(ctx,
			`INSERT INTO products (job_id, ad_id, url, product_name, price, currency, sku, raw) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			jobID, adID, url, p.ProductName, p.Price, p.Currency, p.SKU, productJSON,
		); err != nil {
			fmt.Println("worker: failed to insert product for", url, ":", err)
		}
	}
}
```

- [ ] **Step 4: Write the failing tests for the pre-DB, pre-network failure paths**

```go
// cmd/product-extractor-service/internal/worker/consumer_test.go
package worker

import (
	"testing"
)

func TestHandleMessage_UnmarshalError(t *testing.T) {
	c := &Consumer{}
	err := c.HandleMessage([]byte("not json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}
```

Note: unlike `trend-service`/`fb-ads-service`/`ai-service`, this consumer's `extractOne` swallows all its own errors by design (per the Global Constraints — a broken URL must not nack the whole message), so there is no equivalent "external API error propagates" test to write here; that behavior is exercised in Task 6's manual smoke test against the live stack instead.

- [ ] **Step 5: Run the test to verify RED then GREEN**

Run: `go test ./cmd/product-extractor-service/...`
Before Step 3's file exists: FAIL — `undefined: Consumer`. After Step 3: PASS.

- [ ] **Step 6: Write `cmd/product-extractor-service/main.go`**

```go
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/hiepnv/crawl-ecomerce-golang/cmd/product-extractor-service/internal/worker"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/aiproviders"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/crawl4ai"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/db"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/rabbitmq"

	"github.com/zeromicro/go-zero/core/conf"
)

type Config struct {
	Postgres struct {
		DSN string
	}
	RabbitMQ struct {
		URL      string
		Exchange string
	}
	OpenRouter struct {
		ApiKey  string
		Model   string
		BaseURL string
	}
	Crawl4ai struct {
		BaseURL string
	}
}

var configFile = flag.String("f", "etc/extractor.yaml", "config file")

func main() {
	flag.Parse()

	var c Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())

	if c.OpenRouter.ApiKey == "" || strings.Contains(c.OpenRouter.ApiKey, "${") {
		panic("product-extractor-service: OpenRouter.ApiKey is empty or unexpanded (set OPENROUTER_API_KEY env var)")
	}

	conn, err := db.Connect(c.Postgres.DSN)
	if err != nil {
		panic(err)
	}

	provider := aiproviders.NewOpenRouter(c.OpenRouter.ApiKey, c.OpenRouter.Model, c.OpenRouter.BaseURL, http.DefaultClient)
	crawler := crawl4ai.NewHTTPClient(c.Crawl4ai.BaseURL, &http.Client{Timeout: 2 * time.Minute}) // crawl4ai pages can be slow

	consumer, err := rabbitmq.NewConsumer(rabbitmq.Config{URL: c.RabbitMQ.URL, Exchange: c.RabbitMQ.Exchange})
	if err != nil {
		panic(err)
	}

	w := &worker.Consumer{DB: conn, Crawler: crawler, Provider: provider}
	fmt.Println("Starting product-extractor-service consumer...")
	if err := consumer.Consume(context.Background(), "product-extractor.fbads.completed", "crawl.completed.fbads", w.HandleMessage); err != nil {
		fmt.Fprintln(os.Stderr, "product-extractor-service consumer stopped:", err)
		os.Exit(1)
	}
}
```

- [ ] **Step 7: Build**

Run: `go build ./cmd/product-extractor-service/...`
Expected: succeeds.

- [ ] **Step 8: Write `cmd/product-extractor-service/Dockerfile`**

Same pattern as `cmd/ai-service/Dockerfile` (golang:1.25-alpine build stage, `CGO_ENABLED=0`, alpine:3.20 runtime), binary name `product-extractor-service`, CMD referencing `/etc/product-extractor-service/extractor.docker.yaml`.

- [ ] **Step 9: Commit**

```bash
git add cmd/product-extractor-service
git commit -m "feat: implement product-extractor-service worker"
```

---

## Task 6: Wire into Docker Compose, update docs

**Files:**
- Modify: `docker-compose.yml`
- Modify: `README.md`

**Interfaces:**
- Produces: a runnable `docker compose up` stack including `crawl4ai-service` and `product-extractor-service`.

- [ ] **Step 1: Add `crawl4ai-service` to `docker-compose.yml`**

```yaml
  crawl4ai-service:
    build:
      context: ./crawl4ai-service
    restart: on-failure:5
```

No port needs publishing to the host (only `product-extractor-service` talks to it, over the internal Docker network at `http://crawl4ai-service:8000`) and no `depends_on` (it's stateless, no DB/queue dependency).

- [ ] **Step 2: Add `product-extractor-service` to `docker-compose.yml`**

```yaml
  product-extractor-service:
    build:
      context: .
      dockerfile: cmd/product-extractor-service/Dockerfile
    environment:
      OPENROUTER_API_KEY: ${OPENROUTER_API_KEY}
    depends_on:
      postgres:
        condition: service_healthy
      rabbitmq:
        condition: service_healthy
      migrate:
        condition: service_completed_successfully
      crawl4ai-service:
        condition: service_started
    restart: on-failure:5
```

- [ ] **Step 3: Validate compose syntax**

Run: `docker compose config`
Expected: exit 0, no warnings; confirm `crawl4ai-service` and `product-extractor-service` both appear correctly wired.

- [ ] **Step 4: Bring up the two new services and verify end-to-end with a real fbads job**

Run:
```bash
docker compose up -d --build
```
Wait for containers to stabilize, then trigger a real job (reuse the existing smoke test's fbads path, or `curl` directly):
```bash
curl -s -X POST http://localhost:8888/jobs/fbads -H 'Content-Type: application/json' -d '{"query":"shoes","country":"US"}' | jq .
```
Wait ~30-60s for the job to reach `done` (poll `GET /jobs/fbads/{id}` as in `scripts/smoke_test.sh`), then check the `products` table was populated:
```bash
docker compose exec -T postgres psql -U crawl -d crawl -c "SELECT job_id, url, product_name, price, currency, sku FROM products ORDER BY created_at DESC LIMIT 10;"
```
Expected: at least some rows (not necessarily one per ad — some destination pages may not be product pages, or crawl4ai/extraction may legitimately fail for some URLs per the Global Constraints' per-URL failure tolerance). If ZERO rows appear across multiple real job runs, treat that as a real bug to investigate (check `docker compose logs product-extractor-service crawl4ai-service` for the `fmt.Println` failure messages `extractOne` emits) before considering this task done — not every individual failure is expected, but total silence across a whole job is.

- [ ] **Step 5: Update `README.md`**

Add a short paragraph mentioning the product-extraction enrichment pipeline (crawl4ai-service + product-extractor-service), that it runs automatically after any `fbads` job completes crawling (no separate trigger needed), and a one-liner on how to inspect results (`SELECT * FROM products WHERE job_id = '...'`).

- [ ] **Step 6: Commit**

```bash
git add docker-compose.yml README.md
git commit -m "chore: wire crawl4ai-service and product-extractor-service into docker-compose"
```

---

## Self-Review Notes

- **Spec coverage:** `products` migration (Task 1), `pkg/crawl4ai` client (Task 2), `pkg/aiproviders.CompleteJSON` structured output (Task 3, verified against a live spike call before this plan was written), `crawl4ai-service` Python service (Task 4), `product-extractor-service` orchestrator (Task 5), docker-compose wiring + live verification (Task 6) all covered.
- **Type consistency:** `pkg/crawl4ai.CrawlResult{Markdown, Success, Error}` JSON field names (`markdown`, `success`, `error`) match exactly between Task 2 (Go client) and Task 4 (Python service response model). `pkg/aiproviders.Provider.CompleteJSON` signature defined once in Task 3, consumed identically in Task 5. `products` table columns (Task 1) match Task 5's INSERT statement column-for-column.
- **Explicit deviation from prior plans' error-handling pattern:** this consumer ACKs on partial/total per-URL failure rather than NACKing to the DLQ (documented in Global Constraints and the `Consumer` doc comment) — a deliberate, spec-mandated choice given a broken destination link is expected/common and must not block extraction for the job's other URLs, unlike `trend-service`/`fb-ads-service`/`ai-service` where any failure legitimately means the whole job failed.
- **Known follow-ups (out of scope for this plan):** a REST endpoint on api-gateway to list `products` for a job, retry logic for transient crawl4ai failures, proxy-broker, TokenRouter, scheduler/cron.
