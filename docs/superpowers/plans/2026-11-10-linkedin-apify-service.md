# LinkedIn Apify Service Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** New `linkedin-service` microservice that crawls LinkedIn posts via the Apify actor `harvestapi/linkedin-post-search`, mirroring `reddit-service`.

**Architecture:** RabbitMQ `linkedin.jobs` → worker → `pkg/apify.LinkedInHTTPClient.SearchPosts` (token rotation/slots reused from `pkg/apify.Runner`) → INSERT `linkedin_posts_raw` → publish `crawl.completed.linkedin`. gRPC `GetLinkedInJob` for api-gateway status. Posts only (no comments/reactions).

**Tech Stack:** Go (go-zero, zrpc, gRPC), Apify, PostgreSQL, RabbitMQ, protoc.

**Spec:** `docs/superpowers/specs/2026-11-10-linkedin-apify-service-design.md`

## Global Constraints

- Migration number is **000014** (latest is `000013_allegro_comments`).
- `jobs_type_check` must keep ALL existing types and add `'linkedin'`: `fbads, trend, amazon, china1688, reddit, reddit_api, allegro, linkedin`.
- Actor id default `harvestapi/linkedin-post-search`; `scrapeComments`/`scrapeReactions` stay false.
- Token rotation / concurrency slots come entirely from `pkg/apify` (Runner + SlotStore) — do NOT re-implement; mirror `reddit-service` wiring exactly.
- Output actor field paths are unverified — parse defensively and always keep `raw`; verify against a real dataset in Task 2 Step 6.
- `ErrBusy` from Apify → requeue (`rabbitmq.ErrRequeue`), never mark the job failed.

## Review Focus

- **Actor returns fields under unexpected JSON paths** (author/postedAt/engagement nested differently) → post rows store empty strings/zeros but `raw` is intact → pinned Task 2 parse test + live verify (Step 6).
- **Empty `searchQueries`** (no keyword) → client returns an error, job marked failed, not a silent empty crawl → pinned Task 2.
- **All Apify accounts busy** → `ErrBusy` → requeue not fail → pinned Task 4 consumer test.
- **Unmarshal of a malformed job message** → error, no DB write → pinned Task 4.
- **`max_posts`/`pages` unset in message** → defaults (25 / 1) applied → pinned Task 4 consumer test (params forwarded to client).

---

### Task 1: Migration `000014_linkedin`

**Files:**
- Create: `db/migrations/000014_linkedin.up.sql`
- Create: `db/migrations/000014_linkedin.down.sql`

**Interfaces:**
- Produces: table `linkedin_posts_raw(id, job_id, post_id, url, content, author_name, author_url, posted_at, likes, comments_count, shares, raw, fetched_at)`; `jobs_type_check` includes `'linkedin'`.

- [ ] **Step 1: Write `000014_linkedin.up.sql`**: `ALTER TABLE jobs DROP CONSTRAINT IF EXISTS jobs_type_check;` then re-add CHECK with all existing types + `'linkedin'`. `CREATE TABLE IF NOT EXISTS linkedin_posts_raw (...)` per spec §5 (`job_id UUID NOT NULL REFERENCES jobs(id)`, `posted_at TIMESTAMPTZ`, `raw JSONB NOT NULL`, `fetched_at TIMESTAMPTZ NOT NULL DEFAULT now()`) + `CREATE INDEX IF NOT EXISTS linkedin_posts_raw_job_id_idx ON linkedin_posts_raw(job_id)`.

- [ ] **Step 2: Write `000014_linkedin.down.sql`**: `DROP TABLE IF EXISTS linkedin_posts_raw;` (constraint revert optional — leave `'linkedin'` out of scope for down, or restore prior CHECK; mirror `000013` down style).

- [ ] **Step 3: Commit**

```bash
git add db/migrations/000014_linkedin.up.sql db/migrations/000014_linkedin.down.sql
git commit -m "feat(linkedin): add linkedin_posts_raw table (000014)"
```

---

### Task 2: `pkg/apify/linkedin.go` client

**Files:**
- Create: `pkg/apify/linkedin.go`
- Create: `pkg/apify/linkedin_test.go`

**Interfaces:**
- Consumes: `Runner.RunSync(ctx, actorID, jobID, input) ([]json.RawMessage, error)`, `DecodeItems[T]`.
- Produces:
  - `type LinkedInPost struct { PostID, URL, Content, AuthorName, AuthorURL, PostedAt string; Likes, Comments, Shares int; Raw json.RawMessage }`
  - `type LinkedInSearchParams struct { Keyword string; MaxPosts, Pages int; SortBy, PostedLimit, AuthorKeywords, ContentType string; AuthorUrls, AuthorsCompanies []string }`
  - `type LinkedInSearchResult struct { Posts []LinkedInPost }`
  - `type LinkedInClient interface { SearchPosts(ctx context.Context, jobID string, params LinkedInSearchParams) (LinkedInSearchResult, error) }`
  - `func NewLinkedInHTTPClient(actorID string, runner *Runner) *LinkedInHTTPClient` (default actorID `harvestapi/linkedin-post-search`)
  - pure helper `func mapLinkedInItems(raw []json.RawMessage, maxPosts int) ([]LinkedInPost, error)` — DB-free, unit-testable.

- [ ] **Step 1: Write `linkedin_test.go`** with a pure-parse test `TestMapLinkedInItems`

```go
func TestMapLinkedInItems(t *testing.T) {
    raw := []json.RawMessage{
        json.RawMessage(`{"id":"p1","linkedinUrl":"https://linkedin.com/p1","content":"hello","author":{"name":"Ann","linkedinUrl":"https://linkedin.com/in/ann"},"postedAt":{"date":"2026-01-02 10:00:00"},"engagement":{"likes":5,"comments":2,"shares":1}}`),
    }
    posts, err := mapLinkedInItems(raw, 10)
    // assert: 1 post; PostID p1; URL set; Content hello; AuthorName Ann; AuthorURL set; Likes 5; Comments 2; Shares 1; Raw == raw[0]
}
```
Also a `TestMapLinkedInItems_CapsMaxPosts` (3 items, maxPosts 2 → 2).

- [ ] **Step 2: Run test — fail**

Run: `go test ./pkg/apify/ -run TestMapLinkedInItems -v`
Expected: FAIL (undefined `mapLinkedInItems`).

- [ ] **Step 3: Implement** `linkedin.go`: `linkedinRunInput` (searchQueries, maxPosts, startPage, scrapePages, postedLimit, sortBy, authorUrls, authorsCompanies, authorKeywords, contentType), `linkedinItem` output struct (defensive nested `author{name,linkedinUrl}`, `postedAt{date}`, `engagement{likes,comments,shares}`), `mapLinkedInItems`, and `SearchPosts` (require keyword else error; default MaxPosts=25; build input; `runner.RunSync`; `mapLinkedInItems`).

- [ ] **Step 4: Run test — pass**

Run: `go test ./pkg/apify/ -run TestMapLinkedInItems -v`
Expected: PASS.

- [ ] **Step 5: Add client integration test** `TestLinkedInHTTPClient_SearchPosts` mirroring `client_test.go` (`newTestClient` pattern → httptest server returning a fake dataset, assert path `/acts/harvestapi~linkedin-post-search/run-sync-get-dataset-items`, token, body contains `searchQueries`, and parsed posts). Note: uses `testDB(t)` → **skips** without `APIFY_SLOTS_TEST_DSN`; that's expected locally.

Run: `go test ./pkg/apify/ -run TestLinkedIn -v` → PASS or SKIP (no DSN). Build must compile.

- [ ] **Step 6: Live field-path verification (manual, required before Task done)**

Run with a real token (1 is enough):
```bash
APIFY_API_TOKEN=xxxx go test ./pkg/apify/ -run TestLive_LinkedIn -v   # if a live test is added
# or run the service end-to-end later in Task 6.
```
If no token now: mark deferred to Task 6 live check; keep `raw` so nothing is lost. Record confirmed field paths in a code comment.

- [ ] **Step 7: Commit**

```bash
git add pkg/apify/linkedin.go pkg/apify/linkedin_test.go
git commit -m "feat(apify): LinkedInHTTPClient for harvestapi/linkedin-post-search"
```

---

### Task 3: proto `rpc/linkedin` + generated code

**Files:**
- Create: `rpc/linkedin/linkedin.proto`
- Create (generated): `cmd/linkedin-service/linkedin/linkedin.pb.go`, `cmd/linkedin-service/linkedin/linkedin_grpc.pb.go`

**Interfaces:**
- Produces: `LinkedInService` gRPC with `GetLinkedInJob(GetLinkedInJobRequest{job_id}) returns (GetLinkedInJobResponse{status, post_count, ai_output})`.

- [ ] **Step 1: Write `rpc/linkedin/linkedin.proto`** mirroring `rpc/reddit/reddit.proto`: `package linkedin; option go_package = "./linkedin";` messages `GetLinkedInJobRequest{string job_id=1}`, `GetLinkedInJobResponse{string status=1; int32 post_count=2; string ai_output=3}`, service `LinkedInService`.

- [ ] **Step 2: Generate Go code** into `cmd/linkedin-service/linkedin/`

Run (mirror how `cmd/reddit-service/reddit/*.pb.go` was produced):
```bash
protoc --proto_path=rpc/linkedin \
  --go_out=cmd/linkedin-service --go_opt=paths=import \
  --go-grpc_out=cmd/linkedin-service --go-grpc_opt=paths=import \
  rpc/linkedin/linkedin.proto
```
Expected: two `.pb.go` files under `cmd/linkedin-service/linkedin/`. (Adjust flags to match the existing reddit output layout; the deliverable is pb.go files in that dir that compile.)

- [ ] **Step 3: Verify compile**

Run: `go build ./cmd/linkedin-service/linkedin/`
Expected: no error.

- [ ] **Step 4: Commit**

```bash
git add rpc/linkedin cmd/linkedin-service/linkedin
git commit -m "feat(linkedin): linkedin gRPC proto + generated code"
```

---

### Task 4: `cmd/linkedin-service` (config, svc, worker, server, main)

**Files (mirror `cmd/reddit-service/*`):**
- Create: `internal/config/config.go`, `internal/svc/servicecontext.go`, `internal/worker/consumer.go`, `internal/worker/consumer_test.go`, `internal/server/linkedinserviceserver.go`, `internal/logic/getlinkedinjoblogic.go`, `linkedin.go` (main), `etc/linkedin.yaml`, `etc/linkedin.docker.yaml`, `Dockerfile`.

**Interfaces:**
- Consumes: Task 2 `apify.LinkedInClient`/`NewLinkedInHTTPClient`; Task 3 `linkedin.*` gRPC types; `linkedin_posts_raw` (Task 1).
- Produces: `worker.Consumer{DB, Apify, Publisher}` reading `linkedin.jobs`; `JobMessage{JobID, Keyword, MaxPosts, Pages, SortBy, PostedLimit, AuthorUrls, AuthorsCompanies, AuthorKeywords, ContentType}`.

- [ ] **Step 1: Write `consumer_test.go`** mirroring reddit's: `fakeLinkedIn` implementing `LinkedInClient` (records params, returns err/result). Tests:
  - `TestHandleMessage_UnmarshalError` → error on bad JSON.
  - `TestHandleMessage_SearchError` → error propagated; assert keyword forwarded.
  - `TestHandleMessage_Busy_Requeues` → client returns `&apify.ErrBusy{}` → returned error wraps `rabbitmq.ErrRequeue`.
  - `TestHandleMessage_DefaultsParams` → message without max_posts/pages → `fake` sees MaxPosts==25, Pages==1, SortBy=="relevance".

- [ ] **Step 2: Run tests — fail**

Run: `go test ./cmd/linkedin-service/internal/worker/ -v`
Expected: FAIL (package/types missing).

- [ ] **Step 3: Implement the service shell** mirroring reddit:
  - `config.go`: same as reddit (`Postgres`, `RabbitMQ`, `Apify{ApiToken,ActorID,BaseURL,MaxConcurrencyPerToken}`).
  - `servicecontext.go`: `SlotStore` + `Runner` + `NewLinkedInHTTPClient`; `Apify apify.LinkedInClient`.
  - `consumer.go`: `HandleMessage` unmarshals `JobMessage`, builds `apify.LinkedInSearchParams` (defaults MaxPosts=25, Pages=1, SortBy="relevance"), calls `SearchPosts`, `ErrBusy`→requeue, else on error `markFailed`; INSERT each post into `linkedin_posts_raw` (`NULLIF($posted,'')::timestamptz`), `UPDATE jobs status='crawled'`, publish `crawl.completed.linkedin`.
  - `getlinkedinjoblogic.go` + `server`: status + `COUNT(*) FROM linkedin_posts_raw` + `ai_results` lookup → `GetLinkedInJobResponse`.
  - `linkedin.go` main: mirror reddit main (ParseTokens guard, consumer on `linkedin.jobs`, zrpc server registering `LinkedInService`).
  - `etc/*.yaml`, `Dockerfile` mirror reddit (ListenOn new port e.g. `0.0.0.0:8091`).

- [ ] **Step 4: Run tests — pass**

Run: `go test ./cmd/linkedin-service/... -v`
Expected: PASS (consumer tests; others compile).

- [ ] **Step 5: Commit**

```bash
git add cmd/linkedin-service
git commit -m "feat(linkedin): linkedin-service worker + grpc server"
```

---

### Task 5: api-gateway wiring

**Files:**
- Modify: `api/gateway.api` (types + routes)
- Modify: `cmd/api-gateway/internal/types/types.go`
- Create: `cmd/api-gateway/internal/logic/createlinkedinjoblogic.go`, `cmd/api-gateway/internal/logic/getlinkedinjoblogic.go`
- Modify: `cmd/api-gateway/internal/svc/servicecontext.go` (add LinkedIn rpc client)
- Create/Modify: `cmd/api-gateway/internal/handler/*` + route registration (mirror reddit handlers)

**Interfaces:**
- Consumes: Task 3 `linkedin` gRPC client; Task 4 service on its ListenOn; `JobMessage` fields (Task 4).
- Produces: `POST /jobs/linkedin`, `GET /jobs/linkedin/:id`.

- [ ] **Step 1: Add to `api/gateway.api`** `CreateLinkedInJobRequest{Keyword required; MaxPosts; Pages; SortBy; PostedLimit; AuthorUrls []string; AuthorsCompanies []string; AuthorKeywords; ContentType — all optional except keyword}`, `CreateLinkedInJobResponse{JobId}`, `GetLinkedInJobRequest{Id path}`, `GetLinkedInJobResponse{Status; PostCount; AiOutput}`, and the two routes.

- [ ] **Step 2: Sync `types.go`** with the new structs (hand-edit to match, mirroring reddit types).

- [ ] **Step 3: Implement `createlinkedinjoblogic.go`** (mirror reddit): validate keyword, INSERT `jobs` type `'linkedin'`, publish `linkedin.jobs` with all fields.

- [ ] **Step 4: Implement `getlinkedinjoblogic.go`** (mirror reddit): call LinkedIn gRPC `GetLinkedInJob`.

- [ ] **Step 5: Wire rpc client in `servicecontext.go`** + handler + route registration mirroring reddit (`LinkedInRpc`/zrpc client to the service; register handler for the 2 routes).

- [ ] **Step 6: Verify build**

Run: `go build ./...`
Expected: no error.

- [ ] **Step 7: Commit**

```bash
git add api/gateway.api cmd/api-gateway
git commit -m "feat(api-gateway): linkedin job create/get endpoints"
```

---

### Task 6: docker-compose + full verify + live check

**Files:**
- Modify: `docker-compose.yml`

- [ ] **Step 1: Add `linkedin-service` block** mirroring `reddit-service` (build, `APIFY_API_TOKEN` env, depends_on postgres/rabbitmq/migrate) and add it to `api-gateway` `depends_on`.

- [ ] **Step 2: Full build + tests**

Run: `go build ./... && go test ./...`
Expected: build OK; tests PASS (apify DB tests SKIP without DSN).

- [ ] **Step 3: Live field-path verification (required, needs a real `APIFY_API_TOKEN`)**

Bring up the stack (or run the service locally against a real token), POST a job:
```bash
curl -X POST http://localhost:8888/jobs/linkedin \
  -H 'Content-Type: application/json' \
  -d '{"keyword":"golang developer","max_posts":5,"sort_by":"date","posted_limit":"month"}'
```
Inspect `linkedin_posts_raw`: confirm `content/author_name/author_url/posted_at/likes/comments_count/shares` are populated (not just `raw`). If any are empty, open one `raw` row, fix the `linkedinItem` field paths in `pkg/apify/linkedin.go`, re-run Task 2 tests, re-verify. Record the confirmed paths in a code comment.

- [ ] **Step 4: Commit**

```bash
git add docker-compose.yml
git commit -m "chore(linkedin): add linkedin-service to docker-compose"
```
