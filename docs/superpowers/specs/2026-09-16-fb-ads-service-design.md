# fb-ads-service Design (Apify — Facebook Ads Library)

Date: 2026-09-16

## Mục tiêu

Thêm crawl path thứ hai vào hệ thống (bên cạnh trend-service đã có): crawl dữ liệu quảng cáo Facebook Ads Library qua Apify actor `igolaizola/facebook-ad-library-scraper`, theo đúng pattern kiến trúc đã dùng cho trend-service (api-gateway → RabbitMQ → crawl service → Postgres → ai-service → OpenRouter).

Actor: `igolaizola~facebook-ad-library-scraper` (Apify actor ID dùng `~` khi gọi REST API), gọi qua endpoint đồng bộ:
`POST https://api.apify.com/v2/acts/igolaizola~facebook-ad-library-scraper/run-sync-get-dataset-items?token=<APIFY_API_TOKEN>`

### Input schema của actor (xác nhận qua spike gọi API thật)

| Field | Type | Bắt buộc | Default | Ghi chú |
|---|---|---|---|---|
| `maxItems` | int | có (actor yêu cầu) | 10 | luôn gửi, mặc định 10 nếu client không truyền |
| `query` | string | không | - | dùng hoặc `query` hoặc `pageId` |
| `pageId` | string | không | - | dùng hoặc `query` hoặc `pageId` |
| `country` | string (enum ISO) | không | `ALL` | |
| `category` | string enum | không | `all` | all/issues_elections_politics/housing/employment/financial_products |
| `mediaType` | string enum | không | `all` | all/image/meme/image_and_meme/video/none |
| `sortBy` | string enum | không | `mostRecent` | mostRecent/impressionsDesc |
| `activeStatus` | string enum | không | `active` | active/inactive/all |
| `minDate`/`maxDate` | string `YYYY-MM-DD` | không | - | |
| `advertisers` | []string (page IDs) | không | `[]` | |
| `fetchDetails` | bool | không | `false` | tốn thêm phí, lấy thêm chi tiết advertiser |

Toàn bộ field trên đều **optional** ở tầng API của hệ thống (api-gateway) — client hiện tại chỉ cần truyền `query` + `country`, các field còn lại dùng default của actor.

### Output mẫu (xác nhận qua spike chạy thử job thật, `query=Nike, country=US, maxItems=3`)

Mỗi ad record gồm 2 nhóm field:
- **Top-level:** `ad_archive_id`, `page_id`, `page_name`, `is_active`, `start_date`/`end_date` (unix timestamp), `publisher_platform[]`, `categories`, `spend`, `reach_estimate`, `currency`
- **`snapshot` (nội dung quảng cáo):** `body`, `title`, `cta_text`, `cta_type`, `link_url`, `link_description`, `images[]`, `videos[]`, `cards[]`, `page_profile_picture_url`, `page_like_count`

## Quyết định lưu trữ

Sau khi đánh giá tốc độ truy vấn: **hybrid** — tách cột cho các field quan trọng nhất mà ai-service và truy vấn tương lai chắc chắn cần (`body`, `title`, `cta_text`, `cta_type`, `link_url`, `page_name`, `is_active`, `start_date`, `end_date`), phần còn lại (`images`, `videos`, `cards`, mọi field khác) gom vào 1 cột `raw JSONB` để không mất dữ liệu và chịu được actor đổi output. Lý do: ai-service đọc trực tiếp `body`/`title` để build prompt phân tích — có cột riêng nhanh hơn nhiều so với đào trong JSONB, và filter/sort theo `is_active`/`start_date`/`cta_type` dùng index B-tree chuẩn thay vì cần GIN index phức tạp.

## 1. Service breakdown (bổ sung vào spec gốc)

| Service | Vai trò | Giao tiếp |
|---|---|---|
| **fb-ads-service** (mới) | Consume job từ queue `fbads.jobs`, gọi Apify actor, parse & lưu Postgres, publish event hoàn thành | RabbitMQ consumer + gRPC server (query kết quả), theo đúng pattern trend-service |
| **api-gateway** (mở rộng) | Thêm `POST /jobs/fbads`, `GET /jobs/fbads/{id}` | REST, publish routing key `fbads.jobs`, gRPC client tới fb-ads-service |
| **ai-service** (mở rộng) | Thêm bind routing key `crawl.completed.fbads`, build prompt từ `body`+`title` các ad thay vì trend_data | RabbitMQ consumer (đã có, thêm 1 binding) |

## 2. Data flow / job lifecycle

```
Client → POST /jobs/fbads {query?, pageId?, country?, maxItems?, ...} (api-gateway)
       → insert jobs (type='fbads', status='pending', params=jsonb toàn bộ input)
       → publish routing key "fbads.jobs"
       → trả job_id

fb-ads-service (consumer)
       → gọi Apify run-sync-get-dataset-items với input từ params
       → với mỗi ad trong response: insert 1 row vào fbads_raw
       → update jobs.status = 'crawled'
       → publish routing key "crawl.completed.fbads" ({"job_id":...})

ai-service (consumer, thêm binding "crawl.completed.fbads")
       → load các fbads_raw theo job_id
       → build prompt từ body+title của các ads (vd: "Phân tích các mẫu quảng cáo sau: ...")
       → gọi OpenRouter, lưu ai_results (prompt_type='analysis', provider='openrouter')
       → update jobs.status = 'done'

Client → GET /jobs/fbads/{id} → status, ai_output, danh sách ads tóm tắt
```

Retry/DLQ: dùng lại `pkg/rabbitmq` đã có (exchange `crawl`, DLQ theo queue).

## 3. Data model (PostgreSQL — bảng mới)

```sql
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

`jobs.type` CHECK constraint mở rộng thêm `'fbads'`. `ai_results` dùng lại nguyên bảng đã có (không đổi schema).

## 4. `pkg/apify` — client package mới (mirror `pkg/serpapi`)

```go
type AdParams struct {
    Query, PageID, Country, Category, MediaType, SortBy, ActiveStatus string
    MinDate, MaxDate string
    MaxItems int
    Advertisers []string
    FetchDetails bool
}

type Ad struct {
    AdArchiveID, PageID, PageName string
    IsActive bool
    StartDate, EndDate time.Time
    Body, Title, CtaText, CtaType, LinkURL string
    Raw json.RawMessage
}

type AdsResult struct {
    Ads []Ad
}

type Client interface {
    FetchAds(ctx context.Context, params AdParams) (AdsResult, error)
}

func NewHTTPClient(apiToken, actorID, baseURL string, httpClient *http.Client) *HTTPClient
```

`actorID` cấu hình được (mặc định `igolaizola~facebook-ad-library-scraper`) để có thể đổi actor sau này mà không sửa code. `MaxItems` mặc định 10 nếu client không set (>0).

## 5. Tech stack / repo layout (bổ sung)

```
pkg/apify/{client.go, client_test.go}
rpc/fbads/fbads.proto
cmd/fb-ads-service/{etc/fbads.yaml, etc/fbads.docker.yaml, internal/..., main.go, Dockerfile}
db/migrations/000002_fbads.up.sql / .down.sql
```

`api/gateway.api` mở rộng thêm `CreateFbAdsJob`/`GetFbAdsJob`. `docker-compose.yml` thêm service `fb-ads-service`. `.env.example` thêm `APIFY_API_TOKEN=`.

## Out of scope (giữ nguyên từ spec gốc)

- proxy-broker (chưa cần dùng ở đây — Apify tự quản lý proxy phía họ)
- TokenRouter provider (vẫn chỉ dùng OpenRouter)
- Scheduler/cron
