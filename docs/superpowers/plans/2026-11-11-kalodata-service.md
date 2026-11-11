# Kalodata Service Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** New `kalodata-service` crawling Kalodata product + video data over its cookie-auth REST API, with a Postgres cookie registry (7-day TTL hard-death + per-day 10 search / 10 detail quota that rotates, not dies).

**Architecture:** RabbitMQ `kalodata.jobs` → worker → `pkg/kalodata` client. Client leases a live, in-quota cookie from the Postgres registry per operation (search/detail), sends it as the `Cookie` header to `www.kalodata.com`, rotates to another cookie on quota exhaustion, marks dead on 7-day expiry / auth rejection. Results → `kalodata_products` / `kalodata_videos`; publish `crawl.completed.kalodata`. gRPC `GetKalodataJob` for api-gateway.

**Tech Stack:** Go (go-zero, zrpc, gRPC), PostgreSQL, RabbitMQ, protoc.

**Spec:** `docs/superpowers/specs/2026-11-11-kalodata-service-design.md`

## Global Constraints

- Migration number is **000015** (latest is `000014_linkedin`).
- `jobs_type_check` keeps ALL existing types + `'kalodata'`.
- Cookie lifecycle: daily quota exhaustion → **rotate to another cookie, never `dead`**. `dead=true` ONLY for 7-day TTL expiry or auth rejection.
- Cookies come from env `KALODATA_COOKIES` (one account per line); never hardcoded.
- Scope: product + video (+ optional video/detail). NO transcript/AI, NO login/account creation.
- Output item field paths are assumption-based → parse defensively, always keep `raw`; verify live with a real cookie.
- Base URL `https://www.kalodata.com`; CDN `https://img.kalocdn.com`.

## Review Focus

- **Concurrent lease double-spends quota** (two jobs, same cookie, both increment) → atomic single-statement increment with `FOR UPDATE SKIP LOCKED` → pinned Task 2 lease test.
- **Day rollover mid-run** → stale `usage_date` rows reset before pick → pinned Task 2 reset test.
- **All cookies out of quota vs all dead** → `ErrQuotaExhausted` (requeue) vs `ErrNoCookies` (fail) distinguished → pinned Task 2.
- **Cookie re-injected / restart** must NOT reset the 7-day clock (keep `first_seen_at`) → pinned Task 2 SyncCookies test.
- **Kalodata returns non-2xx / error JSON** (expired vs quota) → client classifies: auth→MarkDead+retry, quota→rotate, else error → pinned Task 3 (parse/classify test; live-verify the exact shape).

---

### Task 1: Migration `000015_kalodata`

**Files:** Create `db/migrations/000015_kalodata.up.sql`, `db/migrations/000015_kalodata.down.sql`

**Interfaces:** Produces tables `kalodata_cookies`, `kalodata_products`, `kalodata_videos`; `jobs_type_check` includes `'kalodata'`.

- [ ] **Step 1: Write up.sql** — `ALTER jobs_type_check` (+`'kalodata'`, keep all existing). Then:
  - `kalodata_cookies(id UUID PK, cookie_hash TEXT UNIQUE NOT NULL, cookie TEXT NOT NULL, label TEXT, first_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(), expires_at TIMESTAMPTZ NOT NULL, dead BOOLEAN NOT NULL DEFAULT false, usage_date DATE, search_used INT NOT NULL DEFAULT 0, detail_used INT NOT NULL DEFAULT 0, last_used_at TIMESTAMPTZ)`.
  - `kalodata_products(id UUID PK, job_id UUID NOT NULL REFERENCES jobs(id), country TEXT, product_id TEXT, title TEXT, revenue NUMERIC, sales INTEGER, rating NUMERIC, price NUMERIC, commission_rate NUMERIC, thumbnail_url TEXT, product_url TEXT, raw JSONB NOT NULL, fetched_at TIMESTAMPTZ NOT NULL DEFAULT now())` + idx (job_id).
  - `kalodata_videos(id UUID PK, job_id UUID NOT NULL REFERENCES jobs(id), country TEXT, video_id TEXT, handle TEXT, title TEXT, views INTEGER, revenue NUMERIC, sales INTEGER, duration INTEGER, thumbnail_url TEXT, tiktok_url TEXT, detail JSONB, raw JSONB NOT NULL, fetched_at TIMESTAMPTZ NOT NULL DEFAULT now())` + idx (job_id).
- [ ] **Step 2: Write down.sql** — drop the 3 tables; restore prior `jobs_type_check`.
- [ ] **Step 3: Apply to local DB** (postgres container is up):
  `docker compose exec -T postgres psql -U crawl -d crawl -f - < db/migrations/000015_kalodata.up.sql` (or via migrate). Expected: no error; `\d kalodata_cookies` shows columns. This DB doubles as the Task 2/3 test DB.
- [ ] **Step 4: Commit** `git add db/migrations/000015_kalodata.* && git commit -m "feat(kalodata): migration 000015 (cookies registry + products + videos)"`

---

### Task 2: `pkg/kalodata` cookie registry

**Files:** Create `pkg/kalodata/registry.go`, `pkg/kalodata/registry_test.go`

**Interfaces:**
- Produces:
  - `func ParseCookies(env string) []string` — split on newline, trim, drop blanks. (pure)
  - `func CookieHash(c string) string` — sha256 hex. (pure)
  - `type CookieRegistry struct{ db *sql.DB; ttl time.Duration; searchQuota, detailQuota int }`
  - `func NewCookieRegistry(db *sql.DB, ttl time.Duration, searchQuota, detailQuota int) *CookieRegistry`
  - `func (r *CookieRegistry) SyncCookies(ctx, cookies []string) error` — upsert by hash; `ON CONFLICT (cookie_hash) DO UPDATE SET dead=false` (keep `first_seen_at`); new rows get `expires_at = now()+ttl`.
  - `type Lease struct{ ID, Cookie string }`
  - `func (r *CookieRegistry) Lease(ctx, op string) (*Lease, error)` — op `"search"|"detail"`; `ErrNoCookies`, `ErrQuotaExhausted` sentinels.
  - `func (r *CookieRegistry) MarkDead(ctx, id string) error`

- [ ] **Step 1: Write `registry_test.go`** (DB-gated like apify `slots_test`: `testDB` reads `KALODATA_TEST_DSN`, else `t.Skip`; TRUNCATE `kalodata_cookies` per test). Tests:
  - `TestParseCookies` (pure): `"a=1; b=2\n\nc=3\n"` → `["a=1; b=2","c=3"]`.
  - `TestSyncCookies_KeepsFirstSeenOnReSync`: sync cookie X; capture `first_seen_at`; sync again → `first_seen_at` unchanged, `dead=false`, one row.
  - `TestLease_RotatesOnQuota`: 2 cookies, searchQuota=1; `Lease("search")` twice returns the two different IDs; 3rd → `ErrQuotaExhausted`. No row `dead`.
  - `TestLease_SkipsDeadAndExpired`: insert one expired (`expires_at<now`) and one dead; a live one → Lease returns only the live; mark it dead → `ErrNoCookies`.
  - `TestLease_ResetsCountersOnNewDay`: set a cookie `usage_date=yesterday, search_used=quota`; `Lease("search")` resets and succeeds.

- [ ] **Step 2: Run — fail.** `KALODATA_TEST_DSN=... go test ./pkg/kalodata/ -run 'TestParseCookies|TestSync|TestLease' -v` → FAIL (undefined).

- [ ] **Step 3: Implement `registry.go`.** `Lease` in a txn:
  1. `UPDATE kalodata_cookies SET search_used=0, detail_used=0, usage_date=CURRENT_DATE WHERE usage_date IS DISTINCT FROM CURRENT_DATE;`
  2. `UPDATE kalodata_cookies SET <col>=<col>+1, last_used_at=now() WHERE id = (SELECT id FROM kalodata_cookies WHERE dead=false AND expires_at>now() AND <col> < $quota ORDER BY random() LIMIT 1 FOR UPDATE SKIP LOCKED) RETURNING id, cookie;` (`<col>`=`search_used`/`detail_used`).
  3. No row → disambiguate: `SELECT count(*) FILTER (WHERE dead=false AND expires_at>now())` → 0 ⇒ `ErrNoCookies`, else `ErrQuotaExhausted`.
  `SyncCookies`: `INSERT ... ON CONFLICT (cookie_hash) DO UPDATE SET dead=false, cookie=EXCLUDED.cookie` with `expires_at` only in INSERT (DO UPDATE must not touch `first_seen_at`/`expires_at`).

- [ ] **Step 4: Run — pass.** `KALODATA_TEST_DSN='postgres://crawl:crawl@127.0.0.1:5433/crawl?sslmode=disable' go test ./pkg/kalodata/ -v` → PASS (needs Task 1 table in local DB).

- [ ] **Step 5: Commit** `git add pkg/kalodata/registry.go pkg/kalodata/registry_test.go && git commit -m "feat(kalodata): cookie registry (7d TTL + daily quota rotation)"`

---

### Task 3: `pkg/kalodata` HTTP client

**Files:** Create `pkg/kalodata/client.go`, `pkg/kalodata/client_test.go`

**Interfaces:**
- Consumes: Task 2 `CookieRegistry.Lease/MarkDead`.
- Produces:
  - `type ProductParams struct{ Country, StartDate, EndDate string; PageNo, PageSize int; SortField, SortType string; CategoryIDs []int; Revenue, UnitPrice, SalesChannel, Strategy, CommissionRate, Creator, LaunchDate string }`
  - `type VideoParams struct{ Country, StartDate, EndDate string; PageNo, PageSize int; SortField, SortType string; ProductID, VideoType, Duration, CreatorFollowers string }`
  - `type Product struct{ ProductID, Title, ThumbnailURL, ProductURL string; Revenue, Rating, Price, CommissionRate float64; Sales int; Raw json.RawMessage }`
  - `type Video struct{ VideoID, Handle, Title, ThumbnailURL, TikTokURL string; Views, Sales, Duration int; Revenue float64; Detail, Raw json.RawMessage }`
  - `type Client interface{ AllLastDay(ctx, country string) (string, error); SearchProducts(ctx, ProductParams) ([]Product, error); SearchVideos(ctx, VideoParams) ([]Video, error); VideoDetail(ctx, country, id, start, end string) (json.RawMessage, error) }`
  - `func NewHTTPClient(baseURL, cdnBase string, reg *CookieRegistry, retries int) *HTTPClient` (defaults baseURL `https://www.kalodata.com`, cdnBase `https://img.kalocdn.com`).
  - pure helpers `mapProducts(data []json.RawMessage, cdn string) []Product`, `mapVideos(...)`.

- [ ] **Step 1: Write `client_test.go`** with pure parse tests + an httptest round-trip:
  - `TestMapProducts` / `TestMapVideos`: given raw item JSON (`{"id":"p1","title":"x","revenue":12.5,...}`), assert normalized fields + thumbnail URL + (video) tiktok_url + Raw preserved.
  - `TestSearchProducts_SendsCookieAndParses` (DB-gated via KALODATA_TEST_DSN + seeded live cookie row): httptest server asserts `Cookie` header present and path `/product/queryList`, returns `{"data":[{"id":"p1",...}]}`; assert parsed + registry `search_used` incremented. (Skips without DSN.)

- [ ] **Step 2: Run — fail.** `go test ./pkg/kalodata/ -run 'TestMapProducts|TestMapVideos' -v` → FAIL.

- [ ] **Step 3: Implement `client.go`.** Each call: `op` = "search" for queryList, "detail" for video/detail; `lease, err := reg.Lease(ctx, op)` (propagate `ErrQuotaExhausted`/`ErrNoCookies`); build request (JSON payload per spec §2), headers `Cookie: lease.Cookie`, `Origin`, `Referer`, `User-Agent`, `Content-Type`; POST. Classify response: 2xx with `data` → parse; **401/403** → `reg.MarkDead(lease.ID)` + retry next lease up to `retries`; a Kalodata quota/error JSON → treat as that cookie out → retry next lease. `AllLastDay`: GET `/api/allLastDay?country=`. `mapProducts/mapVideos` attach `cdn` thumbnail + video tiktok_url.

- [ ] **Step 4: Run — pass.** `go test ./pkg/kalodata/ -v` → pure tests PASS (DB/httptest ones SKIP without DSN). Build compiles.

- [ ] **Step 5: Commit** `git add pkg/kalodata/client.go pkg/kalodata/client_test.go && git commit -m "feat(kalodata): HTTP client with cookie rotation"`

---

### Task 4: proto `rpc/kalodata` + generate

**Files:** Create `rpc/kalodata/kalodata.proto`; generate `cmd/kalodata-service/kalodata/*.pb.go`.

**Interfaces:** `KalodataService.GetKalodataJob(GetKalodataJobRequest{job_id}) returns (GetKalodataJobResponse{status, product_count, video_count, ai_output})`.

- [ ] **Step 1: Write `kalodata.proto`** (mirror `rpc/linkedin/linkedin.proto`; `package kalodata; option go_package="./kalodata";`).
- [ ] **Step 2: Generate** `protoc --proto_path=rpc/kalodata --go_out=cmd/kalodata-service --go-grpc_out=cmd/kalodata-service rpc/kalodata/kalodata.proto`.
- [ ] **Step 3: Verify** `go build ./cmd/kalodata-service/kalodata/` → OK.
- [ ] **Step 4: Commit** `git add rpc/kalodata cmd/kalodata-service/kalodata && git commit -m "feat(kalodata): grpc proto + generated code"`

---

### Task 5: `cmd/kalodata-service`

**Files (mirror `cmd/linkedin-service/*`):** `internal/config/config.go`, `internal/svc/servicecontext.go`, `internal/worker/consumer.go`, `internal/worker/consumer_test.go`, `internal/server/kalodataserviceserver.go`, `internal/logic/getkalodatajoblogic.go`, `kalodata.go` (main), `etc/kalodata.yaml`, `etc/kalodata.docker.yaml`, `Dockerfile`.

**Interfaces:**
- Consumes: Task 2/3 `kalodata.Client` + `CookieRegistry`; Task 4 gRPC types; Task 1 tables.
- Produces: `worker.Consumer{DB, Client, Publisher}`, `JobMessage{JobID, Country, StartDate, EndDate, PageNo, PageSize, SortField, SortType, CategoryIDs, Revenue, UnitPrice, SalesChannel, CommissionRate, Creator, WithProducts, WithVideos, WithVideoDetail, VideoType, Duration, CreatorFollowers}`.

- [ ] **Step 1: Write `consumer_test.go`** mirroring linkedin's: `fakeKalodata` implementing `kalodata.Client`. Tests: unmarshal error; search error propagates (assert Country/defaults forwarded: Country="VN", PageNo=1, PageSize=20, SortField="revenue", SortType="DESC"); `ErrQuotaExhausted` from client → returned error wraps `rabbitmq.ErrRequeue`.
- [ ] **Step 2: Run — fail.** `go test ./cmd/kalodata-service/internal/worker/` → FAIL.
- [ ] **Step 3: Implement shell** (mirror linkedin):
  - `config.go`: `Postgres`, `RabbitMQ`, `Kalodata{BaseURL default https://www.kalodata.com, CDNBase default https://img.kalocdn.com, Cookies (env KALODATA_COOKIES), CookieTTL default 168h, DailySearchQuota default 10, DailyDetailQuota default 10, Retries default 3}`.
  - `servicecontext.go`: build `CookieRegistry` (ParseCookies(c.Kalodata.Cookies), ttl, quotas) → `SyncCookies` at start (panic on error) → `NewHTTPClient`. `Client kalodata.Client`. Panic if `len(ParseCookies(...))==0`.
  - `consumer.go`: unmarshal `JobMessage`; apply defaults; resolve dates (if empty call `Client.AllLastDay`); if `WithProducts` → `SearchProducts` → INSERT `kalodata_products`; if `WithVideos` → `SearchVideos` → (if `WithVideoDetail` → `VideoDetail` per video, best-effort, stop on `ErrQuotaExhausted`) → INSERT `kalodata_videos`; `errors.As(err,&ErrQuotaExhausted)`→requeue; else error → markFailed; UPDATE job `crawled`; publish `crawl.completed.kalodata`.
  - server + `getkalodatajoblogic.go` (status + `COUNT(*)` from products/videos + ai_results), main (guard cookies, consume `kalodata.jobs`, zrpc register), `etc/*.yaml` (ListenOn `0.0.0.0:8093`, `KALODATA_COOKIES` env), Dockerfile (EXPOSE 8093).
- [ ] **Step 4: Run — pass.** `go test ./cmd/kalodata-service/... -v` → consumer tests PASS; build OK.
- [ ] **Step 5: Commit** `git add cmd/kalodata-service && git commit -m "feat(kalodata): kalodata-service worker + grpc server"`

---

### Task 6: api-gateway wiring

**Files:** Modify `api/gateway.api`, `cmd/api-gateway/internal/types/types.go`, `internal/config/config.go`, `internal/svc/servicecontext.go`; create `internal/logic/{create,get}kalodatajoblogic.go`, `internal/handler/{create,get}kalodatajobhandler.go`; modify `internal/handler/routes.go`; append `KalodataRpc` to `etc/gateway-api*.yaml`.

**Interfaces:** `POST /jobs/kalodata`, `GET /jobs/kalodata/:id`. `CreateKalodataJobRequest{Country, StartDate, EndDate, PageNo, PageSize, SortField, SortType, CategoryIds []int, WithProducts, WithVideos, WithVideoDetail, ...}` (all optional). Publish keys must match Task 5 `JobMessage` json tags exactly.

- [ ] **Step 1:** gateway.api types + 2 routes (mirror linkedin block).
- [ ] **Step 2:** types.go structs.
- [ ] **Step 3:** `createkalodatajoblogic.go` (INSERT jobs type `'kalodata'`, publish `kalodata.jobs` with all fields) + `getkalodatajoblogic.go` (gRPC call).
- [ ] **Step 4:** config `KalodataRpc{Target default 127.0.0.1:8093}`; servicecontext import + field + grpc conn + client; handlers + routes.go 2 routes; `etc/gateway-api.yaml` (`127.0.0.1:8093`) + `.docker.yaml` (`kalodata-service:8093`).
- [ ] **Step 5: Verify** `go build ./...` → OK.
- [ ] **Step 6: Commit** `git add api/gateway.api cmd/api-gateway && git commit -m "feat(api-gateway): kalodata job create/get endpoints"`

---

### Task 7: docker-compose + full verify + live check

**Files:** Modify `docker-compose.yml`.

- [ ] **Step 1:** Add `kalodata-service` block (build, env `KALODATA_COOKIES: ${KALODATA_COOKIES}`, depends postgres/rabbitmq/migrate) + api-gateway `depends_on`.
- [ ] **Step 2: Full verify.** `docker compose config >/dev/null`; `go build ./... && go test ./...` → build OK, tests PASS (DB-gated kalodata tests run if `KALODATA_TEST_DSN` set, else SKIP).
- [ ] **Step 3: Live check (needs a real Kalodata cookie).** Put a live cookie in `KALODATA_COOKIES`, run a job via api-gateway (or a small live test), inspect `kalodata_products`/`kalodata_videos`: confirm normalized columns populated (not just raw) and quota counters increment. If fields empty, open a `raw` row, fix `mapProducts/mapVideos` paths, re-run Task 3 tests, re-verify. Record confirmed paths + the real "quota exhausted" vs "cookie dead" response shape in code comments (adjust client classification).
- [ ] **Step 4: Commit** `git add docker-compose.yml && git commit -m "chore(kalodata): add kalodata-service to docker-compose"`
