# Crawl E-commerce Platform — Design

Date: 2026-09-16

## Mục tiêu

Xây dựng backend microservice bằng Golang để:
1. Crawl dữ liệu quảng cáo Facebook (qua Apify actor cho FB Ads Library).
2. Crawl dữ liệu Google Trends (qua SerpApi).
3. Đưa dữ liệu crawl được qua các AI provider (OpenRouter, TokenRouter) để phân tích hoặc sinh nội dung, tuỳ loại job.
4. Lưu toàn bộ dữ liệu vào PostgreSQL.
5. Hỗ trợ proxy xoay (rotating proxy) thông qua nhà cung cấp proxy có sẵn (Bright Data/Oxylabs/Smartproxy) cho các lần gọi HTTP trực tiếp trong tương lai.

Framework: **go-zero** (REST + gRPC codegen, rate-limit/circuit-breaker tích hợp sẵn).
Giao tiếp nội bộ: **gRPC**. Job bất đồng bộ (crawl, AI): **RabbitMQ**.
Trigger job: chỉ qua API gọi thủ công (chưa cần scheduler/cron ở giai đoạn này).
Triển khai: **Docker Compose** (single VPS/local, chưa cần Kubernetes).

## 1. Service breakdown

| Service | Vai trò | Giao tiếp |
|---|---|---|
| **api-gateway** | REST entrypoint, nhận request từ client, validate, publish job vào RabbitMQ, trả job_id; expose endpoint query trạng thái/kết quả | REST (go-zero `.api`) ra ngoài, gRPC nội bộ tới các service khác cho query |
| **fb-ads-service** | Consume job từ queue, gọi Apify actor (FB Ads Library), parse & lưu Postgres | RabbitMQ consumer + gRPC server (query kết quả) |
| **trend-service** | Consume job từ queue, gọi SerpApi (Google Trends), parse & lưu Postgres | RabbitMQ consumer + gRPC server |
| **ai-service** | Consume event "crawl.completed", gọi OpenRouter/TokenRouter (analysis hoặc generate content tùy job type), lưu kết quả | RabbitMQ consumer + gRPC server |
| **proxy-broker** | Cấp proxy endpoint/credential xoay từ 1 provider cho service nào cần gọi HTTP trực tiếp (không qua Apify/SerpApi) | gRPC nội bộ, `GetProxy(target)` |

Ghi chú: Apify và SerpApi tự quản lý proxy phía họ. `proxy-broker` phục vụ các lần gọi HTTP trực tiếp trong tương lai — dựng sẵn interface nhưng chưa bắt buộc dùng ở fb-ads-service/trend-service trong MVP.

## 2. Data flow / job lifecycle

```
Client → POST /jobs (api-gateway)
       → publish message vào RabbitMQ exchange "crawl" (routing key: fbads.jobs | trend.jobs)
       → trả về job_id (202 Accepted)

fb-ads-service / trend-service (consumer)
       → nhận job, gọi Apify actor / SerpApi
       → lưu raw + normalized data vào Postgres (status: crawled)
       → publish event "crawl.completed" (routing key: crawl.completed.fbads | crawl.completed.trend)

ai-service (consumer, bind cả 2 routing key trên)
       → nhận event, load data từ Postgres theo job_id
       → gọi OpenRouter/TokenRouter (chọn model/provider theo config job_type: "analysis" | "generate")
       → lưu kết quả AI vào Postgres, update job status: done

Client → GET /jobs/{id} (api-gateway, gRPC tới service tương ứng) → poll trạng thái + kết quả
```

Retry: mỗi queue chính có dead-letter queue (DLQ) riêng, retry tối đa N lần (config), sau đó vào DLQ để xử lý thủ công.

## 3. Data model (PostgreSQL, đơn giản hoá cho MVP)

- `jobs` (id, type[fbads|trend], status[pending|crawled|ai_processing|done|failed], params jsonb, created_at, updated_at)
- `fbads_raw` (id, job_id, page_id, ad_data jsonb, fetched_at)
- `trend_raw` (id, job_id, keyword, trend_data jsonb, fetched_at)
- `ai_results` (id, job_id, provider, model, prompt_type[analysis|generate], input_ref, output jsonb, created_at)
- `proxy_usage` (id, provider, used_at, target, success bool) — log nhẹ, optional, dùng khi proxy-broker được gọi thật

Mỗi service sở hữu schema/migration riêng cho bảng của mình (`golang-migrate`), tránh service khác trực tiếp ghi vào bảng không thuộc mình.

## 4. Tech stack / repo layout

- **go-zero**: `api-gateway` dùng `.api` (REST); các service nội bộ dùng `.proto` (gRPC), codegen bằng `goctl`.
- **RabbitMQ**: 1 exchange (`crawl`, type topic), nhiều routing key như mô tả ở mục 2, mỗi queue chính có DLQ tương ứng.
- **PostgreSQL**: mỗi service quản lý schema/migration riêng.
- **Docker Compose services**: postgres, rabbitmq, api-gateway, fb-ads-service, trend-service, ai-service, proxy-broker.

Monorepo layout:
```
/cmd/{api-gateway,fb-ads-service,trend-service,ai-service,proxy-broker}
/api  (.api, .proto files)
/internal/{service}/{logic,svc,model}
/pkg/{rabbitmq,proxyclient,aiproviders}
/deploy/docker-compose.yml
```

- `pkg/aiproviders`: abstraction chung cho OpenRouter/TokenRouter (interface `Complete(ctx, prompt, opts) (Result, error)`), cho phép ai-service chọn provider theo config job mà không đổi logic gọi.
- `pkg/proxyclient`: wrap gateway xoay proxy của 1 provider (Bright Data/Oxylabs/Smartproxy) thành `http.RoundTripper`, dùng khi service cần gọi HTTP trực tiếp ra ngoài.
- `pkg/rabbitmq`: publisher/consumer helper dùng chung, có retry + DLQ setup.

## Out of scope (MVP này)

- Scheduler/cron tự động trigger crawl (chỉ trigger qua API thủ công).
- Tự xây proxy pool (chỉ tích hợp provider có sẵn).
- Kubernetes deployment.
- Dashboard/UI cho client — chỉ có REST API.
