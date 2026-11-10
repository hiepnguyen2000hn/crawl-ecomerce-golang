# Thiết kế: linkedin-service — crawl bài post LinkedIn qua Apify (harvestapi/linkedin-post-search)

Ngày: 2026-11-10
Trạng thái: chờ duyệt

## 1. Ý định & lý do

Thêm một microservice mới `linkedin-service` crawl **bài post LinkedIn** bằng Apify
actor **`harvestapi/linkedin-post-search`**, đồng nhất 100% với các service Apify
hiện có (reddit, amazon, china1688). Job search theo **keyword + bộ lọc đầy đủ**
(author/company/content-type…), lưu post vào Postgres, publish event completed.

**Chốt với người dùng:** job drive bằng **keyword + filter đầy đủ**; **chỉ crawl
post** (không comment/reaction — tiết kiệm phí Apify; engagement count likes/
comments/shares vẫn có sẵn trong mỗi post).

## 2. Phát hiện quan trọng (đã recon)

- **Xoay key có sẵn, dùng lại miễn phí.** `pkg/apify.Runner.RunSync(actorID,
  jobID, input)` tự lease Apify token từ Postgres (`apify_tokens`/`SlotStore`),
  park account gần hết budget (`LeasedTokenTransport`), nhả slot concurrency.
  `apify-token-service` làm control-plane refresh budget. Service mới **không tự
  xử lý token** — chỉ gọi Runner như reddit.
- **Không cần tài khoản/cookie LinkedIn.** harvestapi tự quản lý truy cập LinkedIn
  + proxy nội bộ. "proxy/tài khoản" trong hệ này = **Apify token**, đã lo ở trên.
  → Không lưu credential LinkedIn, không field proxy riêng cho actor.
- **Actor input** (`harvestapi/linkedin-post-search`): `searchQueries` (bắt buộc,
  mảng), `maxPosts`, `startPage`, `scrapePages`, `postedLimit`
  (`1h|24h|week|month|3months|6months|year`), `sortBy` (`relevance|date`),
  `authorUrls`, `authorsCompanies`, `authorKeywords`, `contentType`,
  `scrapeComments`/`scrapeReactions` (default false — giữ false).
- **Actor output mỗi post**: `id`, `linkedinUrl`, `content`, `author{}`,
  `postedAt{}`, `postImages`, `engagement{likes,comments,shares,reactions}`.
  Tên field nested chính xác **chưa chắc 100%** (webfetch chỉ cho mức cao) →
  parse phòng thủ + luôn giữ `raw` JSONB; tinh chỉnh path khi có dataset thật
  lúc implement (giống rủi ro DOM của allegro). Giá ~$2/1k posts.

## 3. Goals / Non-goals

**Goals**
- Service `linkedin-service` mirror reddit-service: worker đọc `linkedin.jobs`
  (RabbitMQ) → `LinkedInHTTPClient.SearchPosts` → INSERT `linkedin_posts_raw` →
  `UPDATE jobs status='crawled'` → publish `crawl.completed.linkedin`.
- gRPC `GetLinkedInJob` (status + post_count + ai_output) cho api-gateway.
- api-gateway: `POST /jobs/linkedin`, `GET /jobs/linkedin/:id`.
- Hỗ trợ keyword + filter: sortBy, postedLimit, authorUrls, authorsCompanies,
  authorKeywords, contentType, max_posts, pages.
- Xử lý `ErrBusy` (hết slot mọi account) = requeue, không fail job (như reddit).

**Non-goals**
- Không crawl comment/reaction (giữ `scrapeComments/scrapeReactions=false`).
- Không tích hợp ai-service case `linkedin` (post được lưu; phân tích AI là việc
  sau). GET vẫn đọc `ai_results` nếu có.
- Không lưu tài khoản/cookie LinkedIn.

## 4. Các hướng đã cân nhắc

1. **Service riêng mirror reddit (CHỌN).** Nhất quán với kiến trúc hiện tại, tái
   dùng Runner/SlotStore/token rotation, proto + worker + api-gateway như mọi
   service khác. Rủi ro thấp, dễ review.
2. Nhét vào một service có sẵn (vd reddit) với actor khác. Loại — khác nền tảng,
   khác bảng, phá cô lập job type.

## 5. Quyết định thiết kế (sẽ build)

**Tên:** service `linkedin-service`, job type `linkedin`, queue `linkedin.jobs`,
event `crawl.completed.linkedin`, route `/jobs/linkedin`.

**pkg/apify/linkedin.go** (mirror reddit.go):
```go
type LinkedInPost struct {
    PostID, URL, Content, AuthorName, AuthorURL, PostedAt string
    Likes, Comments, Shares int
    Raw json.RawMessage
}
type LinkedInSearchParams struct {
    Keyword string; MaxPosts, Pages int
    SortBy, PostedLimit, AuthorKeywords, ContentType string
    AuthorUrls, AuthorsCompanies []string
}
type LinkedInSearchResult struct { Posts []LinkedInPost }
type LinkedInClient interface {
    SearchPosts(ctx, jobID string, params LinkedInSearchParams) (LinkedInSearchResult, error)
}
type LinkedInHTTPClient struct { actorID string; runner *Runner }  // default actorID "harvestapi/linkedin-post-search"
```
`linkedinRunInput` map các field actor ở §2. Parse `linkedinItem` phòng thủ, giữ
`rawItems[i]` làm `Raw`. Cắt `MaxPosts`. `searchQueries` rỗng → lỗi.

**Migration `000014_linkedin`:**
- `ALTER jobs_type_check` thêm `'linkedin'` (giữ nguyên fbads/trend/amazon/
  china1688/reddit/reddit_api/allegro).
- Bảng `linkedin_posts_raw(id, job_id FK jobs, post_id, url, content,
  author_name, author_url, posted_at TIMESTAMPTZ, likes, comments_count, shares,
  raw JSONB NOT NULL, fetched_at)` + index `(job_id)`.
> Số migration: mới nhất là `000013_allegro_comments` → dùng **000014**.

**proto `rpc/linkedin/linkedin.proto`** (mirror reddit.proto): `LinkedInService`
với `GetLinkedInJob(GetLinkedInJobRequest{job_id}) returns (GetLinkedInJobResponse
{status, post_count, ai_output})`. Sinh code bằng protoc vào
`cmd/linkedin-service/linkedin/`.

**cmd/linkedin-service/** mirror reddit-service: `linkedin.go` (main), `internal/
config`, `internal/svc` (SlotStore + Runner + LinkedInHTTPClient), `internal/
worker/consumer.go`, `internal/server` + `internal/logic/getlinkedinjoblogic.go`,
`etc/linkedin.yaml` + `linkedin.docker.yaml`, `Dockerfile`.

**JobMessage** (worker): `job_id, keyword, max_posts, pages, sort_by,
posted_limit, author_urls, authors_companies, author_keywords, content_type`.
Defaults: max_posts=25, pages=1, sort_by="relevance".

**api-gateway:** `gateway.api` thêm `CreateLinkedInJobRequest`/`Response`,
`GetLinkedInJobRequest`/`Response` + 2 route; `types.go`; `createlinkedinjoblogic.go`
(INSERT jobs type='linkedin' + publish) và `getlinkedinjoblogic.go` (gRPC call);
`servicecontext` thêm LinkedIn rpc client; handler/routes. Mirror reddit, chỉnh
tay cho khớp (không regen toàn bộ để tránh clobber).

**docker-compose:** thêm block `linkedin-service` (env `APIFY_API_TOKEN`, depends
postgres/rabbitmq/migrate) + api-gateway depends_on `linkedin-service`.

## 6. Phần để lại cho người implement

- Xác nhận path field nested của output actor (author.name, postedAt.*,
  engagement.likes…) bằng 1 dataset thật khi có Apify token; trước đó parse theo
  giả định + giữ `raw`. Verify live bằng `APIFY_API_TOKEN` thật (1 token đủ).
- Sinh proto bằng protoc (tooling đã có); mirror Makefile/convention proto hiện tại.
- Test: `pkg/apify` client test (mock Runner/httptest như reddit/amazon test),
  worker insert test, api-gateway create test. Build `go build ./...`.
- Cập nhật README tổng (nếu có mục liệt kê service).
