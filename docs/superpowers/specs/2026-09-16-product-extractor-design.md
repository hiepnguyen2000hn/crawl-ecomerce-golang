# Product Extractor Pipeline Design

Date: 2026-09-16

## Mục tiêu

Sau khi `fb-ads-service` crawl xong quảng cáo Facebook (đã có `link_url` trỏ tới trang bán hàng), thêm 1 nhánh xử lý song song để: crawl trang đích bằng crawl4ai → convert HTML sang Markdown → dùng AI (structured output) trích xuất tên sản phẩm/giá/SKU → lưu vào bảng `products`.

Nhánh này **độc lập với `ai-service`** (không đổi `jobs.status` state machine hiện có) — chạy song song như 1 consumer bổ sung trên cùng event `crawl.completed.fbads`.

## Quyết định kiến trúc (đã chốt qua brainstorm)

1. Vì `crawl4ai` là thư viện Python (không có SDK Go), tách riêng 1 microservice Python nhỏ (`crawl4ai-service`) chỉ làm 1 việc: nhận URL, trả về Markdown. Toàn bộ orchestration (đọc DB, gọi AI, lưu kết quả) vẫn ở Go, giữ đúng kiến trúc go-zero + RabbitMQ hiện có.
2. Trích xuất sản phẩm dùng **structured output** (`response_format: json_schema`) của OpenRouter — đã spike xác nhận model free `nvidia/nemotron-3-super-120b-a12b:free` hỗ trợ tốt, trả đúng JSON theo schema, không cần fallback parse thủ công.
3. `products` là bảng riêng (1-nhiều với `fbads_raw`), vì 1 trang đích có thể chứa nhiều sản phẩm.

## 1. Service breakdown (bổ sung)

| Service | Vai trò | Giao tiếp |
|---|---|---|
| **crawl4ai-service** (mới, Python) | Nhận URL, dùng `crawl4ai` (`AsyncWebCrawler`, render JS) crawl và convert sang Markdown, trả về | REST nội bộ (`POST /crawl`), không expose ra ngoài, không cần RabbitMQ/Postgres |
| **product-extractor-service** (mới, Go) | Consume `crawl.completed.fbads`, với mỗi `link_url` distinct của job: gọi crawl4ai-service lấy markdown → gọi OpenRouter (structured output) trích sản phẩm → lưu `products` | RabbitMQ consumer (giống pattern `ai-service`), gọi HTTP ra `crawl4ai-service` và OpenRouter |

`fb-ads-service` và `ai-service` **không đổi** — `product-extractor-service` chỉ là 1 binding bổ sung trên cùng routing key, không ảnh hưởng 2 service kia.

## 2. Data flow

```
fb-ads-service (đã có, không đổi)
       → crawl xong, lưu fbads_raw (có link_url mỗi ad)
       → publish "crawl.completed.fbads" {"job_id":...}   (event đã có sẵn)

product-extractor-service (consumer mới, bind cùng routing key)
       → SELECT DISTINCT link_url FROM fbads_raw WHERE job_id = $1 AND link_url != ''
       → với mỗi url:
           → POST crawl4ai-service/crawl {"url": url} → {"markdown": "...", "success": true}
           → (nếu crawl fail: log, bỏ qua url đó, không fail cả job)
           → gọi OpenRouter CompleteJSON(markdown, product_schema) → có thể trả 0..N sản phẩm
           → INSERT vào products cho mỗi sản phẩm trích được (job_id, ad_id, url, product_name, price, currency, sku, raw)
       → không đổi jobs.status (ai-service vẫn là chủ của state machine này)

crawl4ai-service (Python, stateless)
       → nhận {"url": "..."}
       → AsyncWebCrawler().arun(url) → CrawlResult.markdown
       → trả {"markdown": "...", "success": true} hoặc {"success": false, "error": "..."}
```

## 3. Data model (PostgreSQL — bảng mới)

```sql
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

`ad_id` nullable vì 1 url có thể được nhiều ad trỏ tới (dedupe theo url trong 1 job) — không map 1-1 tuyệt đối với 1 ad cụ thể; lưu `ad_id` của ad đầu tiên gặp url đó để truy vết.

## 4. `pkg/crawl4ai` — client Go mới (mirror `pkg/apify`)

```go
type CrawlResult struct {
    Markdown string
    Success  bool
    Error    string
}

type Client interface {
    Crawl(ctx context.Context, url string) (CrawlResult, error)
}

func NewHTTPClient(baseURL string, httpClient *http.Client) *HTTPClient
```

`baseURL` mặc định `http://crawl4ai-service:8000` (container-internal) / `http://localhost:8000` (host dev).

## 5. Mở rộng `pkg/aiproviders`

Thêm method mới vào `Provider` interface (không đổi `Complete` hiện có, chỉ thêm):

```go
type Result struct {
    Model   string
    Content string
}

// CompleteJSON gọi model với response_format: json_schema, trả JSON thô đã
// được ép đúng schema — dùng cho tác vụ trích xuất có cấu trúc.
type Provider interface {
    Complete(ctx context.Context, prompt string) (Result, error)
    CompleteJSON(ctx context.Context, prompt string, schemaName string, schema json.RawMessage) (json.RawMessage, error)
}
```

`OpenRouter.CompleteJSON` gửi `response_format: {"type":"json_schema","json_schema":{"name":schemaName,"strict":true,"schema":schema}}` (đã verify format này hoạt động qua spike thật).

Product schema dùng trong `product-extractor-service`:
```json
{
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
}
```
(Mảng `products` cho phép 0..N sản phẩm/trang, và `strict: true` không cho phép field ngoài schema.)

## 6. Tech stack / repo layout (bổ sung)

```
crawl4ai-service/
  main.py
  requirements.txt
  Dockerfile
pkg/crawl4ai/{client.go, client_test.go}
db/migrations/000003_products.up.sql / .down.sql
cmd/product-extractor-service/
  etc/{extractor.yaml, extractor.docker.yaml}
  internal/worker/{consumer.go, consumer_test.go}
  main.go
  Dockerfile
```

`docker-compose.yml` thêm 2 service: `crawl4ai-service`, `product-extractor-service`.

## Out of scope (giữ nguyên/tiếp tục deferred từ spec trước)

- REST endpoint để client xem `products` qua api-gateway (có thể thêm sau, không cần cho pipeline chạy được)
- proxy-broker, TokenRouter, scheduler/cron
- Retry riêng cho crawl4ai lỗi từng URL (lỗi 1 URL chỉ log & bỏ qua, không chặn các URL khác trong cùng job)
