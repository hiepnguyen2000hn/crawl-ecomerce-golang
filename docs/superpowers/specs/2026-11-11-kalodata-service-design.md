# Thiết kế: kalodata-service — crawl Product + Video từ Kalodata

Ngày: 2026-11-11
Trạng thái: chờ duyệt

## 1. Ý định & lý do

Service Go mới `kalodata-service` trong repo này, crawl **sản phẩm (product)** và
**video (+ detail)** từ Kalodata REST API (`https://www.kalodata.com`), mirror
pattern reddit/linkedin (job queue → crawl → DB → publish). Khác biệt: Kalodata
**không phải Apify** — là REST API xác thực bằng **session cookie** của tài khoản
trial, nên có **client cookie + xoay account** riêng (không dùng `pkg/apify`).

**Chốt với người dùng:**
- Scope: **Product (`/product/queryList`) + Video (`/video/queryList` + `/video/detail`)**. KHÔNG làm transcript/AI, KHÔNG làm quản lý/tạo tài khoản (login do `farm_account` lo).
- **Cookie nạp từ `.env`** (danh sách nhiều account để xoay).
- Job drive bằng **country + date range + filter/sort**.

## 2. Phát hiện (recon từ farm_account/app/kalo_search_service.py)

- Base URL `https://www.kalodata.com`; CDN ảnh `https://img.kalocdn.com`.
- Auth = **cookie header** (account có ~60 cookie, key chính `SESSION` + cookie
  Cloudflare `__cf_bm`/`_cfuvid`). Client gửi nguyên `Cookie: <chuỗi>` + headers
  `Origin`, `Referer`, `User-Agent`.
- `GET /api/allLastDay?country=VN` → ngày data mới nhất (dùng khi job không truyền date).
- `POST /product/queryList` payload: `{country, startDate, endDate, pageNo, pageSize,
  sort:[{field,type}], cateIds, showCateIds, (optional) revenue, unitPrice,
  salesChannel, strategy, commissionRate, creator, launchDate}` → `{"data":[...]}`.
- `POST /video/queryList` payload: `{country, startDate, endDate, pageNo, pageSize,
  sort:[{field,type}], (optional) productId, videoType, duration, creatorFollowers}`
  → `{"data":[...]}`, mỗi video có `id`, `handle`.
- `POST /video/detail` payload: `{id, country, startDate, endDate}` → `{"data":{...}}`
  (nhạc, hashtags, ads...).
- Thumbnail: product `img.kalocdn.com/tiktok.product/{id}/cover.png`, video
  `img.kalocdn.com/tiktok.video/{id}/cover.png`; TikTok link `tiktok.com/@{handle}/video/{id}`.
- Shape field nội bộ của item (ngoài `id`/`handle`) **chưa chắc chắn** → parse
  phòng thủ + luôn giữ `raw` JSONB; tinh chỉnh khi có cookie thật.

## 3. Goals / Non-goals

**Goals**
- `kalodata-service` worker đọc `kalodata.jobs` → crawl product + video (+ detail
  tuỳ chọn) → lưu `kalodata_products` / `kalodata_videos` → publish `crawl.completed.kalodata`.
- Client `pkg/kalodata` với **xoay cookie/account**: random 1 cookie từ pool (nạp
  từ env), gặp lỗi auth/quota → thử cookie kế (giống retry proxy của allegro).
- gRPC `GetKalodataJob` (status + product_count + video_count) cho api-gateway.
- api-gateway `POST /jobs/kalodata`, `GET /jobs/kalodata/:id`.

**Non-goals**
- Không login/tạo tài khoản, không transcript/VIDEO.AI.
- Không tự refresh cookie hết hạn (farm_account lo; ta chỉ đọc cookie từ env).
- Không track quota chính xác theo ngày ở v1 (chỉ xoay-khi-lỗi); có thể thêm sau.

## 4. Các hướng đã cân nhắc

1. **Service Go riêng + client cookie (CHỌN).** Nhất quán kiến trúc; cookie từ env;
   xoay account in-memory + retry-on-failure. Rủi ro thấp, không phụ thuộc Apify.
2. Gọi sang farm_account (Python) qua subprocess/HTTP. Loại — thêm coupling, khó
   vận hành, trùng trách nhiệm.
3. Lưu cookie/quota trong DB (bảng kalo_accounts). Mạnh hơn cho quota nhưng người
   dùng đã chọn **env** → để sau nếu cần track quota chặt.

## 5. Quyết định thiết kế (sẽ build)

**Tên:** service `kalodata-service` (ListenOn `0.0.0.0:8093`), job type `kalodata`,
queue `kalodata.jobs`, event `crawl.completed.kalodata`, route `/jobs/kalodata`.

**Cookie từ env — `KALODATA_COOKIES`:** mỗi account = một chuỗi `Cookie` header
đầy đủ; **nhiều account phân tách bằng xuống dòng** (mỗi dòng 1 account). Parse:
split theo newline, bỏ dòng trống. Pool rỗng → service panic lúc start (như
APIFY_API_TOKEN). *(Đây là call của tôi — nếu bạn muốn format khác, vd JSON array
hay biến đánh số KALODATA_COOKIE_1/2, nói để tôi đổi.)*

**Quản lý cookie (registry TTL 7 ngày) — tương tự apify-token-service nhưng theo
tuổi cookie, KHÔNG theo budget.** Mỗi cookie sống **7 ngày tính từ lúc inject vào
env lần đầu**; quá 7 ngày coi như chết. Cơ chế (backed Postgres để clock sống qua
restart):
- Bảng `kalodata_cookies(id, cookie_hash UNIQUE, cookie TEXT, label, first_seen_at,
  expires_at, dead BOOLEAN, last_used_at)`. `cookie_hash` = sha256(cookie) để nhận
  diện cookie mà không so cả chuỗi dài.
- `SyncCookies(envCookies)` lúc start: với mỗi cookie trong env, upsert theo
  `cookie_hash`; **chỉ set `first_seen_at=now()` khi cookie mới** (giữ nguyên nếu
  đã có → clock 7 ngày không reset khi restart); `expires_at = first_seen_at + 7d`.
  Cookie trong DB không còn trong env coi như rút (có thể để `dead=true`).
- `LeaseCookie()`: chọn ngẫu nhiên cookie `dead=false AND expires_at > now()`; gặp
  lỗi auth sớm (bị logout trước 7 ngày) → `MarkDead`. Hết cookie sống → `ErrNoCookies`.
- TTL 7 ngày là **suy ra được** (không gọi API ngoài như apify budget) → **không
  cần process control-plane riêng**; làm trong chính kalodata-service (sync lúc
  start + check expires_at mỗi lần lease). *(Nếu sau này muốn tách thành
  `kalodata-cookie-service` quét định kỳ như apify-token-service thì mở rộng được.)*
- TTL cấu hình qua config `Kalodata.CookieTTL` (default `168h` = 7 ngày).

**Quota NGÀY (reset mỗi ngày) — khác TTL 7 ngày.** Mỗi cookie/account trial có:
**10 lượt search/ngày** (product/queryList + video/queryList) và **10 lượt
view-detail/ngày** (video/detail). Hết quota → **XOAY sang cookie khác còn quota**,
TUYỆT ĐỐI không `dead`. Cookie hết lượt chỉ tạm nghỉ, hôm sau `usage_date` đổi →
counter reset → dùng lại. (`dead` CHỈ dành cho quá 7 ngày hoặc bị logout sớm.)
- Bảng `kalodata_cookies` thêm: `usage_date DATE, search_used INT DEFAULT 0,
  detail_used INT DEFAULT 0`.
- `LeaseCookie(op)` với `op ∈ {search, detail}`, trong 1 transaction:
  (1) nếu `usage_date <> CURRENT_DATE` → reset `search_used=0, detail_used=0,
  usage_date=CURRENT_DATE`; (2) chọn cookie sống còn quota cho `op`
  (`search_used < SearchQuota` hoặc `detail_used < DetailQuota`); (3) **tăng counter
  tương ứng + set last_used_at** (atomic, tránh vượt quota khi chạy song song —
  giống SlotStore của apify). Không còn cookie nào còn quota → `ErrQuotaExhausted`.
- Caps cấu hình: `Kalodata.DailySearchQuota` (default 10), `Kalodata.DailyDetailQuota`
  (default 10). Reset mốc theo `CURRENT_DATE` của Postgres (UTC; chỉnh timezone sau
  nếu cần khớp giờ reset thật của Kalodata).
- **Worker tôn trọng quota:** mỗi page queryList = 1 search; mỗi video/detail = 1
  detail. Khi `with_video_detail=true` nhưng hết detail-quota giữa chừng → dừng lấy
  detail cho các video còn lại (best-effort), vẫn lưu video. `ErrQuotaExhausted`
  toàn job → **requeue** (đợi ngày mai reset) thay vì fail.

**pkg/kalodata/client.go:**
```go
type Client interface {
    AllLastDay(ctx, country string) (string, error)
    SearchProducts(ctx context.Context, p ProductParams) ([]Product, error)
    SearchVideos(ctx context.Context, p VideoParams) ([]Video, error)
    VideoDetail(ctx context.Context, country, id, startDate, endDate string) (json.RawMessage, error)
}
```
- `HTTPClient{baseURL, cdnBase, cookies []string}`; mỗi call: random cookie, gửi
  `Cookie` + headers; lỗi auth (401/403 hoặc JSON code lỗi / "login") hoặc quota →
  park cookie đó + thử cookie kế (tối đa N). Hết cookie tốt → `ErrNoCookies`.
- `Product`/`Video` parse `id`, một số field phẳng (revenue, sales, rating, title,
  handle...) + `Raw json.RawMessage`; gắn thumbnail/tiktok_url.
- Pure helper `parseItems` test được (không mạng).

**Migration `000015_kalodata`:**
- `jobs_type_check` += `'kalodata'`.
- `kalodata_cookies(id, cookie_hash TEXT UNIQUE, cookie TEXT, label, first_seen_at,
  expires_at, dead BOOLEAN DEFAULT false, last_used_at, usage_date DATE,
  search_used INT DEFAULT 0, detail_used INT DEFAULT 0)` — registry: TTL 7 ngày +
  quota ngày (10 search / 10 detail, reset theo CURRENT_DATE).
- `kalodata_products(id, job_id, country, product_id, title, revenue, sales, rating,
  price, commission_rate, thumbnail_url, product_url, raw JSONB, fetched_at)` + idx job_id.
- `kalodata_videos(id, job_id, country, video_id, handle, title, views, revenue,
  sales, duration, thumbnail_url, tiktok_url, detail JSONB, raw JSONB, fetched_at)` + idx job_id.
> Số migration mới nhất là 000014 → dùng **000015**.

**JobMessage (worker):** `job_id, country, start_date, end_date, page_no, page_size,
sort_field, sort_type, category_ids[], revenue, unit_price, sales_channel,
commission_rate, creator, with_products(bool), with_videos(bool),
with_video_detail(bool), video_type, duration, creator_followers`. Defaults:
country="VN", page_no=1, page_size=20, sort_field="revenue", sort_type="DESC",
with_products=true, with_videos=true, with_video_detail=false. date trống →
`AllLastDay`.

**Worker:** nếu with_products → SearchProducts → insert kalodata_products. Nếu
with_videos → SearchVideos → (với mỗi video, nếu with_video_detail → VideoDetail,
gắn vào `detail`) → insert kalodata_videos. Lỗi cookie hết → `ErrNoCookies` → mark
failed (hoặc requeue — chốt: **requeue** nếu còn khả năng cookie khác tốt; ở đây
cookie từ env tĩnh nên hết tốt = fail). Lưu best-effort cho detail.

**proto `rpc/kalodata`:** `GetKalodataJob(job_id) → {status, product_count, video_count, ai_output}`; sinh bằng protoc.

**cmd/kalodata-service** mirror linkedin-service (config có `Kalodata{BaseURL
default https://www.kalodata.com, CDNBase default https://img.kalocdn.com, Cookies
(env KALODATA_COOKIES), Retries default 3}`), svc, worker(+test), server+logic, main,
etc yaml, Dockerfile.

**api-gateway:** gateway.api types+routes, types.go, create/get logic, servicecontext
rpc client (`KalodataRpc` target 127.0.0.1:8093 / kalodata-service:8093), handler, routes.

**docker-compose:** block `kalodata-service` (env `KALODATA_COOKIES`) + api-gateway depends.

## 6. Phần để lại cho người implement / cần từ người dùng

- **Cần 1 cookie Kalodata còn hạn** (của bạn) để **verify live**: xác nhận endpoint
  product/video chạy, chốt tên field nội bộ của item (revenue/sales/title/handle...),
  và cách Kalodata báo lỗi cookie hết hạn/quota (để trigger xoay). Trước đó parse
  theo giả định + giữ `raw`.
- Chốt format env cookie (newline vs JSON array vs biến đánh số).
- Có cần endpoint trả data (`GET /jobs/kalodata/:id/products|videos`) như linkedin
  `/posts` không — hay query DB là đủ (có thể thêm sau).
