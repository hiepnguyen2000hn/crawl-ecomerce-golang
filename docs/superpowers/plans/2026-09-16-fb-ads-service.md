# fb-ads-service Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a second crawl path — Facebook Ads Library via Apify — mirroring the existing trend-service architecture end-to-end (api-gateway → RabbitMQ → fb-ads-service → Postgres → ai-service → OpenRouter), and extend ai-service to branch its prompt-building by job type.

**Architecture:** `fb-ads-service` is a new go-zero gRPC service + RabbitMQ consumer, structurally identical to `trend-service` (Task 6 of the prior plan) but calling the Apify actor `igolaizola/facebook-ad-library-scraper` instead of SerpApi. `api-gateway` gains `POST /jobs/fbads` / `GET /jobs/fbads/{id}`. `ai-service`'s existing worker is modified to look up `jobs.type` and branch: `trend` reads `trend_raw` (unchanged), `fbads` reads `fbads_raw` and concatenates ad bodies/titles into the analysis prompt.

**Tech Stack:** Go 1.25 (matches this repo's current `go.mod` toolchain), go-zero + `goctl` (already installed and on PATH from the prior plan), `pkg/rabbitmq`/`pkg/db`/`pkg/aiproviders` (already built, reused as-is), PostgreSQL via `golang-migrate`, Docker Compose.

**Spec:** `docs/superpowers/specs/2026-09-16-fb-ads-service-design.md` (extends `docs/superpowers/specs/2026-09-16-crawl-ecommerce-platform-design.md`)

## Global Constraints

- Module path: `github.com/hiepnv/crawl-ecomerce-golang` (existing repo, continue in this worktree/branch — do not start a fresh module).
- Go version: this repo's `go.mod` already specifies `go 1.25.0` — Dockerfiles must use `golang:1.25-alpine` (established in the prior plan's final-review fix, not the originally-planned 1.22).
- RabbitMQ exchange: `crawl` (topic, already declared). New routing keys this plan adds: `fbads.jobs` (job intake), `crawl.completed.fbads` (completion event).
- Apify actor ID for REST calls: `igolaizola~facebook-ad-library-scraper` (tilde-separated, per Apify's URL convention), called via `POST https://api.apify.com/v2/acts/{actorId}/run-sync-get-dataset-items?token=<APIFY_API_TOKEN>`. Make the actor ID configurable (not hardcoded past the default) so it can be swapped later.
- Confirmed Apify actor response shape (verified via a live API spike, not guessed): each dataset item has top-level `ad_archive_id`, `page_id`, `page_name`, `is_active` (bool), `start_date`/`end_date` (**int64 unix seconds**), and a `snapshot` object containing `body` (**an object `{"text": "..."}"`, NOT a plain string**), `title` (string), `cta_text` (string), `cta_type` (string), `link_url` (string), plus `images`/`videos`/`cards` (arrays) and other fields not individually modeled — captured via the raw JSONB column instead.
- Every service that talks to an external API must go through a small interface so tests can substitute an `httptest.Server` — never call the real API in a unit test (same rule as the prior plan; `pkg/apify` follows the same pattern as `pkg/serpapi`).
- Config values come from go-zero `.yaml` config files loaded at startup with **`conf.MustLoad(*configFile, &c, conf.UseEnv())`** (not bare `conf.MustLoad` — that was a Critical bug fixed in the prior plan's final review; do not repeat it) plus a startup guard that fails fast if the relevant API key is empty or still contains the literal substring `${`.
- Two config variants per service, same pattern as trend-service/ai-service/api-gateway: a host-oriented `.yaml` (uses `localhost:5433`/`localhost:5673` — this dev environment's remapped host ports) for local `go run` development, and a `.docker.yaml` (uses docker-compose service names `postgres:5432`/`rabbitmq:5672`) for the containerized deployment. Never mix the two.
- `jobs.type` CHECK constraint already allows `'fbads'` (it was defined as `CHECK (type IN ('fbads', 'trend'))` from the very first migration in the prior plan) — no migration needed for that column, only a new `fbads_raw` table.

---

## File Structure

```
db/migrations/
  000002_fbads.up.sql
  000002_fbads.down.sql
pkg/apify/
  client.go
  client_test.go
rpc/fbads/
  fbads.proto
cmd/fb-ads-service/
  etc/fbads.yaml
  etc/fbads.docker.yaml
  Dockerfile
  main.go
  internal/{config,logic,server,svc,worker}/...
  fbads/ (generated pb.go)
cmd/api-gateway/
  (modify) api/gateway.api, internal/config/config.go, internal/svc/servicecontext.go
  (add) internal/logic/createfbadsjoblogic.go, internal/logic/getfbadsjoblogic.go
cmd/ai-service/
  (modify) internal/worker/consumer.go, internal/worker/consumer_test.go
docker-compose.yml (modify)
.env.example (modify)
scripts/smoke_test.sh (modify)
```

---

## Task 1: Database migration — `fbads_raw` table

**Files:**
- Create: `db/migrations/000002_fbads.up.sql`
- Create: `db/migrations/000002_fbads.down.sql`

**Interfaces:**
- Produces: table `fbads_raw` that Task 3 (fb-ads-service worker) writes to and Task 4 (ai-service) reads from.

- [ ] **Step 1: Write the up migration**

```sql
-- db/migrations/000002_fbads.up.sql
CREATE TABLE fbads_raw (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id UUID NOT NULL REFERENCES jobs(id),
    ad_archive_id TEXT NOT NULL,
    page_id TEXT,
    page_name TEXT,
    is_active BOOLEAN,
    start_date TIMESTAMPTZ,
    end_date TIMESTAMPTZ,
    body TEXT,
    title TEXT,
    cta_text TEXT,
    cta_type TEXT,
    link_url TEXT,
    raw JSONB NOT NULL,
    fetched_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

- [ ] **Step 2: Write the down migration**

```sql
-- db/migrations/000002_fbads.down.sql
DROP TABLE IF EXISTS fbads_raw;
```

- [ ] **Step 3: Apply the migration against the running dev Postgres container (host port 5433 in this environment)**

Run:
```bash
migrate -database "postgres://crawl:crawl@localhost:5433/crawl?sslmode=disable" -path db/migrations up
```
Expected: output ends with `2/u fbads (X.XXXms)` and no error.

- [ ] **Step 4: Commit**

```bash
git add db/migrations/000002_fbads.up.sql db/migrations/000002_fbads.down.sql
git commit -m "feat: add fbads_raw table migration"
```

---

## Task 2: `pkg/apify` client

**Files:**
- Create: `pkg/apify/client.go`
- Test: `pkg/apify/client_test.go`

**Interfaces:**
- Produces:
  - `apify.Ad{AdArchiveID, PageID, PageName string; IsActive bool; StartDate, EndDate time.Time; Body, Title, CtaText, CtaType, LinkURL string; Raw json.RawMessage}`
  - `apify.AdParams{Query, PageID, Country, Category, MediaType, SortBy, ActiveStatus, MinDate, MaxDate string; MaxItems int; Advertisers []string; FetchDetails bool}`
  - `apify.AdsResult{Ads []Ad}`
  - `apify.Client` interface: `FetchAds(ctx context.Context, params AdParams) (AdsResult, error)`
  - `apify.NewHTTPClient(apiToken, actorID, baseURL string, httpClient *http.Client) *HTTPClient` — `baseURL` defaults to `"https://api.apify.com/v2"` in production config, overridden in tests; `actorID` defaults to `"igolaizola~facebook-ad-library-scraper"`.

- [ ] **Step 1: Write the failing test**

```go
// pkg/apify/client_test.go
package apify

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPClient_FetchAds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("token") != "test-token" {
			t.Errorf("query param token = %q, want %q", r.URL.Query().Get("token"), "test-token")
		}
		if r.URL.Path != "/acts/igolaizola~facebook-ad-library-scraper/run-sync-get-dataset-items" {
			t.Errorf("path = %q, unexpected", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[
			{
				"ad_archive_id": "123",
				"page_id": "456",
				"page_name": "Acme",
				"is_active": true,
				"start_date": 1700000000,
				"end_date": 1701000000,
				"snapshot": {
					"body": {"text": "Buy now"},
					"title": "Acme Sale",
					"cta_text": "Shop Now",
					"cta_type": "SHOP_NOW",
					"link_url": "https://acme.example/sale"
				}
			}
		]`))
	}))
	defer srv.Close()

	c := NewHTTPClient("test-token", "igolaizola~facebook-ad-library-scraper", srv.URL, srv.Client())
	result, err := c.FetchAds(context.Background(), AdParams{Query: "acme", Country: "US", MaxItems: 5})
	if err != nil {
		t.Fatalf("FetchAds() error = %v", err)
	}
	if len(result.Ads) != 1 {
		t.Fatalf("len(Ads) = %d, want 1", len(result.Ads))
	}
	ad := result.Ads[0]
	if ad.AdArchiveID != "123" || ad.PageName != "Acme" || !ad.IsActive {
		t.Errorf("ad top-level fields wrong: %+v", ad)
	}
	if ad.Body != "Buy now" || ad.Title != "Acme Sale" || ad.CtaText != "Shop Now" || ad.CtaType != "SHOP_NOW" || ad.LinkURL != "https://acme.example/sale" {
		t.Errorf("ad snapshot fields wrong: %+v", ad)
	}
	wantStart := time.Unix(1700000000, 0).UTC()
	if !ad.StartDate.Equal(wantStart) {
		t.Errorf("StartDate = %v, want %v", ad.StartDate, wantStart)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./pkg/apify/...`
Expected: FAIL — `undefined: NewHTTPClient`

- [ ] **Step 3: Write `pkg/apify/client.go`**

```go
package apify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Ad is a normalized Facebook Ads Library record.
type Ad struct {
	AdArchiveID string
	PageID      string
	PageName    string
	IsActive    bool
	StartDate   time.Time
	EndDate     time.Time
	Body        string
	Title       string
	CtaText     string
	CtaType     string
	LinkURL     string
	Raw         json.RawMessage
}

// AdParams mirrors the Apify actor's input schema. All fields are
// optional except MaxItems, which the client always sends (defaulting
// to 10 when unset) since the actor requires it.
type AdParams struct {
	Query        string
	PageID       string
	Country      string
	Category     string
	MediaType    string
	SortBy       string
	ActiveStatus string
	MinDate      string
	MaxDate      string
	MaxItems     int
	Advertisers  []string
	FetchDetails bool
}

type AdsResult struct {
	Ads []Ad
}

type Client interface {
	FetchAds(ctx context.Context, params AdParams) (AdsResult, error)
}

type HTTPClient struct {
	apiToken   string
	actorID    string
	baseURL    string
	httpClient *http.Client
}

func NewHTTPClient(apiToken, actorID, baseURL string, httpClient *http.Client) *HTTPClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	if actorID == "" {
		actorID = "igolaizola~facebook-ad-library-scraper"
	}
	return &HTTPClient{apiToken: apiToken, actorID: actorID, baseURL: baseURL, httpClient: httpClient}
}

type apifyRunInput struct {
	Query        string   `json:"query,omitempty"`
	PageID       string   `json:"pageId,omitempty"`
	Country      string   `json:"country,omitempty"`
	Category     string   `json:"category,omitempty"`
	MediaType    string   `json:"mediaType,omitempty"`
	SortBy       string   `json:"sortBy,omitempty"`
	ActiveStatus string   `json:"activeStatus,omitempty"`
	MinDate      string   `json:"minDate,omitempty"`
	MaxDate      string   `json:"maxDate,omitempty"`
	MaxItems     int      `json:"maxItems"`
	Advertisers  []string `json:"advertisers,omitempty"`
	FetchDetails bool     `json:"fetchDetails,omitempty"`
}

type apifySnapshot struct {
	Body struct {
		Text string `json:"text"`
	} `json:"body"`
	Title   string `json:"title"`
	CtaText string `json:"cta_text"`
	CtaType string `json:"cta_type"`
	LinkURL string `json:"link_url"`
}

type apifyAdItem struct {
	AdArchiveID string        `json:"ad_archive_id"`
	PageID      string        `json:"page_id"`
	PageName    string        `json:"page_name"`
	IsActive    bool          `json:"is_active"`
	StartDate   int64         `json:"start_date"`
	EndDate     int64         `json:"end_date"`
	Snapshot    apifySnapshot `json:"snapshot"`
}

func (c *HTTPClient) FetchAds(ctx context.Context, params AdParams) (AdsResult, error) {
	maxItems := params.MaxItems
	if maxItems <= 0 {
		maxItems = 10
	}
	input := apifyRunInput{
		Query:        params.Query,
		PageID:       params.PageID,
		Country:      params.Country,
		Category:     params.Category,
		MediaType:    params.MediaType,
		SortBy:       params.SortBy,
		ActiveStatus: params.ActiveStatus,
		MinDate:      params.MinDate,
		MaxDate:      params.MaxDate,
		MaxItems:     maxItems,
		Advertisers:  params.Advertisers,
		FetchDetails: params.FetchDetails,
	}
	payload, err := json.Marshal(input)
	if err != nil {
		return AdsResult{}, fmt.Errorf("apify: marshal input: %w", err)
	}

	q := url.Values{}
	q.Set("token", c.apiToken)
	reqURL := fmt.Sprintf("%s/acts/%s/run-sync-get-dataset-items?%s", c.baseURL, c.actorID, q.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(payload))
	if err != nil {
		return AdsResult{}, fmt.Errorf("apify: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return AdsResult{}, fmt.Errorf("apify: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return AdsResult{}, fmt.Errorf("apify: returned status %d", resp.StatusCode)
	}

	items, rawItems, err := decodeItems(resp.Body)
	if err != nil {
		return AdsResult{}, err
	}

	ads := make([]Ad, 0, len(items))
	for i, it := range items {
		ads = append(ads, Ad{
			AdArchiveID: it.AdArchiveID,
			PageID:      it.PageID,
			PageName:    it.PageName,
			IsActive:    it.IsActive,
			StartDate:   time.Unix(it.StartDate, 0).UTC(),
			EndDate:     time.Unix(it.EndDate, 0).UTC(),
			Body:        it.Snapshot.Body.Text,
			Title:       it.Snapshot.Title,
			CtaText:     it.Snapshot.CtaText,
			CtaType:     it.Snapshot.CtaType,
			LinkURL:     it.Snapshot.LinkURL,
			Raw:         rawItems[i],
		})
	}
	return AdsResult{Ads: ads}, nil
}

// decodeItems reads the full response body, keeping each dataset item's
// raw bytes (for the Ad.Raw JSONB column) alongside its typed decode.
func decodeItems(body io.Reader) ([]apifyAdItem, []json.RawMessage, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return nil, nil, fmt.Errorf("apify: read response body: %w", err)
	}
	var rawItems []json.RawMessage
	if err := json.Unmarshal(data, &rawItems); err != nil {
		return nil, nil, fmt.Errorf("apify: unmarshal raw items: %w", err)
	}
	items := make([]apifyAdItem, len(rawItems))
	for i, raw := range rawItems {
		if err := json.Unmarshal(raw, &items[i]); err != nil {
			return nil, nil, fmt.Errorf("apify: unmarshal item %d: %w", i, err)
		}
	}
	return items, rawItems, nil
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./pkg/apify/...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/apify
git commit -m "feat: add apify client for Facebook Ads Library"
```

---

## Task 3: fb-ads-service — gRPC scaffold, consumer worker, business logic

**Files:**
- Create: `rpc/fbads/fbads.proto`
- Create: `cmd/fb-ads-service/etc/fbads.yaml`
- Create: `cmd/fb-ads-service/etc/fbads.docker.yaml`
- Create: `cmd/fb-ads-service/` (generated via `goctl rpc protoc`)
- Modify (generated file, hand-filled): `cmd/fb-ads-service/internal/logic/getfbadsjoblogic.go`
- Create: `cmd/fb-ads-service/internal/worker/consumer.go`
- Test: `cmd/fb-ads-service/internal/worker/consumer_test.go`
- Create: `cmd/fb-ads-service/main.go`
- Create: `cmd/fb-ads-service/Dockerfile`

**Interfaces:**
- Consumes: `pkg/rabbitmq.Consumer`/`Publisher`, `pkg/apify.Client`, `pkg/db.Connect`
- Produces: gRPC method `GetFbAdsJob(job_id) returns (status, ad_count, ai_output)` for api-gateway to call in Task 5.
- Produces: consumer reading `fbads.jobs` queue, writing `fbads_raw` rows, updating `jobs.status`, publishing `crawl.completed.fbads`.

- [ ] **Step 1: Write the proto definition**

```protobuf
// rpc/fbads/fbads.proto
syntax = "proto3";

package fbads;
option go_package = "./fbads";

message GetFbAdsJobRequest {
  string job_id = 1;
}

message GetFbAdsJobResponse {
  string status = 1;
  int32 ad_count = 2;
  string ai_output = 3;
}

service FbAdsService {
  rpc GetFbAdsJob(GetFbAdsJobRequest) returns (GetFbAdsJobResponse);
}
```

- [ ] **Step 2: Generate the RPC scaffold**

Run:
```bash
mkdir -p cmd/fb-ads-service
cd rpc/fbads && goctl rpc protoc fbads.proto --go_out=../../cmd/fb-ads-service --go-grpc_out=../../cmd/fb-ads-service --zrpc_out=../../cmd/fb-ads-service && cd ../..
```
Expected: `cmd/fb-ads-service/` contains generated `main.go` (or `fbads.go` depending on goctl version — check and rename/adapt as Task 6 of the prior plan's implementer did), `etc/`, `internal/{config,logic,server,svc}`, `fbads/` (pb.go files).

If goctl's generated file/type names differ from what this task assumes below (same caveat as the prior plan's Task 6), adapt the hand-written code to match what was actually generated, preserving intent, and note the adaptation in your report.

- [ ] **Step 3: Write `cmd/fb-ads-service/etc/fbads.yaml` (host dev config)**

```yaml
Name: fb-ads-service
ListenOn: 0.0.0.0:8082
Postgres:
  DSN: postgres://crawl:crawl@localhost:5433/crawl?sslmode=disable
RabbitMQ:
  URL: amqp://guest:guest@localhost:5673/
  Exchange: crawl
Apify:
  ApiToken: ${APIFY_API_TOKEN}
  ActorID: igolaizola~facebook-ad-library-scraper
  BaseURL: https://api.apify.com/v2
```

- [ ] **Step 4: Write `cmd/fb-ads-service/etc/fbads.docker.yaml` (containerized config)**

Identical to Step 3 except:
```yaml
Postgres:
  DSN: postgres://crawl:crawl@postgres:5432/crawl?sslmode=disable
RabbitMQ:
  URL: amqp://guest:guest@rabbitmq:5672/
  Exchange: crawl
```
(`Apify` section and `Name`/`ListenOn` stay the same as Step 3.)

- [ ] **Step 5: Extend the generated `Config` struct**

Add to `cmd/fb-ads-service/internal/config/config.go`:

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
	Apify struct {
		ApiToken string
		ActorID  string
		BaseURL  string
	}
}
```

- [ ] **Step 6: Extend the generated `ServiceContext`**

```go
package svc

import (
	"database/sql"
	"net/http"

	"github.com/hiepnv/crawl-ecomerce-golang/cmd/fb-ads-service/internal/config"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/apify"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/db"
)

type ServiceContext struct {
	Config config.Config
	DB     *sql.DB
	Apify  apify.Client
}

func NewServiceContext(c config.Config) *ServiceContext {
	conn, err := db.Connect(c.Postgres.DSN)
	if err != nil {
		panic(err)
	}
	return &ServiceContext{
		Config: c,
		DB:     conn,
		Apify:  apify.NewHTTPClient(c.Apify.ApiToken, c.Apify.ActorID, c.Apify.BaseURL, http.DefaultClient),
	}
}
```

- [ ] **Step 7: Implement `GetFbAdsJob` in `internal/logic/getfbadsjoblogic.go`**

```go
package logic

import (
	"context"
	"database/sql"

	"github.com/hiepnv/crawl-ecomerce-golang/cmd/fb-ads-service/fbads"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/fb-ads-service/internal/svc"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetFbAdsJobLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetFbAdsJobLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetFbAdsJobLogic {
	return &GetFbAdsJobLogic{ctx: ctx, svcCtx: svcCtx, Logger: logx.WithContext(ctx)}
}

func (l *GetFbAdsJobLogic) GetFbAdsJob(in *fbads.GetFbAdsJobRequest) (*fbads.GetFbAdsJobResponse, error) {
	var status string
	err := l.svcCtx.DB.QueryRowContext(l.ctx, `SELECT status FROM jobs WHERE id = $1`, in.JobId).Scan(&status)
	if err == sql.ErrNoRows {
		return &fbads.GetFbAdsJobResponse{Status: "not_found"}, nil
	}
	if err != nil {
		return nil, err
	}

	var adCount int32
	if err := l.svcCtx.DB.QueryRowContext(l.ctx,
		`SELECT COUNT(*) FROM fbads_raw WHERE job_id = $1`, in.JobId,
	).Scan(&adCount); err != nil {
		return nil, err
	}

	resp := &fbads.GetFbAdsJobResponse{Status: status, AdCount: adCount}

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

- [ ] **Step 8: Write the consumer worker**

```go
// cmd/fb-ads-service/internal/worker/consumer.go
package worker

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/hiepnv/crawl-ecomerce-golang/pkg/apify"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/rabbitmq"
)

type JobMessage struct {
	JobID        string   `json:"job_id"`
	Query        string   `json:"query,omitempty"`
	PageID       string   `json:"page_id,omitempty"`
	Country      string   `json:"country,omitempty"`
	Category     string   `json:"category,omitempty"`
	MediaType    string   `json:"media_type,omitempty"`
	SortBy       string   `json:"sort_by,omitempty"`
	ActiveStatus string   `json:"active_status,omitempty"`
	MinDate      string   `json:"min_date,omitempty"`
	MaxDate      string   `json:"max_date,omitempty"`
	MaxItems     int      `json:"max_items,omitempty"`
	Advertisers  []string `json:"advertisers,omitempty"`
	FetchDetails bool     `json:"fetch_details,omitempty"`
}

type CompletedMessage struct {
	JobID string `json:"job_id"`
}

type Consumer struct {
	DB        *sql.DB
	Apify     apify.Client
	Publisher *rabbitmq.Publisher
}

func (c *Consumer) markFailed(ctx context.Context, jobID string) {
	if c.DB == nil {
		return
	}
	if _, err := c.DB.ExecContext(ctx,
		`UPDATE jobs SET status = 'failed', updated_at = now() WHERE id = $1`, jobID,
	); err != nil {
		fmt.Println("worker: markFailed error:", err)
	}
}

func (c *Consumer) HandleMessage(body []byte) error {
	var msg JobMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		return fmt.Errorf("worker: unmarshal job message: %w", err)
	}

	ctx := context.Background()

	result, err := c.Apify.FetchAds(ctx, apify.AdParams{
		Query:        msg.Query,
		PageID:       msg.PageID,
		Country:      msg.Country,
		Category:     msg.Category,
		MediaType:    msg.MediaType,
		SortBy:       msg.SortBy,
		ActiveStatus: msg.ActiveStatus,
		MinDate:      msg.MinDate,
		MaxDate:      msg.MaxDate,
		MaxItems:     msg.MaxItems,
		Advertisers:  msg.Advertisers,
		FetchDetails: msg.FetchDetails,
	})
	if err != nil {
		c.markFailed(ctx, msg.JobID)
		return fmt.Errorf("worker: fetch ads: %w", err)
	}

	for _, ad := range result.Ads {
		if _, err := c.DB.ExecContext(ctx,
			`INSERT INTO fbads_raw (job_id, ad_archive_id, page_id, page_name, is_active, start_date, end_date, body, title, cta_text, cta_type, link_url, raw)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
			msg.JobID, ad.AdArchiveID, ad.PageID, ad.PageName, ad.IsActive, ad.StartDate, ad.EndDate, ad.Body, ad.Title, ad.CtaText, ad.CtaType, ad.LinkURL, ad.Raw,
		); err != nil {
			c.markFailed(ctx, msg.JobID)
			return fmt.Errorf("worker: insert fbads_raw: %w", err)
		}
	}

	if _, err := c.DB.ExecContext(ctx,
		`UPDATE jobs SET status = 'crawled', updated_at = now() WHERE id = $1`, msg.JobID,
	); err != nil {
		c.markFailed(ctx, msg.JobID)
		return fmt.Errorf("worker: update job status: %w", err)
	}

	payload, err := json.Marshal(CompletedMessage{JobID: msg.JobID})
	if err != nil {
		return fmt.Errorf("worker: marshal completed message: %w", err)
	}
	if err := c.Publisher.Publish(ctx, "crawl.completed.fbads", payload); err != nil {
		return fmt.Errorf("worker: publish completed event: %w", err)
	}
	return nil
}
```

- [ ] **Step 9: Write the failing tests for the pre-DB failure paths**

```go
// cmd/fb-ads-service/internal/worker/consumer_test.go
package worker

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/hiepnv/crawl-ecomerce-golang/pkg/apify"
)

type fakeApify struct {
	called bool
	query  string
	err    error
}

func (f *fakeApify) FetchAds(ctx context.Context, params apify.AdParams) (apify.AdsResult, error) {
	f.called = true
	f.query = params.Query
	if f.err != nil {
		return apify.AdsResult{}, f.err
	}
	return apify.AdsResult{}, nil
}

func TestHandleMessage_UnmarshalError(t *testing.T) {
	c := &Consumer{Apify: &fakeApify{}}
	err := c.HandleMessage([]byte("not json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestHandleMessage_ApifyError(t *testing.T) {
	fake := &fakeApify{err: errors.New("boom")}
	c := &Consumer{Apify: fake}
	msg, _ := json.Marshal(JobMessage{JobID: "11111111-1111-1111-1111-111111111111", Query: "nike"})
	err := c.HandleMessage(msg)
	if err == nil {
		t.Fatal("expected error propagated from Apify, got nil")
	}
	if !fake.called || fake.query != "nike" {
		t.Errorf("expected Apify.FetchAds called with query=nike, got called=%v query=%q", fake.called, fake.query)
	}
}
```

Note: `TestHandleMessage_ApifyError` exercises `markFailed` with `c.DB == nil` (the nil-guard added in `markFailed`, matching the established pattern from the prior plan's final-review fix), so this test needs no real database.

- [ ] **Step 10: Run the tests to verify RED then GREEN**

Run: `go test ./cmd/fb-ads-service/...`
Before Step 8's file exists: FAIL — `undefined: Consumer`. After Step 8: PASS.

- [ ] **Step 11: Write `cmd/fb-ads-service/main.go`**

Follow the exact same structure as `cmd/trend-service/main.go` (Task 6 of the prior plan) — a `zrpc.MustNewServer` running `FbAdsServiceServer`, plus a goroutine running the RabbitMQ consumer on queue `fbads.jobs` bound to routing key `fbads.jobs`. Load config with:
```go
conf.MustLoad(*configFile, &c, conf.UseEnv())
if c.Apify.ApiToken == "" || strings.Contains(c.Apify.ApiToken, "${") {
	panic("fb-ads-service: APIFY_API_TOKEN is not set")
}
```
(mirrors the startup guard added to trend-service/ai-service in the prior plan's final-review fix — add `"strings"` to the imports.)

- [ ] **Step 12: Build**

Run: `go build ./cmd/fb-ads-service/...`
Expected: builds successfully.

- [ ] **Step 13: Write `cmd/fb-ads-service/Dockerfile`**

Same pattern as `cmd/trend-service/Dockerfile` (golang:1.25-alpine build stage, `CGO_ENABLED=0`, alpine:3.20 runtime), binary name `fb-ads-service`, CMD referencing `/etc/fb-ads-service/fbads.docker.yaml`.

- [ ] **Step 14: Commit**

```bash
git add rpc/fbads cmd/fb-ads-service
git commit -m "feat: implement fb-ads-service gRPC query and RabbitMQ consumer worker"
```

---

## Task 4: ai-service — branch prompt-building by job type

**Files:**
- Modify: `cmd/ai-service/internal/worker/consumer.go`
- Test: `cmd/ai-service/internal/worker/consumer_test.go` (add a case, keep existing)

**Interfaces:**
- Consumes: existing `pkg/db`, `pkg/aiproviders` wiring (unchanged) plus a new binding on routing key `crawl.completed.fbads` (wired in `cmd/ai-service/main.go`, Task 5 covers that config; the binding itself is set up wherever `main.go` currently calls `consumer.Consume(...)` — see Step 4 below).
- Produces: `HandleMessage` now looks up `jobs.type` before deciding which raw table to read.

- [ ] **Step 1: Read the current `HandleMessage` implementation**

Open `cmd/ai-service/internal/worker/consumer.go` and locate the existing logic that: unmarshals `CompletedMessage`, loads `trend_raw.trend_data`, marks `ai_processing`, builds a prompt, calls `Provider.Complete`, inserts `ai_results`, marks `done`, and calls `markFailed` on error paths (added in the prior plan's final-review fix).

- [ ] **Step 2: Replace the trend-only data-loading section with a job-type branch**

Immediately after the existing `json.Unmarshal(body, &msg)` step (before the current unconditional `trend_raw` query), insert:

```go
	var jobType string
	if err := c.DB.QueryRowContext(ctx, `SELECT type FROM jobs WHERE id = $1`, msg.JobID).Scan(&jobType); err != nil {
		return fmt.Errorf("worker: load job type: %w", err)
	}

	var prompt string
	switch jobType {
	case "trend":
		var trendData []byte
		if err := c.DB.QueryRowContext(ctx,
			`SELECT trend_data FROM trend_raw WHERE job_id = $1 ORDER BY fetched_at DESC LIMIT 1`, msg.JobID,
		).Scan(&trendData); err != nil {
			c.markFailed(ctx, msg.JobID)
			return fmt.Errorf("worker: load trend_raw: %w", err)
		}
		prompt = fmt.Sprintf("Analyze this Google Trends data and summarize the key insight in 2-3 sentences:\n%s", string(trendData))
	case "fbads":
		rows, err := c.DB.QueryContext(ctx,
			`SELECT title, body FROM fbads_raw WHERE job_id = $1 ORDER BY fetched_at ASC`, msg.JobID)
		if err != nil {
			c.markFailed(ctx, msg.JobID)
			return fmt.Errorf("worker: load fbads_raw: %w", err)
		}
		var sb strings.Builder
		count := 0
		for rows.Next() {
			var title, body string
			if err := rows.Scan(&title, &body); err != nil {
				rows.Close()
				c.markFailed(ctx, msg.JobID)
				return fmt.Errorf("worker: scan fbads_raw: %w", err)
			}
			fmt.Fprintf(&sb, "Ad %d — Title: %s\nBody: %s\n\n", count+1, title, body)
			count++
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			c.markFailed(ctx, msg.JobID)
			return fmt.Errorf("worker: iterate fbads_raw: %w", err)
		}
		prompt = fmt.Sprintf("Analyze these %d Facebook ad creatives and summarize the common patterns, messaging themes, and calls to action in 3-4 sentences:\n%s", count, sb.String())
	default:
		c.markFailed(ctx, msg.JobID)
		return fmt.Errorf("worker: unknown job type %q", jobType)
	}
```

Then delete the old unconditional `trend_raw` query block that this replaces (it's now inside the `case "trend":` branch above).

The rest of the function (mark `ai_processing` — note this should now happen *before* the branch above, right after the job-type lookup, not after; move the existing `UPDATE jobs SET status = 'ai_processing'` call to immediately follow the `jobType` lookup and before the `switch` — call `Provider.Complete(ctx, prompt)`, insert `ai_results`, mark `done`) stays structurally the same, just using the `prompt` variable built above instead of one built inline from `trendData`.

- [ ] **Step 3: Add `"strings"` to the imports** of `cmd/ai-service/internal/worker/consumer.go` (used by `strings.Builder` above).

- [ ] **Step 4: Add a unit test for the fbads prompt-building path's failure mode (pre-DB, consistent with existing test style)**

The existing `TestHandleMessage_UnmarshalError` test needs no change (it fails before any DB call). Add:

```go
func TestHandleMessage_JobTypeLookupError(t *testing.T) {
	// This test documents that a job-type lookup failure (e.g. unknown
	// job ID) propagates as an error without reaching Provider.Complete.
	// A real DB is required to exercise this path meaningfully; this
	// repo's established pattern (see trend-service/api-gateway tests)
	// only unit-tests pre-DB/pre-network failure paths, so this case is
	// covered instead by cmd/ai-service's manual verification against
	// the live dev database in Task 6's smoke test, not here.
	t.Skip("covered by Task 6 smoke test against a live database")
}
```

- [ ] **Step 5: Build and test**

Run: `go build ./cmd/ai-service/...` then `go test ./cmd/ai-service/...`
Expected: both succeed; existing `TestHandleMessage_UnmarshalError` still passes, new `TestHandleMessage_JobTypeLookupError` reports SKIP (not FAIL).

- [ ] **Step 6: Extend `cmd/ai-service/main.go` to also bind `crawl.completed.fbads`**

The current `main.go` calls `consumer.Consume(ctx, "ai.trend.completed", "crawl.completed.trend", w.HandleMessage)` in a single blocking call. Since `pkg/rabbitmq.Consumer.Consume` only binds one routing key per call, start a second consumer+goroutine for the fbads binding:

```go
	fbadsConsumer, err := rabbitmq.NewConsumer(rabbitmq.Config{URL: c.RabbitMQ.URL, Exchange: c.RabbitMQ.Exchange})
	if err != nil {
		panic(err)
	}
	go func() {
		if err := fbadsConsumer.Consume(context.Background(), "ai.fbads.completed", "crawl.completed.fbads", w.HandleMessage); err != nil {
			fmt.Fprintln(os.Stderr, "ai-service fbads consumer stopped:", err)
		}
	}()
```
Place this before the existing trend consumer's blocking `Consume` call (which must remain the last statement in `main`, since it blocks — or convert both to goroutines with a shared `select {}` / wait mechanism if the existing trend consumer isn't already the final blocking call; check the actual current structure and adapt so both consumers run concurrently and the process doesn't exit early).

- [ ] **Step 7: Build**

Run: `go build ./cmd/ai-service/...`
Expected: succeeds.

- [ ] **Step 8: Commit**

```bash
git add cmd/ai-service
git commit -m "feat: extend ai-service to analyze fbads jobs alongside trend jobs"
```

---

## Task 5: api-gateway — REST endpoints for fbads jobs

**Files:**
- Modify: `api/gateway.api`
- Modify: `cmd/api-gateway/internal/config/config.go`
- Modify: `cmd/api-gateway/internal/svc/servicecontext.go`
- Create: `cmd/api-gateway/internal/logic/createfbadsjoblogic.go`
- Create: `cmd/api-gateway/internal/logic/getfbadsjoblogic.go`
- Modify: `cmd/api-gateway/etc/gateway-api.yaml`, `cmd/api-gateway/etc/gateway-api.docker.yaml`

**Interfaces:**
- Consumes: `pkg/rabbitmq.Publisher` (existing), `pkg/db` (existing), new gRPC client `fbads.FbAdsServiceClient` (from Task 3) dialed at `FbAdsRpc.Target`.
- Produces: `POST /jobs/fbads {query?, page_id?, country?, category?, media_type?, sort_by?, active_status?, min_date?, max_date?, max_items?, advertisers?, fetch_details?}` → `{job_id}`; `GET /jobs/fbads/{id}` → `{status, ad_count, ai_output}`.

- [ ] **Step 1: Extend `api/gateway.api`**

Add alongside the existing `CreateTrendJob`/`GetTrendJob` types and routes:

```
type CreateFbAdsJobRequest {
	Query        string   `json:"query,optional"`
	PageId       string   `json:"page_id,optional"`
	Country      string   `json:"country,optional"`
	Category     string   `json:"category,optional"`
	MediaType    string   `json:"media_type,optional"`
	SortBy       string   `json:"sort_by,optional"`
	ActiveStatus string   `json:"active_status,optional"`
	MinDate      string   `json:"min_date,optional"`
	MaxDate      string   `json:"max_date,optional"`
	MaxItems     int      `json:"max_items,optional"`
	Advertisers  []string `json:"advertisers,optional"`
	FetchDetails bool     `json:"fetch_details,optional"`
}

type CreateFbAdsJobResponse {
	JobId string `json:"job_id"`
}

type GetFbAdsJobRequest {
	Id string `path:"id"`
}

type GetFbAdsJobResponse {
	Status   string `json:"status"`
	AdCount  int32  `json:"ad_count"`
	AiOutput string `json:"ai_output"`
}
```

And add to the `service gateway-api` block:
```
	@handler CreateFbAdsJob
	post /jobs/fbads (CreateFbAdsJobRequest) returns (CreateFbAdsJobResponse)

	@handler GetFbAdsJob
	get /jobs/fbads/:id (GetFbAdsJobRequest) returns (GetFbAdsJobResponse)
```

- [ ] **Step 2: Regenerate handlers/types/routes**

Run:
```bash
goctl api go -api api/gateway.api -dir cmd/api-gateway
```
This regenerates `internal/handler/routes.go` and `internal/types/types.go` to include the new request/response types and routes, and creates stub logic files for `CreateFbAdsJob`/`GetFbAdsJob` if they don't already exist (goctl does not overwrite existing hand-written logic files — verify `createtrendjoblogic.go`/`gettrendjoblogic.go` are untouched after this run).

- [ ] **Step 3: Extend `Config`**

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
	FbAdsRpc struct {
		Target string
	}
}
```

- [ ] **Step 4: Extend `ServiceContext`**

```go
// cmd/api-gateway/internal/svc/servicecontext.go
// add alongside the existing TrendRpc wiring:
	fbAdsGrpcConn, err := grpc.NewClient(c.FbAdsRpc.Target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		panic(err)
	}
	// ... and add to the returned &ServiceContext{...}:
	//   FbAdsRpc: fbads.NewFbAdsServiceClient(fbAdsGrpcConn),
```
Add the `FbAdsRpc fbads.FbAdsServiceClient` field to the `ServiceContext` struct and the `github.com/hiepnv/crawl-ecomerce-golang/cmd/fb-ads-service/fbads` import. Reuse the existing `grpc.WithTransportCredentials(insecure.NewCredentials())` pattern already used for `TrendRpc` (the prior plan's final review flagged and — check whether it was fixed to non-deprecated form; if `TrendRpc`'s dial still uses `grpc.WithInsecure()`, use the same call here for consistency rather than mixing styles, and note this as a candidate for a follow-up cleanup rather than fixing both in this task).

- [ ] **Step 5: Implement `CreateFbAdsJob` logic**

```go
// cmd/api-gateway/internal/logic/createfbadsjoblogic.go
package logic

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hiepnv/crawl-ecomerce-golang/cmd/api-gateway/internal/svc"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/api-gateway/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateFbAdsJobLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewCreateFbAdsJobLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateFbAdsJobLogic {
	return &CreateFbAdsJobLogic{ctx: ctx, svcCtx: svcCtx, Logger: logx.WithContext(ctx)}
}

func (l *CreateFbAdsJobLogic) CreateFbAdsJob(req *types.CreateFbAdsJobRequest) (*types.CreateFbAdsJobResponse, error) {
	if req.Query == "" && req.PageId == "" {
		return nil, fmt.Errorf("either query or page_id is required")
	}

	params, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	var jobID string
	err = l.svcCtx.DB.QueryRowContext(l.ctx,
		`INSERT INTO jobs (type, status, params) VALUES ('fbads', 'pending', $1) RETURNING id`, params,
	).Scan(&jobID)
	if err != nil {
		return nil, fmt.Errorf("insert job: %w", err)
	}

	msg, err := json.Marshal(map[string]interface{}{
		"job_id":        jobID,
		"query":         req.Query,
		"page_id":       req.PageId,
		"country":       req.Country,
		"category":      req.Category,
		"media_type":    req.MediaType,
		"sort_by":       req.SortBy,
		"active_status": req.ActiveStatus,
		"min_date":      req.MinDate,
		"max_date":      req.MaxDate,
		"max_items":     req.MaxItems,
		"advertisers":   req.Advertisers,
		"fetch_details": req.FetchDetails,
	})
	if err != nil {
		return nil, err
	}
	if err := l.svcCtx.Publisher.Publish(l.ctx, "fbads.jobs", msg); err != nil {
		return nil, fmt.Errorf("publish job: %w", err)
	}

	return &types.CreateFbAdsJobResponse{JobId: jobID}, nil
}
```

- [ ] **Step 6: Implement `GetFbAdsJob` logic**

```go
// cmd/api-gateway/internal/logic/getfbadsjoblogic.go
package logic

import (
	"context"

	"github.com/hiepnv/crawl-ecomerce-golang/cmd/api-gateway/internal/svc"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/api-gateway/internal/types"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/fb-ads-service/fbads"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetFbAdsJobLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetFbAdsJobLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetFbAdsJobLogic {
	return &GetFbAdsJobLogic{ctx: ctx, svcCtx: svcCtx, Logger: logx.WithContext(ctx)}
}

func (l *GetFbAdsJobLogic) GetFbAdsJob(req *types.GetFbAdsJobRequest) (*types.GetFbAdsJobResponse, error) {
	resp, err := l.svcCtx.FbAdsRpc.GetFbAdsJob(l.ctx, &fbads.GetFbAdsJobRequest{JobId: req.Id})
	if err != nil {
		return nil, err
	}
	return &types.GetFbAdsJobResponse{
		Status:   resp.Status,
		AdCount:  resp.AdCount,
		AiOutput: resp.AiOutput,
	}, nil
}
```

- [ ] **Step 7: Add `FbAdsRpc.Target` to both api-gateway config files**

`cmd/api-gateway/etc/gateway-api.yaml`:
```yaml
FbAdsRpc:
  Target: 127.0.0.1:8082
```
`cmd/api-gateway/etc/gateway-api.docker.yaml`:
```yaml
FbAdsRpc:
  Target: fb-ads-service:8082
```

- [ ] **Step 8: Build**

Run: `go build ./cmd/api-gateway/...`
Expected: succeeds.

- [ ] **Step 9: Commit**

```bash
git add api cmd/api-gateway
git commit -m "feat: implement api-gateway REST endpoints for fbads jobs"
```

---

## Task 6: Wire fb-ads-service into Docker Compose, update docs, extend smoke test

**Files:**
- Modify: `docker-compose.yml`
- Modify: `.env.example`
- Modify: `README.md`
- Modify: `scripts/smoke_test.sh`

**Interfaces:**
- Produces: a runnable `docker compose up` stack including `fb-ads-service`, and a smoke-test script that can exercise either crawl path.

- [ ] **Step 1: Add `fb-ads-service` to `docker-compose.yml`**

Follow the exact pattern already established for `trend-service` in the prior plan (healthcheck-gated `depends_on` on `postgres`/`rabbitmq`/`migrate`, `restart: on-failure:5`, `APIFY_API_TOKEN` passed through as an environment variable):

```yaml
  fb-ads-service:
    build:
      context: .
      dockerfile: cmd/fb-ads-service/Dockerfile
    environment:
      APIFY_API_TOKEN: ${APIFY_API_TOKEN}
    depends_on:
      postgres:
        condition: service_healthy
      rabbitmq:
        condition: service_healthy
      migrate:
        condition: service_completed_successfully
    restart: on-failure:5
    ports:
      - "8082:8082"
```

Also add `fb-ads-service: condition: service_started` to `api-gateway`'s `depends_on` (same treatment as its existing `trend-service` dependency, no healthcheck defined on either gRPC service).

- [ ] **Step 2: Add `APIFY_API_TOKEN=` to `.env.example`**

```
SERPAPI_API_KEY=
OPENROUTER_API_KEY=
APIFY_API_TOKEN=
```

- [ ] **Step 3: Update `README.md`**

Add a short section documenting the second crawl path: `POST /jobs/fbads {"query": "nike", "country": "US"}` / `GET /jobs/fbads/{id}`, and mention `APIFY_API_TOKEN` alongside the existing two keys in the setup section.

- [ ] **Step 4: Extend `scripts/smoke_test.sh` to accept a `--path` flag selecting `trend` (default) or `fbads`**

Modify the script so it builds the POST body and endpoint path based on `--path`:
- `trend` (default, existing behavior): `POST /jobs/trend {"keyword": "<--keyword value>"}`, poll `GET /jobs/trend/$JOB_ID`
- `fbads`: `POST /jobs/fbads {"query": "<--keyword value>", "country": "US"}`, poll `GET /jobs/fbads/$JOB_ID`

Keep the existing `--keyword` flag as the shared search-term input for both paths. Document both invocations in the README (`./scripts/smoke_test.sh --path fbads --keyword "nike"`).

- [ ] **Step 5: Validate compose syntax**

Run: `docker compose config`
Expected: exit 0, no warnings; verify the printed config shows `fb-ads-service` with the correct `postgres:5432` dependency chain (via the `migrate` service, same as `trend-service`/`ai-service`).

- [ ] **Step 6: Commit**

```bash
git add docker-compose.yml .env.example README.md scripts/smoke_test.sh
git commit -m "chore: wire fb-ads-service into docker-compose and extend smoke test"
```

---

## Self-Review Notes

- **Spec coverage:** `pkg/apify` client (Task 2), fb-ads-service consumer+gRPC (Task 3), ai-service job-type branching (Task 4), api-gateway REST endpoints (Task 5), full docker-compose wiring (Task 6), `fbads_raw` schema (Task 1) all covered. Confirmed field types (`body` as `{"text":...}` object, `start_date`/`end_date` as unix seconds) came from a live API spike, not assumption — the struct in Task 2 reflects that.
- **Type consistency:** `JobMessage`/`CompletedMessage` field names in Task 3's worker match what Task 5's `CreateFbAdsJobLogic` publishes (`job_id`, `query`, `page_id`, etc. — JSON tags aligned). `apify.Client`/`apify.AdParams`/`apify.Ad` signatures defined once in Task 2, reused identically in Task 3. `fbads.GetFbAdsJobRequest`/`Response` field names (`JobId`, `Status`, `AdCount`, `AiOutput`) consistent between Task 3's proto/logic and Task 5's gRPC client usage.
- **Known follow-ups (out of scope for this plan, per the design spec):** proxy-broker, TokenRouter as a second AI provider, scheduler/cron — unchanged from the original spec's deferred list.
