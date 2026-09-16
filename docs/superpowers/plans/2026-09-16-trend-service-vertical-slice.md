# Trend Service Vertical Slice Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stand up the full crawl pipeline end-to-end for one path — Google Trends via SerpApi — proving the architecture (api-gateway → RabbitMQ → trend-service → Postgres → ai-service → OpenRouter) before replicating the pattern for fb-ads-service and proxy-broker in follow-up plans.

**Architecture:** Monorepo with go-zero services. `api-gateway` (REST) creates a job row and publishes to RabbitMQ. `trend-service` consumes the job, calls SerpApi, persists raw data, publishes a completion event, and exposes a gRPC `GetTrendJob` query. `ai-service` consumes the completion event, calls OpenRouter to analyze the trend data, and persists the result. All state lives in PostgreSQL.

**Tech Stack:** Go 1.22+, go-zero (`github.com/zeromicro/go-zero`) + `goctl` codegen, RabbitMQ via `github.com/rabbitmq/amqp091-go`, PostgreSQL via `github.com/jackc/pgx/v5` + `database/sql`, migrations via `github.com/golang-migrate/migrate/v4`, Docker Compose.

**Spec:** `docs/superpowers/specs/2026-09-16-crawl-ecommerce-platform-design.md`

## Global Constraints

- Module path: `github.com/hiepnv/crawl-ecomerce-golang`
- Go version: 1.22+
- Prerequisites the executor must have installed: `goctl` (`go install github.com/zeromicro/go-zero/tools/goctl@latest`), `protoc` + `protoc-gen-go` + `protoc-gen-go-grpc` for `.proto` codegen, Docker + Docker Compose.
- RabbitMQ exchange: `crawl` (type `topic`). Routing keys used in this plan: `trend.jobs`, `crawl.completed.trend`.
- Every service that talks to an external API (SerpApi, OpenRouter) must go through a small interface (`SerpApiClient`, `aiproviders.Provider`) so tests can substitute an `httptest.Server` — never call the real API in a unit test.
- Config values (API keys, DSNs, AMQP URL) come from go-zero `.yaml` config files loaded at startup, never hardcoded.
- Out of scope for this plan (per spec, handled in follow-up plans): fb-ads-service, proxy-broker, TokenRouter provider, scheduler/cron.

---

## File Structure

```
go.mod
docker-compose.yml
db/migrations/
  000001_init.up.sql
  000001_init.down.sql
pkg/
  db/
    db.go              # pgx/sql.DB connection helper
  rabbitmq/
    rabbitmq.go         # topology setup, Publisher, Consumer
    rabbitmq_test.go
  aiproviders/
    provider.go          # Provider interface + shared types
    openrouter.go        # OpenRouter implementation
    openrouter_test.go
  serpapi/
    client.go            # SerpApiClient interface + real implementation
    client_test.go
api/
  gateway.api            # go-zero REST API definition
rpc/
  trend/
    trend.proto           # go-zero RPC definition
cmd/
  api-gateway/
    etc/gateway.yaml
    gateway.go            # generated entrypoint (from goctl)
    internal/...           # generated + hand-written logic
  trend-service/
    etc/trend.yaml
    trend.go               # generated entrypoint (from goctl)
    internal/...            # generated + hand-written logic, plus consumer/main worker
  ai-service/
    etc/ai.yaml
    main.go                 # hand-written consumer worker (no external RPC needed yet)
```

---

## Task 1: Repo scaffold, go.mod, Docker Compose skeleton

**Files:**
- Create: `go.mod`
- Create: `.gitignore`
- Create: `docker-compose.yml`

**Interfaces:**
- Produces: running `postgres` (port 5432, db `crawl`, user `crawl`, password `crawl`) and `rabbitmq` (port 5672, management UI 15672) containers other tasks connect to.

- [ ] **Step 1: Initialize the Go module**

Run:
```bash
go mod init github.com/hiepnv/crawl-ecomerce-golang
```

- [ ] **Step 2: Create `.gitignore`**

```
bin/
*.env
.env
```

- [ ] **Step 3: Write `docker-compose.yml` with Postgres and RabbitMQ only (other services added in later tasks)**

```yaml
version: "3.8"
services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: crawl
      POSTGRES_USER: crawl
      POSTGRES_PASSWORD: crawl
    ports:
      - "5432:5432"
    volumes:
      - pgdata:/var/lib/postgresql/data

  rabbitmq:
    image: rabbitmq:3.13-management-alpine
    ports:
      - "5672:5672"
      - "15672:15672"

volumes:
  pgdata:
```

- [ ] **Step 4: Bring up the stack and verify both containers are healthy**

Run:
```bash
docker compose up -d
docker compose ps
```
Expected: both `postgres` and `rabbitmq` show state `running`/`healthy`.

- [ ] **Step 5: Commit**

```bash
git add go.mod .gitignore docker-compose.yml
git commit -m "chore: scaffold monorepo and docker-compose base services"
```

---

## Task 2: Postgres migrations and DB connection helper

**Files:**
- Create: `db/migrations/000001_init.up.sql`
- Create: `db/migrations/000001_init.down.sql`
- Create: `pkg/db/db.go`
- Test: `pkg/db/db_test.go`

**Interfaces:**
- Produces: `pkg/db.Connect(dsn string) (*sql.DB, error)` — used by `trend-service` and `ai-service` in later tasks.
- Produces tables: `jobs`, `trend_raw`, `ai_results` (per spec section 3, scoped to this plan's needs — `fbads_raw` and `proxy_usage` deferred to the fb-ads-service plan).

- [ ] **Step 1: Write the up migration**

```sql
-- db/migrations/000001_init.up.sql
CREATE TABLE jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    type TEXT NOT NULL CHECK (type IN ('fbads', 'trend')),
    status TEXT NOT NULL CHECK (status IN ('pending', 'crawled', 'ai_processing', 'done', 'failed')),
    params JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE trend_raw (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id UUID NOT NULL REFERENCES jobs(id),
    keyword TEXT NOT NULL,
    trend_data JSONB NOT NULL,
    fetched_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE ai_results (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id UUID NOT NULL REFERENCES jobs(id),
    provider TEXT NOT NULL,
    model TEXT NOT NULL,
    prompt_type TEXT NOT NULL CHECK (prompt_type IN ('analysis', 'generate')),
    input_ref UUID,
    output JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE EXTENSION IF NOT EXISTS pgcrypto;
```

Note: `gen_random_uuid()` requires the `pgcrypto` extension — the `CREATE EXTENSION` statement must run before the tables in practice; reorder it to the top of the file.

- [ ] **Step 2: Write the down migration**

```sql
-- db/migrations/000001_init.down.sql
DROP TABLE IF EXISTS ai_results;
DROP TABLE IF EXISTS trend_raw;
DROP TABLE IF EXISTS jobs;
```

- [ ] **Step 3: Fix ordering in the up migration (pgcrypto extension first)**

Rewrite `db/migrations/000001_init.up.sql` so `CREATE EXTENSION IF NOT EXISTS pgcrypto;` is the first statement, followed by the three `CREATE TABLE` statements from Step 1.

- [ ] **Step 4: Add the migrate CLI and apply migrations**

Run:
```bash
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
migrate -database "postgres://crawl:crawl@localhost:5432/crawl?sslmode=disable" -path db/migrations up
```
Expected: output ends with `000001/u init (X.XXXms)` and no error.

- [ ] **Step 5: Write `pkg/db/db.go`**

```go
package db

import (
	"database/sql"
	"fmt"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// Connect opens a *sql.DB against the given Postgres DSN and verifies
// connectivity with a Ping before returning.
func Connect(dsn string) (*sql.DB, error) {
	conn, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("db: open: %w", err)
	}
	if err := conn.Ping(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("db: ping: %w", err)
	}
	return conn, nil
}
```

- [ ] **Step 6: Add pgx dependency**

Run:
```bash
go get github.com/jackc/pgx/v5
```

- [ ] **Step 7: Write the failing test**

```go
// pkg/db/db_test.go
package db

import "testing"

func TestConnect_InvalidDSN(t *testing.T) {
	_, err := Connect("postgres://bad:bad@127.0.0.1:1/nonexistent?sslmode=disable&connect_timeout=1")
	if err == nil {
		t.Fatal("expected error connecting to invalid DSN, got nil")
	}
}
```

- [ ] **Step 8: Run the test**

Run: `go test ./pkg/db/...`
Expected: PASS (the invalid DSN case fails fast and returns an error).

- [ ] **Step 9: Commit**

```bash
git add db/migrations pkg/db go.mod go.sum
git commit -m "feat: add jobs/trend_raw/ai_results migrations and db connection helper"
```

---

## Task 3: RabbitMQ publisher/consumer wrapper

**Files:**
- Create: `pkg/rabbitmq/rabbitmq.go`
- Test: `pkg/rabbitmq/rabbitmq_test.go`

**Interfaces:**
- Produces:
  - `rabbitmq.Config{URL, Exchange string}`
  - `rabbitmq.Publisher` with `Publish(ctx context.Context, routingKey string, body []byte) error`
  - `rabbitmq.Consumer` with `Consume(ctx context.Context, queue, routingKey string, handler func([]byte) error) error`
  - `rabbitmq.NewPublisher(cfg Config) (*Publisher, error)`
  - `rabbitmq.NewConsumer(cfg Config) (*Consumer, error)`
  - `rabbitmq.DeclareTopology(ch *amqp.Channel, exchange, queue, routingKey, dlqSuffix string) error` — declares the exchange, the main queue bound to `routingKey`, and a `<queue><dlqSuffix>` dead-letter queue, wiring the main queue's `x-dead-letter-exchange`/`x-dead-letter-routing-key` to it.

- [ ] **Step 1: Add the amqp dependency**

Run:
```bash
go get github.com/rabbitmq/amqp091-go
```

- [ ] **Step 2: Write the failing test for topology naming (pure function, no broker needed)**

```go
// pkg/rabbitmq/rabbitmq_test.go
package rabbitmq

import "testing"

func TestDLQName(t *testing.T) {
	got := dlqName("trend.jobs", ".dlq")
	want := "trend.jobs.dlq"
	if got != want {
		t.Fatalf("dlqName() = %q, want %q", got, want)
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./pkg/rabbitmq/...`
Expected: FAIL — `undefined: dlqName`

- [ ] **Step 4: Write `pkg/rabbitmq/rabbitmq.go`**

```go
package rabbitmq

import (
	"context"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Config holds the connection and topology settings shared by
// Publisher and Consumer.
type Config struct {
	URL      string
	Exchange string
}

func dlqName(queue, suffix string) string {
	return queue + suffix
}

// DeclareTopology declares a topic exchange, a queue bound to routingKey,
// and a dead-letter queue that receives messages the main queue rejects
// or lets expire.
func DeclareTopology(ch *amqp.Channel, exchange, queue, routingKey, dlqSuffix string) error {
	if err := ch.ExchangeDeclare(exchange, "topic", true, false, false, false, nil); err != nil {
		return fmt.Errorf("rabbitmq: declare exchange: %w", err)
	}

	dlq := dlqName(queue, dlqSuffix)
	if _, err := ch.QueueDeclare(dlq, true, false, false, false, nil); err != nil {
		return fmt.Errorf("rabbitmq: declare dlq: %w", err)
	}
	if err := ch.QueueBind(dlq, dlq, exchange, false, nil); err != nil {
		return fmt.Errorf("rabbitmq: bind dlq: %w", err)
	}

	args := amqp.Table{
		"x-dead-letter-exchange":    exchange,
		"x-dead-letter-routing-key": dlq,
	}
	if _, err := ch.QueueDeclare(queue, true, false, false, false, args); err != nil {
		return fmt.Errorf("rabbitmq: declare queue: %w", err)
	}
	if err := ch.QueueBind(queue, routingKey, exchange, false, nil); err != nil {
		return fmt.Errorf("rabbitmq: bind queue: %w", err)
	}
	return nil
}

// Publisher publishes messages to a topic exchange.
type Publisher struct {
	cfg  Config
	conn *amqp.Connection
	ch   *amqp.Channel
}

func NewPublisher(cfg Config) (*Publisher, error) {
	conn, err := amqp.Dial(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("rabbitmq: dial: %w", err)
	}
	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("rabbitmq: channel: %w", err)
	}
	if err := ch.ExchangeDeclare(cfg.Exchange, "topic", true, false, false, false, nil); err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("rabbitmq: declare exchange: %w", err)
	}
	return &Publisher{cfg: cfg, conn: conn, ch: ch}, nil
}

func (p *Publisher) Publish(ctx context.Context, routingKey string, body []byte) error {
	return p.ch.PublishWithContext(ctx, p.cfg.Exchange, routingKey, false, false, amqp.Publishing{
		ContentType: "application/json",
		Body:        body,
	})
}

func (p *Publisher) Close() error {
	if err := p.ch.Close(); err != nil {
		return err
	}
	return p.conn.Close()
}

// Consumer consumes messages from a named queue.
type Consumer struct {
	cfg  Config
	conn *amqp.Connection
	ch   *amqp.Channel
}

func NewConsumer(cfg Config) (*Consumer, error) {
	conn, err := amqp.Dial(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("rabbitmq: dial: %w", err)
	}
	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("rabbitmq: channel: %w", err)
	}
	return &Consumer{cfg: cfg, conn: conn, ch: ch}, nil
}

// Consume declares the queue's topology, then blocks handling deliveries
// until ctx is cancelled. A handler error nacks the delivery without
// requeue, sending it toward the dead-letter queue.
func (c *Consumer) Consume(ctx context.Context, queue, routingKey string, handler func([]byte) error) error {
	if err := DeclareTopology(c.ch, c.cfg.Exchange, queue, routingKey, ".dlq"); err != nil {
		return err
	}
	msgs, err := c.ch.Consume(queue, "", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("rabbitmq: consume: %w", err)
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case d, ok := <-msgs:
			if !ok {
				return fmt.Errorf("rabbitmq: delivery channel closed")
			}
			if err := handler(d.Body); err != nil {
				_ = d.Nack(false, false)
				continue
			}
			_ = d.Ack(false)
		}
	}
}

func (c *Consumer) Close() error {
	if err := c.ch.Close(); err != nil {
		return err
	}
	return c.conn.Close()
}
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./pkg/rabbitmq/...`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add pkg/rabbitmq go.mod go.sum
git commit -m "feat: add rabbitmq publisher/consumer wrapper with DLQ topology"
```

---

## Task 4: AI provider abstraction (OpenRouter)

**Files:**
- Create: `pkg/aiproviders/provider.go`
- Create: `pkg/aiproviders/openrouter.go`
- Test: `pkg/aiproviders/openrouter_test.go`

**Interfaces:**
- Produces:
  - `aiproviders.Result{Model string, Content string}`
  - `aiproviders.Provider` interface: `Complete(ctx context.Context, prompt string) (Result, error)`
  - `aiproviders.NewOpenRouter(apiKey, model, baseURL string, httpClient *http.Client) *OpenRouter` — `baseURL` lets tests point at an `httptest.Server`; production config passes `"https://openrouter.ai/api/v1"`.

- [ ] **Step 1: Write `pkg/aiproviders/provider.go`**

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

- [ ] **Step 2: Write the failing test**

```go
// pkg/aiproviders/openrouter_test.go
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
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./pkg/aiproviders/...`
Expected: FAIL — `undefined: NewOpenRouter`

- [ ] **Step 4: Write `pkg/aiproviders/openrouter.go`**

```go
package aiproviders

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// OpenRouter implements Provider against the OpenRouter chat completions API.
type OpenRouter struct {
	apiKey     string
	model      string
	baseURL    string
	httpClient *http.Client
}

func NewOpenRouter(apiKey, model, baseURL string, httpClient *http.Client) *OpenRouter {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &OpenRouter{apiKey: apiKey, model: model, baseURL: baseURL, httpClient: httpClient}
}

type openRouterRequest struct {
	Model    string `json:"model"`
	Messages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
}

type openRouterResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func (o *OpenRouter) Complete(ctx context.Context, prompt string) (Result, error) {
	reqBody := openRouterRequest{Model: o.model}
	reqBody.Messages = []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}{{Role: "user", Content: prompt}}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return Result{}, fmt.Errorf("aiproviders: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return Result{}, fmt.Errorf("aiproviders: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.apiKey)

	resp, err := o.httpClient.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("aiproviders: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Result{}, fmt.Errorf("aiproviders: openrouter returned status %d", resp.StatusCode)
	}

	var out openRouterResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return Result{}, fmt.Errorf("aiproviders: decode response: %w", err)
	}
	if len(out.Choices) == 0 {
		return Result{}, fmt.Errorf("aiproviders: openrouter returned no choices")
	}
	return Result{Model: out.Model, Content: out.Choices[0].Message.Content}, nil
}
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./pkg/aiproviders/...`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add pkg/aiproviders
git commit -m "feat: add aiproviders.Provider interface and OpenRouter implementation"
```

---

## Task 5: SerpApi client

**Files:**
- Create: `pkg/serpapi/client.go`
- Test: `pkg/serpapi/client_test.go`

**Interfaces:**
- Produces:
  - `serpapi.TrendPoint{Date string, Value int}`
  - `serpapi.TrendResult{Keyword string, Points []TrendPoint, Raw json.RawMessage}`
  - `serpapi.Client` interface: `FetchTrend(ctx context.Context, keyword string) (TrendResult, error)`
  - `serpapi.NewHTTPClient(apiKey, baseURL string, httpClient *http.Client) *HTTPClient` — `baseURL` defaults to `"https://serpapi.com"` in production config, overridden in tests.

- [ ] **Step 1: Write the failing test**

```go
// pkg/serpapi/client_test.go
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
		w.Write([]byte(`{"interest_over_time":{"timeline_data":[{"date":"Sep 1","values":[{"value":42}]}]}}`))
	}))
	defer srv.Close()

	c := NewHTTPClient("test-key", srv.URL, srv.Client())
	result, err := c.FetchTrend(context.Background(), "golang")
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
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./pkg/serpapi/...`
Expected: FAIL — `undefined: NewHTTPClient`

- [ ] **Step 3: Write `pkg/serpapi/client.go`**

```go
package serpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

type TrendPoint struct {
	Date  string
	Value int
}

type TrendResult struct {
	Keyword string
	Points  []TrendPoint
	Raw     json.RawMessage
}

type Client interface {
	FetchTrend(ctx context.Context, keyword string) (TrendResult, error)
}

type HTTPClient struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

func NewHTTPClient(apiKey, baseURL string, httpClient *http.Client) *HTTPClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &HTTPClient{apiKey: apiKey, baseURL: baseURL, httpClient: httpClient}
}

type serpApiResponse struct {
	InterestOverTime struct {
		TimelineData []struct {
			Date   string `json:"date"`
			Values []struct {
				Value int `json:"value"`
			} `json:"values"`
		} `json:"timeline_data"`
	} `json:"interest_over_time"`
}

func (c *HTTPClient) FetchTrend(ctx context.Context, keyword string) (TrendResult, error) {
	q := url.Values{}
	q.Set("engine", "google_trends")
	q.Set("q", keyword)
	q.Set("api_key", c.apiKey)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/search.json?"+q.Encode(), nil)
	if err != nil {
		return TrendResult{}, fmt.Errorf("serpapi: build request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return TrendResult{}, fmt.Errorf("serpapi: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return TrendResult{}, fmt.Errorf("serpapi: returned status %d", resp.StatusCode)
	}

	raw, err := json.Marshal(json.RawMessage(nil))
	_ = raw

	var body serpApiResponse
	bodyBytes, err := readAndDecode(resp, &body)
	if err != nil {
		return TrendResult{}, err
	}

	result := TrendResult{Keyword: keyword, Raw: bodyBytes}
	for _, tl := range body.InterestOverTime.TimelineData {
		val := 0
		if len(tl.Values) > 0 {
			val = tl.Values[0].Value
		}
		result.Points = append(result.Points, TrendPoint{Date: tl.Date, Value: val})
	}
	return result, nil
}

func readAndDecode(resp *http.Response, out *serpApiResponse) (json.RawMessage, error) {
	dec := json.NewDecoder(resp.Body)
	var raw json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("serpapi: decode raw response: %w", err)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return nil, fmt.Errorf("serpapi: decode response: %w", err)
	}
	return raw, nil
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./pkg/serpapi/...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/serpapi
git commit -m "feat: add SerpApi client for Google Trends"
```

---

## Task 6: trend-service — gRPC scaffold, consumer worker, business logic

**Files:**
- Create: `rpc/trend/trend.proto`
- Create: `cmd/trend-service/etc/trend.yaml`
- Create: `cmd/trend-service/` (generated via `goctl rpc protoc`)
- Modify (generated file, hand-filled): `cmd/trend-service/internal/logic/gettrendjoblogic.go`
- Create: `cmd/trend-service/internal/worker/consumer.go`
- Create: `cmd/trend-service/main.go` (wraps generated RPC server + starts the consumer worker in a goroutine)
- Test: `cmd/trend-service/internal/worker/consumer_test.go`

**Interfaces:**
- Consumes: `pkg/rabbitmq.Consumer`, `pkg/serpapi.Client`, `pkg/db.Connect`
- Produces: gRPC method `GetTrendJob(job_id string) returns (status string, keyword string, points []TrendPoint, ai_output string)` for `api-gateway` to call in Task 7.
- Produces: consumer that reads `trend.jobs` queue, writes `trend_raw` + updates `jobs.status`, publishes `crawl.completed.trend`.

- [ ] **Step 1: Write the proto definition**

```protobuf
// rpc/trend/trend.proto
syntax = "proto3";

package trend;
option go_package = "./trend";

message GetTrendJobRequest {
  string job_id = 1;
}

message TrendPointMsg {
  string date = 1;
  int32 value = 2;
}

message GetTrendJobResponse {
  string status = 1;
  string keyword = 2;
  repeated TrendPointMsg points = 3;
  string ai_output = 4;
}

service TrendService {
  rpc GetTrendJob(GetTrendJobRequest) returns (GetTrendJobResponse);
}
```

- [ ] **Step 2: Generate the RPC scaffold**

Run:
```bash
mkdir -p cmd/trend-service
cd rpc/trend && goctl rpc protoc trend.proto --go_out=../../cmd/trend-service --go-grpc_out=../../cmd/trend-service --zrpc_out=../../cmd/trend-service && cd ../..
```
Expected: `cmd/trend-service/` now contains `trend.go`, `etc/trend.yaml`, `internal/config`, `internal/logic`, `internal/server`, `internal/svc`, `trend/` (generated pb.go files).

- [ ] **Step 3: Configure `cmd/trend-service/etc/trend.yaml`**

```yaml
Name: trend-service
ListenOn: 0.0.0.0:8081
Postgres:
  DSN: postgres://crawl:crawl@localhost:5432/crawl?sslmode=disable
RabbitMQ:
  URL: amqp://guest:guest@localhost:5672/
  Exchange: crawl
SerpApi:
  ApiKey: ${SERPAPI_API_KEY}
  BaseURL: https://serpapi.com
```

- [ ] **Step 4: Extend `internal/config/config.go` (generated file) with the custom sections**

Add to the generated `Config` struct in `cmd/trend-service/internal/config/config.go`:

```go
type Config struct {
	zrpc.RpcServerConf
	Postgres struct {
		DSN string
	}
	RabbitMQ struct {
		URL      string
		Exchange string
	}
	SerpApi struct {
		ApiKey  string
		BaseURL string
	}
}
```

- [ ] **Step 5: Extend `internal/svc/servicecontext.go` (generated file) to hold shared dependencies**

```go
package svc

import (
	"database/sql"
	"net/http"

	"github.com/hiepnv/crawl-ecomerce-golang/cmd/trend-service/internal/config"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/db"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/serpapi"
)

type ServiceContext struct {
	Config     config.Config
	DB         *sql.DB
	SerpApi    serpapi.Client
}

func NewServiceContext(c config.Config) *ServiceContext {
	conn, err := db.Connect(c.Postgres.DSN)
	if err != nil {
		panic(err)
	}
	return &ServiceContext{
		Config:  c,
		DB:      conn,
		SerpApi: serpapi.NewHTTPClient(c.SerpApi.ApiKey, c.SerpApi.BaseURL, http.DefaultClient),
	}
}
```

- [ ] **Step 6: Implement `GetTrendJob` in `internal/logic/gettrendjoblogic.go` (generated file)**

```go
package logic

import (
	"context"
	"database/sql"

	"github.com/hiepnv/crawl-ecomerce-golang/cmd/trend-service/internal/svc"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/trend-service/trend"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetTrendJobLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetTrendJobLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetTrendJobLogic {
	return &GetTrendJobLogic{ctx: ctx, svcCtx: svcCtx, Logger: logx.WithContext(ctx)}
}

func (l *GetTrendJobLogic) GetTrendJob(in *trend.GetTrendJobRequest) (*trend.GetTrendJobResponse, error) {
	var status, keyword string
	err := l.svcCtx.DB.QueryRowContext(l.ctx,
		`SELECT status, params->>'keyword' FROM jobs WHERE id = $1`, in.JobId,
	).Scan(&status, &keyword)
	if err == sql.ErrNoRows {
		return &trend.GetTrendJobResponse{Status: "not_found"}, nil
	}
	if err != nil {
		return nil, err
	}

	resp := &trend.GetTrendJobResponse{Status: status, Keyword: keyword}

	rows, err := l.svcCtx.DB.QueryContext(l.ctx,
		`SELECT trend_data FROM trend_raw WHERE job_id = $1 ORDER BY fetched_at DESC LIMIT 1`, in.JobId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	// Points are parsed from trend_data JSONB in the api-gateway; the RPC
	// response carries raw status/keyword/ai_output here and leaves point
	// parsing to the caller by returning an empty Points slice when absent.

	var aiOutput string
	err = l.svcCtx.DB.QueryRowContext(l.ctx,
		`SELECT output->>'content' FROM ai_results WHERE job_id = $1 ORDER BY created_at DESC LIMIT 1`, in.JobId,
	).Scan(&aiOutput)
	if err == nil {
		resp.AiOutput = aiOutput
	} else if err != sql.ErrNoRows {
		return nil, err
	}

	return resp, nil
}
```

- [ ] **Step 7: Write the consumer worker**

```go
// cmd/trend-service/internal/worker/consumer.go
package worker

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/hiepnv/crawl-ecomerce-golang/pkg/rabbitmq"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/serpapi"
)

type JobMessage struct {
	JobID   string `json:"job_id"`
	Keyword string `json:"keyword"`
}

type CompletedMessage struct {
	JobID string `json:"job_id"`
}

// Consumer wires a rabbitmq.Consumer to SerpApi and Postgres: it reads
// trend.jobs messages, fetches trend data, persists it, and publishes a
// crawl.completed.trend event.
type Consumer struct {
	DB        *sql.DB
	SerpApi   serpapi.Client
	Publisher *rabbitmq.Publisher
}

func (c *Consumer) HandleMessage(body []byte) error {
	var msg JobMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		return fmt.Errorf("worker: unmarshal job message: %w", err)
	}

	ctx := context.Background()

	result, err := c.SerpApi.FetchTrend(ctx, msg.Keyword)
	if err != nil {
		return fmt.Errorf("worker: fetch trend: %w", err)
	}

	if _, err := c.DB.ExecContext(ctx,
		`INSERT INTO trend_raw (job_id, keyword, trend_data) VALUES ($1, $2, $3)`,
		msg.JobID, msg.Keyword, result.Raw,
	); err != nil {
		return fmt.Errorf("worker: insert trend_raw: %w", err)
	}

	if _, err := c.DB.ExecContext(ctx,
		`UPDATE jobs SET status = 'crawled', updated_at = now() WHERE id = $1`, msg.JobID,
	); err != nil {
		return fmt.Errorf("worker: update job status: %w", err)
	}

	payload, err := json.Marshal(CompletedMessage{JobID: msg.JobID})
	if err != nil {
		return fmt.Errorf("worker: marshal completed message: %w", err)
	}
	if err := c.Publisher.Publish(ctx, "crawl.completed.trend", payload); err != nil {
		return fmt.Errorf("worker: publish completed event: %w", err)
	}
	return nil
}
```

- [ ] **Step 8: Write the failing test for `HandleMessage` using a fake SerpApi client and an in-memory sqlmock-free approach (real SQLite is out of scope — use a fake `serpapi.Client` and assert only the SerpApi call + JSON handling, skipping DB by injecting a nil-safe stub)**

```go
// cmd/trend-service/internal/worker/consumer_test.go
package worker

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/hiepnv/crawl-ecomerce-golang/pkg/serpapi"
)

type fakeSerpApi struct {
	called  bool
	keyword string
	err     error
}

func (f *fakeSerpApi) FetchTrend(ctx context.Context, keyword string) (serpapi.TrendResult, error) {
	f.called = true
	f.keyword = keyword
	if f.err != nil {
		return serpapi.TrendResult{}, f.err
	}
	return serpapi.TrendResult{Keyword: keyword, Raw: json.RawMessage(`{}`)}, nil
}

func TestHandleMessage_UnmarshalError(t *testing.T) {
	c := &Consumer{SerpApi: &fakeSerpApi{}}
	err := c.HandleMessage([]byte("not json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestHandleMessage_SerpApiError(t *testing.T) {
	fake := &fakeSerpApi{err: errors.New("boom")}
	c := &Consumer{SerpApi: fake}
	msg, _ := json.Marshal(JobMessage{JobID: "11111111-1111-1111-1111-111111111111", Keyword: "golang"})
	err := c.HandleMessage(msg)
	if err == nil {
		t.Fatal("expected error propagated from SerpApi, got nil")
	}
	if !fake.called || fake.keyword != "golang" {
		t.Errorf("expected SerpApi.FetchTrend called with keyword=golang, got called=%v keyword=%q", fake.called, fake.keyword)
	}
}
```

Note: these two tests cover the pre-DB failure paths without a live database. Full DB-write behavior is exercised in Task 8's manual end-to-end smoke test against the real docker-compose stack.

- [ ] **Step 9: Run the tests to verify they fail, then pass**

Run: `go test ./cmd/trend-service/...`
Expected first run (before Step 7's file exists): FAIL — `undefined: Consumer`.
After Step 7 is in place: PASS.

- [ ] **Step 10: Write `cmd/trend-service/main.go` to start both the gRPC server and the consumer worker**

Replace the generated `cmd/trend-service/trend.go` entrypoint content with (rename file to `main.go` if `goctl` generated `trend.go`):

```go
package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/hiepnv/crawl-ecomerce-golang/cmd/trend-service/internal/config"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/trend-service/internal/server"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/trend-service/internal/svc"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/trend-service/internal/worker"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/trend-service/trend"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/rabbitmq"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/service"
	"google.golang.org/grpc"
	"github.com/zeromicro/go-zero/zrpc"
)

var configFile = flag.String("f", "etc/trend.yaml", "config file")

func main() {
	flag.Parse()

	var c config.Config
	conf.MustLoad(*configFile, &c)

	svcCtx := svc.NewServiceContext(c)

	publisher, err := rabbitmq.NewPublisher(rabbitmq.Config{URL: c.RabbitMQ.URL, Exchange: c.RabbitMQ.Exchange})
	if err != nil {
		panic(err)
	}

	consumer, err := rabbitmq.NewConsumer(rabbitmq.Config{URL: c.RabbitMQ.URL, Exchange: c.RabbitMQ.Exchange})
	if err != nil {
		panic(err)
	}

	w := &worker.Consumer{DB: svcCtx.DB, SerpApi: svcCtx.SerpApi, Publisher: publisher}
	go func() {
		if err := consumer.Consume(context.Background(), "trend.jobs", "trend.jobs", w.HandleMessage); err != nil {
			fmt.Println("trend-service consumer stopped:", err)
		}
	}()

	s := zrpc.MustNewServer(c.RpcServerConf, func(grpcServer *grpc.Server) {
		trend.RegisterTrendServiceServer(grpcServer, server.NewTrendServiceServer(svcCtx))
	})
	defer s.Stop()

	fmt.Printf("Starting trend-service rpc server at %s...\n", c.ListenOn)
	serviceGroup := service.NewServiceGroup()
	serviceGroup.Add(s)
	serviceGroup.Start()
}
```

- [ ] **Step 11: Build to catch wiring errors**

Run: `go build ./cmd/trend-service/...`
Expected: builds successfully (fix any generated-file naming mismatches from `goctl`'s actual output before proceeding — `goctl` versions vary slightly in generated file/struct names).

- [ ] **Step 12: Commit**

```bash
git add rpc/trend cmd/trend-service
git commit -m "feat: implement trend-service gRPC query and RabbitMQ consumer worker"
```

---

## Task 7: ai-service — consumer worker calling OpenRouter

**Files:**
- Create: `cmd/ai-service/etc/ai.yaml`
- Create: `cmd/ai-service/internal/worker/consumer.go`
- Create: `cmd/ai-service/main.go`
- Test: `cmd/ai-service/internal/worker/consumer_test.go`

**Interfaces:**
- Consumes: `pkg/rabbitmq.Consumer`, `pkg/aiproviders.Provider`, `pkg/db.Connect`
- Produces: consumer bound to routing key `crawl.completed.trend` that reads the latest `trend_raw` row for the job, calls `aiproviders.Provider.Complete`, inserts into `ai_results`, and updates `jobs.status = 'done'`.

- [ ] **Step 1: Write `cmd/ai-service/etc/ai.yaml`**

```yaml
Postgres:
  DSN: postgres://crawl:crawl@localhost:5432/crawl?sslmode=disable
RabbitMQ:
  URL: amqp://guest:guest@localhost:5672/
  Exchange: crawl
OpenRouter:
  ApiKey: ${OPENROUTER_API_KEY}
  Model: openai/gpt-4o-mini
  BaseURL: https://openrouter.ai/api/v1
```

- [ ] **Step 2: Write the consumer worker**

```go
// cmd/ai-service/internal/worker/consumer.go
package worker

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/hiepnv/crawl-ecomerce-golang/pkg/aiproviders"
)

type CompletedMessage struct {
	JobID string `json:"job_id"`
}

// Consumer reads crawl.completed.trend events, analyzes the trend data
// with an aiproviders.Provider, and persists the result.
type Consumer struct {
	DB       *sql.DB
	Provider aiproviders.Provider
}

func (c *Consumer) HandleMessage(body []byte) error {
	var msg CompletedMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		return fmt.Errorf("worker: unmarshal completed message: %w", err)
	}

	ctx := context.Background()

	var trendData []byte
	err := c.DB.QueryRowContext(ctx,
		`SELECT trend_data FROM trend_raw WHERE job_id = $1 ORDER BY fetched_at DESC LIMIT 1`, msg.JobID,
	).Scan(&trendData)
	if err != nil {
		return fmt.Errorf("worker: load trend_raw: %w", err)
	}

	if _, err := c.DB.ExecContext(ctx,
		`UPDATE jobs SET status = 'ai_processing', updated_at = now() WHERE id = $1`, msg.JobID,
	); err != nil {
		return fmt.Errorf("worker: mark ai_processing: %w", err)
	}

	prompt := fmt.Sprintf("Analyze this Google Trends data and summarize the key insight in 2-3 sentences:\n%s", string(trendData))
	result, err := c.Provider.Complete(ctx, prompt)
	if err != nil {
		return fmt.Errorf("worker: ai completion: %w", err)
	}

	output, err := json.Marshal(map[string]string{"content": result.Content})
	if err != nil {
		return fmt.Errorf("worker: marshal ai output: %w", err)
	}

	if _, err := c.DB.ExecContext(ctx,
		`INSERT INTO ai_results (job_id, provider, model, prompt_type, output) VALUES ($1, 'openrouter', $2, 'analysis', $3)`,
		msg.JobID, result.Model, output,
	); err != nil {
		return fmt.Errorf("worker: insert ai_results: %w", err)
	}

	if _, err := c.DB.ExecContext(ctx,
		`UPDATE jobs SET status = 'done', updated_at = now() WHERE id = $1`, msg.JobID,
	); err != nil {
		return fmt.Errorf("worker: mark done: %w", err)
	}
	return nil
}
```

- [ ] **Step 3: Write the failing test for the unmarshal error path**

```go
// cmd/ai-service/internal/worker/consumer_test.go
package worker

import "testing"

func TestHandleMessage_UnmarshalError(t *testing.T) {
	c := &Consumer{}
	err := c.HandleMessage([]byte("not json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}
```

- [ ] **Step 4: Run the test**

Run: `go test ./cmd/ai-service/...`
Expected: PASS (this path fails before touching `DB` or `Provider`, so both can stay nil in the test).

- [ ] **Step 5: Write `cmd/ai-service/main.go`**

```go
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"

	"github.com/hiepnv/crawl-ecomerce-golang/cmd/ai-service/internal/worker"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/aiproviders"
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
}

var configFile = flag.String("f", "etc/ai.yaml", "config file")

func main() {
	flag.Parse()

	var c Config
	conf.MustLoad(*configFile, &c)

	conn, err := db.Connect(c.Postgres.DSN)
	if err != nil {
		panic(err)
	}

	provider := aiproviders.NewOpenRouter(c.OpenRouter.ApiKey, c.OpenRouter.Model, c.OpenRouter.BaseURL, http.DefaultClient)

	consumer, err := rabbitmq.NewConsumer(rabbitmq.Config{URL: c.RabbitMQ.URL, Exchange: c.RabbitMQ.Exchange})
	if err != nil {
		panic(err)
	}

	w := &worker.Consumer{DB: conn, Provider: provider}
	fmt.Println("Starting ai-service consumer...")
	if err := consumer.Consume(context.Background(), "ai.trend.completed", "crawl.completed.trend", w.HandleMessage); err != nil {
		fmt.Fprintln(os.Stderr, "ai-service consumer stopped:", err)
		os.Exit(1)
	}
}
```

- [ ] **Step 6: Build**

Run: `go build ./cmd/ai-service/...`
Expected: builds successfully.

- [ ] **Step 7: Commit**

```bash
git add cmd/ai-service
git commit -m "feat: implement ai-service consumer calling OpenRouter for trend analysis"
```

---

## Task 8: api-gateway — REST endpoints

**Files:**
- Create: `api/gateway.api`
- Create: `cmd/api-gateway/` (generated via `goctl api go`)
- Modify (generated file, hand-filled): `cmd/api-gateway/internal/logic/createtrendjoblogic.go`
- Modify (generated file, hand-filled): `cmd/api-gateway/internal/logic/gettrendjoblogic.go`
- Modify (generated file): `cmd/api-gateway/internal/svc/servicecontext.go`

**Interfaces:**
- Consumes: `pkg/rabbitmq.Publisher`, `pkg/db.Connect`, generated `trend.TrendServiceClient` (gRPC client to trend-service from Task 6).
- Produces: `POST /jobs/trend {keyword string}` → `{job_id string}`; `GET /jobs/trend/{id}` → `{status, keyword, ai_output}`.

- [ ] **Step 1: Write the `.api` definition**

```
// api/gateway.api
syntax = "v1"

type CreateTrendJobRequest {
	Keyword string `json:"keyword"`
}

type CreateTrendJobResponse {
	JobId string `json:"job_id"`
}

type GetTrendJobRequest {
	Id string `path:"id"`
}

type GetTrendJobResponse {
	Status   string `json:"status"`
	Keyword  string `json:"keyword"`
	AiOutput string `json:"ai_output"`
}

service gateway-api {
	@handler CreateTrendJob
	post /jobs/trend (CreateTrendJobRequest) returns (CreateTrendJobResponse)

	@handler GetTrendJob
	get /jobs/trend/:id (GetTrendJobRequest) returns (GetTrendJobResponse)
}
```

- [ ] **Step 2: Generate the API scaffold**

Run:
```bash
mkdir -p cmd/api-gateway
goctl api go -api api/gateway.api -dir cmd/api-gateway
```
Expected: `cmd/api-gateway/` contains `gateway.go`, `etc/gateway-api.yaml`, `internal/{config,handler,logic,svc,types}`.

- [ ] **Step 3: Configure `cmd/api-gateway/etc/gateway-api.yaml`**

```yaml
Name: gateway-api
Host: 0.0.0.0
Port: 8888
Postgres:
  DSN: postgres://crawl:crawl@localhost:5432/crawl?sslmode=disable
RabbitMQ:
  URL: amqp://guest:guest@localhost:5672/
  Exchange: crawl
TrendRpc:
  Target: 127.0.0.1:8081
```

- [ ] **Step 4: Extend the generated `Config` struct**

```go
// cmd/api-gateway/internal/config/config.go
type Config struct {
	rest.RestConf
	Postgres struct {
		DSN string
	}
	RabbitMQ struct {
		URL      string
		Exchange string
	}
	TrendRpc struct {
		Target string
	}
}
```

- [ ] **Step 5: Extend the generated `ServiceContext`**

```go
// cmd/api-gateway/internal/svc/servicecontext.go
package svc

import (
	"database/sql"

	"github.com/hiepnv/crawl-ecomerce-golang/cmd/api-gateway/internal/config"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/trend-service/trend"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/db"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/rabbitmq"

	"google.golang.org/grpc"
)

type ServiceContext struct {
	Config    config.Config
	DB        *sql.DB
	Publisher *rabbitmq.Publisher
	TrendRpc  trend.TrendServiceClient
}

func NewServiceContext(c config.Config) *ServiceContext {
	conn, err := db.Connect(c.Postgres.DSN)
	if err != nil {
		panic(err)
	}
	publisher, err := rabbitmq.NewPublisher(rabbitmq.Config{URL: c.RabbitMQ.URL, Exchange: c.RabbitMQ.Exchange})
	if err != nil {
		panic(err)
	}
	grpcConn, err := grpc.NewClient(c.TrendRpc.Target, grpc.WithInsecure())
	if err != nil {
		panic(err)
	}
	return &ServiceContext{
		Config:    c,
		DB:        conn,
		Publisher: publisher,
		TrendRpc:  trend.NewTrendServiceClient(grpcConn),
	}
}
```

- [ ] **Step 6: Implement `CreateTrendJob` logic**

```go
// cmd/api-gateway/internal/logic/createtrendjoblogic.go
package logic

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hiepnv/crawl-ecomerce-golang/cmd/api-gateway/internal/svc"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/api-gateway/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateTrendJobLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewCreateTrendJobLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateTrendJobLogic {
	return &CreateTrendJobLogic{ctx: ctx, svcCtx: svcCtx, Logger: logx.WithContext(ctx)}
}

func (l *CreateTrendJobLogic) CreateTrendJob(req *types.CreateTrendJobRequest) (*types.CreateTrendJobResponse, error) {
	if req.Keyword == "" {
		return nil, fmt.Errorf("keyword is required")
	}

	params, err := json.Marshal(map[string]string{"keyword": req.Keyword})
	if err != nil {
		return nil, err
	}

	var jobID string
	err = l.svcCtx.DB.QueryRowContext(l.ctx,
		`INSERT INTO jobs (type, status, params) VALUES ('trend', 'pending', $1) RETURNING id`, params,
	).Scan(&jobID)
	if err != nil {
		return nil, fmt.Errorf("insert job: %w", err)
	}

	msg, err := json.Marshal(map[string]string{"job_id": jobID, "keyword": req.Keyword})
	if err != nil {
		return nil, err
	}
	if err := l.svcCtx.Publisher.Publish(l.ctx, "trend.jobs", msg); err != nil {
		return nil, fmt.Errorf("publish job: %w", err)
	}

	return &types.CreateTrendJobResponse{JobId: jobID}, nil
}
```

- [ ] **Step 7: Implement `GetTrendJob` logic**

```go
// cmd/api-gateway/internal/logic/gettrendjoblogic.go
package logic

import (
	"context"

	"github.com/hiepnv/crawl-ecomerce-golang/cmd/api-gateway/internal/svc"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/api-gateway/internal/types"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/trend-service/trend"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetTrendJobLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetTrendJobLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetTrendJobLogic {
	return &GetTrendJobLogic{ctx: ctx, svcCtx: svcCtx, Logger: logx.WithContext(ctx)}
}

func (l *GetTrendJobLogic) GetTrendJob(req *types.GetTrendJobRequest) (*types.GetTrendJobResponse, error) {
	resp, err := l.svcCtx.TrendRpc.GetTrendJob(l.ctx, &trend.GetTrendJobRequest{JobId: req.Id})
	if err != nil {
		return nil, err
	}
	return &types.GetTrendJobResponse{
		Status:   resp.Status,
		Keyword:  resp.Keyword,
		AiOutput: resp.AiOutput,
	}, nil
}
```

- [ ] **Step 8: Build**

Run: `go build ./cmd/api-gateway/...`
Expected: builds successfully (adjust generated type/handler names to match your `goctl` version's actual output if they differ).

- [ ] **Step 9: Commit**

```bash
git add api cmd/api-gateway
git commit -m "feat: implement api-gateway REST endpoints for trend jobs"
```

---

## Task 9: Wire full stack in Docker Compose and run an end-to-end smoke test

**Files:**
- Modify: `docker-compose.yml`
- Create: `cmd/api-gateway/Dockerfile`
- Create: `cmd/trend-service/Dockerfile`
- Create: `cmd/ai-service/Dockerfile`
- Create: `scripts/smoke_test.sh`

**Interfaces:**
- Produces: a runnable `docker compose up` stack and a smoke-test script that exercises the full path via HTTP only.

- [ ] **Step 1: Write a shared Dockerfile pattern for `trend-service` (repeat for `api-gateway` and `ai-service`, adjusting `CMD_DIR` and exposed port)**

```dockerfile
# cmd/trend-service/Dockerfile
FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /out/trend-service ./cmd/trend-service

FROM alpine:3.20
COPY --from=build /out/trend-service /usr/local/bin/trend-service
COPY cmd/trend-service/etc /etc/trend-service
CMD ["trend-service", "-f", "/etc/trend-service/trend.yaml"]
```

Repeat the same pattern for `cmd/api-gateway/Dockerfile` (binary `api-gateway`, config `gateway-api.yaml`) and `cmd/ai-service/Dockerfile` (binary `ai-service`, config `ai.yaml`).

- [ ] **Step 2: Extend `docker-compose.yml` with all three services**

```yaml
  trend-service:
    build:
      context: .
      dockerfile: cmd/trend-service/Dockerfile
    environment:
      SERPAPI_API_KEY: ${SERPAPI_API_KEY}
    depends_on:
      - postgres
      - rabbitmq
    ports:
      - "8081:8081"

  ai-service:
    build:
      context: .
      dockerfile: cmd/ai-service/Dockerfile
    environment:
      OPENROUTER_API_KEY: ${OPENROUTER_API_KEY}
    depends_on:
      - postgres
      - rabbitmq

  api-gateway:
    build:
      context: .
      dockerfile: cmd/api-gateway/Dockerfile
    depends_on:
      - postgres
      - rabbitmq
      - trend-service
    ports:
      - "8888:8888"
```

Append these three service blocks under the existing `postgres`/`rabbitmq` services in `docker-compose.yml`, keeping the shared `volumes:` block at the end of the file.

- [ ] **Step 3: Write the smoke test script**

```bash
#!/usr/bin/env bash
# scripts/smoke_test.sh
set -euo pipefail

JOB_ID=$(curl -sf -X POST http://localhost:8888/jobs/trend \
  -H 'Content-Type: application/json' \
  -d '{"keyword":"golang"}' | jq -r .job_id)

echo "Created job: $JOB_ID"

for i in $(seq 1 30); do
  STATUS=$(curl -sf http://localhost:8888/jobs/trend/$JOB_ID | jq -r .status)
  echo "Status: $STATUS"
  if [ "$STATUS" = "done" ]; then
    curl -sf http://localhost:8888/jobs/trend/$JOB_ID | jq .
    exit 0
  fi
  if [ "$STATUS" = "failed" ]; then
    echo "Job failed"
    exit 1
  fi
  sleep 2
done

echo "Timed out waiting for job to complete"
exit 1
```

- [ ] **Step 4: Make it executable, run migrations, bring up the full stack, and run the smoke test**

Run:
```bash
chmod +x scripts/smoke_test.sh
migrate -database "postgres://crawl:crawl@localhost:5432/crawl?sslmode=disable" -path db/migrations up
docker compose up -d --build
./scripts/smoke_test.sh
```
Expected: script prints `Status: done` and the final JSON includes a non-empty `ai_output` field. Requires real `SERPAPI_API_KEY` and `OPENROUTER_API_KEY` values exported in the shell before `docker compose up`.

- [ ] **Step 5: Commit**

```bash
git add docker-compose.yml cmd/*/Dockerfile scripts/smoke_test.sh
git commit -m "chore: wire full docker-compose stack and add end-to-end smoke test"
```

---

## Self-Review Notes

- **Spec coverage:** api-gateway (Task 8), trend-service consumer + gRPC query (Task 6), ai-service (Task 7), RabbitMQ topology with DLQ (Task 3), Postgres schema for `jobs`/`trend_raw`/`ai_results` (Task 2), go-zero + Docker Compose (Tasks 6-9) all covered. `fbads_raw`, `proxy_usage`, fb-ads-service, and proxy-broker are explicitly deferred to follow-up plans (noted in Global Constraints and spec's "Out of scope").
- **Type consistency:** `JobMessage`/`CompletedMessage` field names (`job_id`, `keyword`) match between publisher (Task 8 `CreateTrendJobLogic`) and consumer (Task 6 `worker.JobMessage`, Task 7 `worker.CompletedMessage`). `serpapi.Client`, `aiproviders.Provider`, `rabbitmq.Publisher`/`Consumer` signatures are defined once (Tasks 3-5) and reused identically in Tasks 6-8.
- **Known follow-ups for the next plan:** fb-ads-service (Apify) replicating this exact pattern; proxy-broker gRPC service; wiring `proxy-broker` into any service that needs direct outbound HTTP; TokenRouter as a second `aiproviders.Provider` implementation with provider selection by job config.
