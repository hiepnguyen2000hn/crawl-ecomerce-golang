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

## 6. crawl4ai-service — hạ tầng crawl HTML → Markdown

- Không phải service nghiệp vụ, mà là **tool nội bộ** dùng chung: nhận một URL, trả về nội dung trang dạng Markdown sạch (loại bỏ nav/footer/script).
- Được `product-extractor-service` gọi qua HTTP nội bộ; xem client tại [`pkg/crawl4ai`](../pkg/crawl4ai).

---

## Tổng hợp nguồn dữ liệu

| Service | Nguồn crawl | Tool/API | Bảng lưu |
|---|---|---|---|
| trend-service | Google Trends | SerpApi | `trend_raw` |
| fb-ads-service | Facebook Ads Library | Apify | `fbads_raw` |
| amazon-service | Amazon product | Apify | `amazon_raw` |
| product-extractor-service | Landing page của ad Facebook | crawl4ai-service + OpenRouter LLM | `products` |
| niche-research-service | Google Trends (theo nhiều quốc gia) | SerpApi + OpenRouter LLM | `niche_sessions` |
| ai-service | (không crawl) tóm tắt insight từ `trend_raw`/`fbads_raw` | OpenRouter LLM | `ai_results` |
