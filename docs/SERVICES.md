# Các service crawl dữ liệu

Tài liệu này mô tả từng service đang crawl dữ liệu trong hệ thống: crawl **cái gì**, dùng **tool/API** nào, lưu vào **bảng** nào trong Postgres, và **dữ liệu mẫu** thực tế theo schema hiện có. Tổng quan kiến trúc, cách chạy, cách test xem [README.md](../README.md).

---

## 1. trend-service — Google Trends

- **Crawl gì:** dữ liệu xu hướng tìm kiếm (search interest over time) theo từ khoá.
- **Tool/API dùng:** [SerpApi](https://serpapi.com) (Google Trends engine) — client tại [`pkg/serpapi`](../pkg/serpapi).
- **Cơ chế:** `api-gateway` nhận job → đẩy vào RabbitMQ → `trend-service` (gRPC + worker) tiêu thụ job, gọi SerpApi, lưu kết quả thô vào bảng `trend_raw`.
- **Bảng lưu:** `trend_raw` (`db/migrations/000001_init.up.sql`).
- **Request mẫu:**

```sh
curl -X POST localhost:8888/jobs/trend -d '{"keyword": "wireless earbuds"}'
```

- **Dữ liệu mẫu lưu trong `trend_raw.trend_data` (JSONB, rút gọn từ response SerpApi):**

```json
{
  "keyword": "wireless earbuds",
  "interest_over_time": {
    "timeline_data": [
      { "date": "Sep 7 – 13, 2026", "value": 62 },
      { "date": "Sep 14 – 20, 2026", "value": 71 },
      { "date": "Sep 21 – 27, 2026", "value": 88 }
    ]
  },
  "related_queries": {
    "rising": [
      { "query": "wireless earbuds bluetooth 5.3", "value": "+250%" }
    ],
    "top": [
      { "query": "best wireless earbuds", "value": 100 }
    ]
  }
}
```

---

## 2. fb-ads-service — Facebook Ads Library

- **Crawl gì:** quảng cáo đang/đã chạy trên Facebook Ads Library theo từ khoá/tên trang và quốc gia.
- **Tool/API dùng:** [Apify](https://apify.com) actor cho Facebook Ads Library — client tại [`pkg/apify`](../pkg/apify).
- **Cơ chế:** giống trend-service — `api-gateway` tạo job → RabbitMQ → `fb-ads-service` gọi Apify actor → lưu từng ad vào `fbads_raw`.
- **Bảng lưu:** `fbads_raw` (`db/migrations/000002_fbads.up.sql`).
- **Request mẫu:**

```sh
curl -X POST localhost:8888/jobs/fbads -d '{"query": "nike", "country": "US"}'
```

- **Dữ liệu mẫu (1 row trong `fbads_raw`):**

| cột | giá trị mẫu |
|---|---|
| `ad_archive_id` | `1234567890123456` |
| `page_name` | `Nike` |
| `is_active` | `true` |
| `start_date` | `2026-08-01T00:00:00Z` |
| `body` | `"Just Do It. New Air Max drops this week."` |
| `title` | `"Air Max 2026 — Shop Now"` |
| `cta_text` | `"Shop Now"` |
| `link_url` | `https://www.nike.com/launch/t/air-max-2026` |
| `raw` | JSON gốc trả về từ Apify (toàn bộ ad object) |

---

## 3. amazon-service — Amazon Product

- **Crawl gì:** dữ liệu sản phẩm Amazon — tìm theo **từ khoá** hoặc crawl trực tiếp theo **URL/ASIN**.
- **Tool/API dùng:** Apify actor cho Amazon — cùng client [`pkg/apify`](../pkg/apify).
- **Cơ chế:** `api-gateway` → RabbitMQ → `amazon-service` (gRPC + worker, proto tại [`rpc/amazon`](../rpc/amazon)) gọi Apify, lưu vào `amazon_raw`.
- **Bảng lưu:** `amazon_raw` (`db/migrations/000005_amazon.up.sql`).
- **Request mẫu (tìm theo từ khoá):**

```sh
curl -X POST localhost:8888/jobs/amazon \
  -d '{"keyword": "wireless earbuds", "country": "US"}'
```

- **Request mẫu (crawl theo URL cụ thể):**

```sh
curl -X POST localhost:8888/jobs/amazon \
  -d '{"urls": ["https://www.amazon.com/dp/B0CX23V2ZK"]}'
```

- **Dữ liệu mẫu (1 row trong `amazon_raw`):**

| cột | giá trị mẫu |
|---|---|
| `asin` | `B0CX23V2ZK` |
| `title` | `Soundcore by Anker Wireless Earbuds` |
| `price` | `39.99` |
| `currency` | `USD` |
| `rating` | `4.5` |
| `reviews_count` | `18234` |
| `brand` | `Soundcore` |
| `product_url` | `https://www.amazon.com/dp/B0CX23V2ZK` |
| `raw` | JSON gốc trả về từ Apify actor |

---

## 4. product-extractor-service — trích xuất sản phẩm từ landing page quảng cáo

- **Crawl gì:** landing page (`link_url`) của từng ad crawl được từ `fb-ads-service`, sau đó dùng AI trích xuất thông tin sản phẩm có cấu trúc (tên, giá, currency, SKU...).
- **Tool/API dùng:**
  - [`crawl4ai-service`](../crawl4ai-service) (Python, dùng thư viện Crawl4AI) — fetch URL và convert HTML → Markdown.
  - LLM qua [OpenRouter](https://openrouter.ai) — client tại [`pkg/aiproviders`](../pkg/aiproviders) — đọc Markdown và trích xuất field.
- **Cơ chế:** tự động chạy khi có event `crawl.completed.fbads` trên RabbitMQ — không cần gọi API riêng. Với mỗi ad, service này lấy `link_url`, gọi `crawl4ai-service` để lấy Markdown, đưa Markdown cho LLM để trích xuất, rồi lưu vào `products`. Lỗi từng URL (không phải trang sản phẩm, timeout...) được log và bỏ qua, không làm fail cả job.
- **Bảng lưu:** `products` (`db/migrations/000003_products.up.sql`).
- **Dữ liệu mẫu (1 row trong `products`):**

| cột | giá trị mẫu |
|---|---|
| `url` | `https://www.nike.com/launch/t/air-max-2026` |
| `product_name` | `Nike Air Max 2026` |
| `price` | `180.00` |
| `currency` | `USD` |
| `sku` | `AM2026-BLK-42` |
| `raw` | JSON các field khác do LLM trích xuất (màu, size, mô tả ngắn...) |

Truy vấn sản phẩm đã trích xuất theo job:

```sql
SELECT * FROM products WHERE job_id = '<job-id>';
```

---

## 5. niche-research-service — nghiên cứu ngách sản phẩm (demand + seasonality)

- **Crawl gì:** dữ liệu nhu cầu tìm kiếm (search volume) và tính thời vụ (seasonality) cho một từ khoá/ngách sản phẩm, qua nhiều quốc gia, rồi dùng AI tổng hợp thành nhận định (risk/action).
- **Tool/API dùng:**
  - [SerpApi](https://serpapi.com) (Google Trends) — cùng client với `trend-service`.
  - LLM qua OpenRouter — tổng hợp `ai_summary`, `ai_risks`, `ai_actions`.
- **Cơ chế:** đây là HTTP service độc lập (không qua RabbitMQ), expose endpoint `POST/GET /api/niche/sessions` phục vụ dashboard [`web/`](../web). Gọi `POST` sẽ chạy đồng bộ: lấy trend data → tính điểm nhu cầu/thời vụ → gọi AI tóm tắt → lưu kết quả vào `niche_sessions`.
- **Bảng lưu:** `niche_sessions` (`db/migrations/000004_niche_sessions.up.sql`).
- **Request mẫu:**

```sh
curl -X POST localhost:8891/api/niche/sessions \
  -d '{"raw_keyword": "yoga mat", "country_codes": ["US", "VN"]}'
```

- **Dữ liệu mẫu (response / row trong `niche_sessions`):**

```json
{
  "raw_keyword": "yoga mat",
  "countries": ["US", "VN"],
  "status": "done",
  "step1_score": 78.5,
  "avg_monthly_searches": 40500,
  "demand_type": "seasonal",
  "demand_label": "Nhu cầu theo mùa, tăng mạnh đầu năm",
  "peak_months": ["January", "February"],
  "low_months": ["November", "December"],
  "trend_direction": "up",
  "volatility": "medium",
  "fluctuation_ratio": 1.8,
  "ai_summary": "Nhu cầu 'yoga mat' tăng mạnh dịp năm mới do xu hướng 'New Year resolution', ổn định quanh năm với biên độ vừa phải.",
  "ai_risks": ["Cạnh tranh cao ở phân khúc giá thấp", "Phụ thuộc mùa vụ Q1"],
  "ai_actions": ["Đẩy quảng cáo trước Tết dương lịch 2-3 tuần", "Bundle với dây kháng lực để tăng AOV"]
}
```

---

## 6. china1688-service — 1688 Wholesale Product

- **Crawl gì:** dữ liệu sản phẩm bán buôn trên 1688.com — tìm theo **từ khoá** (tiếng Trung/Anh) hoặc crawl trực tiếp theo **offer ID**.
- **Tool/API dùng:** Apify actor [`zen-studio/1688-wholesale-scraper`](https://apify.com/zen-studio/1688-wholesale-scraper) — cùng client [`pkg/apify`](../pkg/apify).
- **Cơ chế:** `api-gateway` → RabbitMQ → `china1688-service` (gRPC + worker, proto tại [`rpc/china1688`](../rpc/china1688)) gọi Apify, lưu vào `china1688_raw`.
- **Bảng lưu:** `china1688_raw` (`db/migrations/000006_china1688.up.sql`).
- **Request mẫu (tìm theo từ khoá):**

```sh
curl -X POST localhost:8888/jobs/china1688 \
  -d '{"keywords": ["蓝牙耳机"]}'
```

- **Request mẫu (crawl theo offer ID cụ thể):**

```sh
curl -X POST localhost:8888/jobs/china1688 \
  -d '{"offer_ids": ["123456789"]}'
```

- **Dữ liệu mẫu (1 row trong `china1688_raw`):**

| cột | giá trị mẫu |
|---|---|
| `offer_id` | `123456789` |
| `title` | `蓝牙耳机批发 无线耳机` |
| `price_min` | `12.50` |
| `price_max` | `18.00` |
| `currency` | `CNY` |
| `moq` | `50` |
| `image_url` | `https://cbu01.alicdn.com/img/....jpg` |
| `supplier_name` | `深圳市XX电子有限公司` |
| `supplier_province` | `广东` |
| `detail_url` | `https://detail.1688.com/offer/123456789.html` |
| `raw` | JSON gốc trả về từ Apify actor |

---

## 7. reddit-service — Reddit Post & Top Comments

- **Crawl gì:** tìm theo **từ khoá**, lấy **top 3 bài viết** (sort theo score), và với mỗi bài lấy **100 comment có upvote cao nhất**.
- **Tool/API dùng:** Apify actor [`harshmaur/reddit-scraper`](https://apify.com/harshmaur/reddit-scraper) — client riêng tại [`pkg/apify/reddit.go`](../pkg/apify/reddit.go).
- **Cơ chế:** `api-gateway` → RabbitMQ → `reddit-service` (gRPC + worker, proto tại [`rpc/reddit`](../rpc/reddit)) gọi Apify **1 lần duy nhất** (search + crawl comment của cả 3 post trong cùng 1 run actor, buffer comment gấp 3 lần cần thiết) → tự sort theo `score` giảm dần ở code và cắt lấy top 100/post trước khi lưu — actor không đảm bảo thứ tự trả về đã sort theo score.
- **Bảng lưu:** `reddit_posts_raw` + `reddit_comments_raw` (`db/migrations/000007_reddit.up.sql`).
- **Request mẫu:**

```sh
curl -X POST localhost:8888/jobs/reddit \
  -d '{"keyword": "wireless earbuds"}'
```

- **Dữ liệu mẫu (1 row trong `reddit_posts_raw`):**

| cột | giá trị mẫu |
|---|---|
| `post_id` | `t3_tq4ctx` |
| `title` | `Wired earphones are superior to wireless headphones` |
| `url` | `https://www.reddit.com/r/unpopularopinion/comments/tq4ctx/...` |
| `community_name` | `r/unpopularopinion` |
| `upvotes` | `33688` |
| `num_comments` | `3110` |
| `raw` | JSON gốc trả về từ Apify actor |

- **Dữ liệu mẫu (1 row trong `reddit_comments_raw`):**

| cột | giá trị mẫu |
|---|---|
| `post_id` | `t3_tq4ctx` |
| `comment_id` | `i2eycpn` |
| `username` | `glxssz` |
| `body` | `True. I have very curly hair and somehow wireless ones get caught in it...` |
| `upvotes` | `3476` |
| `raw` | JSON gốc trả về từ Apify actor |

- **Chi phí:** dùng Apify actor tính phí theo kết quả (pay-per-result). Job thật (3 post + buffer 300 comment/post để đảm bảo sort đúng) tốn khoảng **$1.6/lần chạy** — xem thêm [reddit-api-service](#8-reddit-api-service--reddit-post--top-comments-qua-oauth-mi%E1%BB%85n-ph%C3%AD) nếu cần phương án miễn phí.

---

## 8. reddit-api-service — Reddit Post & Top Comments qua OAuth (miễn phí)

- **Crawl gì:** giống hệt `reddit-service` (top 3 bài theo từ khoá + top 100 comment/bài theo upvote), nhưng gọi thẳng **Reddit API chính thức** thay vì Apify.
- **Tool/API dùng:** Reddit OAuth API (`oauth.reddit.com`), xác thực bằng grant `client_credentials` (app-only, không cần đăng nhập tài khoản Reddit) — client tại [`pkg/redditapi`](../pkg/redditapi).
- **Cơ chế:** `api-gateway` → RabbitMQ (`reddit-api.jobs`) → `reddit-api-service` (gRPC + worker, proto tại [`rpc/redditapi`](../rpc/redditapi)) gọi `GET /search` để lấy top post, rồi `GET /comments/{id}` cho từng post (kèm flatten đệ quy các reply lồng nhau), tự sort theo `score` giảm dần và cắt top 100 — **không giới hạn buffer vì API này miễn phí**, không tính phí theo kết quả như Apify.
- **Giới hạn:** endpoint `/comments` không tự "load more" cho các nhánh reply bị Reddit ẩn dưới node `more` — comment ẩn sâu trong thread rất dài (nghìn comment) có thể bị bỏ qua. Rate limit ~100 request/phút/app (đủ dùng, mỗi job chỉ tốn 4 request).
- **Bảng lưu:** dùng chung `reddit_posts_raw` + `reddit_comments_raw` với `reddit-service` (phân biệt qua `jobs.type = 'reddit_api'`).
- **Cấu hình:** cần `REDDIT_CLIENT_ID`/`REDDIT_CLIENT_SECRET` (tạo app loại "script" tại [reddit.com/prefs/apps](https://www.reddit.com/prefs/apps)) và `REDDIT_USER_AGENT`.
- **Request mẫu:**

```sh
curl -X POST localhost:8888/jobs/reddit-api \
  -d '{"keyword": "wireless earbuds"}'
```

---

## 9. crawl4ai-service — hạ tầng crawl HTML → Markdown

- Không phải service nghiệp vụ, mà là **tool nội bộ** dùng chung: nhận một URL, trả về nội dung trang dạng Markdown sạch (loại bỏ nav/footer/script).
- Được `product-extractor-service` gọi qua HTTP nội bộ; xem client tại [`pkg/crawl4ai`](../pkg/crawl4ai).

---

## Tổng hợp nguồn dữ liệu

| Service | Nguồn crawl | Tool/API | Bảng lưu |
|---|---|---|---|
| trend-service | Google Trends | SerpApi | `trend_raw` |
| fb-ads-service | Facebook Ads Library | Apify | `fbads_raw` |
| amazon-service | Amazon product | Apify | `amazon_raw` |
| china1688-service | 1688 wholesale product | Apify | `china1688_raw` |
| reddit-service | Reddit post + top comments | Apify | `reddit_posts_raw`, `reddit_comments_raw` |
| reddit-api-service | Reddit post + top comments | Reddit OAuth API (miễn phí) | `reddit_posts_raw`, `reddit_comments_raw` |
| product-extractor-service | Landing page của ad Facebook | crawl4ai-service + OpenRouter LLM | `products` |
| niche-research-service | Google Trends (theo nhiều quốc gia) | SerpApi + OpenRouter LLM | `niche_sessions` |
| ai-service | (không crawl) tóm tắt insight từ `trend_raw`/`fbads_raw` | OpenRouter LLM | `ai_results` |
