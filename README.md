# crawl-ecomerce-golang

A small e-commerce trend-crawling platform built as four Go microservices:

- **trend-service** — a gRPC service backed by a RabbitMQ worker that fetches
  Google Trends data via SerpApi and persists it.
- **fb-ads-service** — a gRPC service backed by a RabbitMQ worker that fetches
  Facebook Ads Library data via Apify and persists it.
- **ai-service** — a RabbitMQ worker that takes the crawled trend or fbads
  data and asks an OpenRouter-hosted LLM to summarize the key insight.
- **amazon-service** — a gRPC service backed by a RabbitMQ worker that
  fetches Amazon product data (search by keyword or crawl specific product
  URLs/ASINs) via an Apify actor and persists it.
- **api-gateway** — a REST API (go-zero) that lets clients create trend,
  fbads, or amazon jobs and poll their status/results.

The services communicate through Postgres (job/result storage) and RabbitMQ
(job queue and completion events). See the full design spec at
[`docs/superpowers/specs/2026-09-16-crawl-ecommerce-platform-design.md`](docs/superpowers/specs/2026-09-16-crawl-ecommerce-platform-design.md).

For a per-service breakdown of what each crawler fetches, which tool/API it
uses, and example data, see [`docs/SERVICES.md`](docs/SERVICES.md) (mô tả chi
tiết các service crawl kèm ví dụ dữ liệu, bằng tiếng Việt).

### Product-extraction enrichment pipeline

Two additional services enrich each Facebook Ads crawl with structured
product data extracted from the ad's destination page:

- **crawl4ai-service** — a Python HTTP service (Crawl4AI) that fetches a URL
  and converts the page to Markdown.
- **product-extractor-service** — a RabbitMQ worker that consumes the
  `crawl.completed.fbads` event, crawls each ad's `link_url` via
  `crawl4ai-service`, asks an OpenRouter-hosted LLM to extract structured
  product fields (name, price, currency, SKU, etc.) from the page Markdown,
  and persists the results to the `products` table.

This pipeline runs automatically — there is no separate trigger. Any
`POST /jobs/fbads` job that completes crawling will kick it off in the
background. Per-URL crawl/extraction failures are tolerated and logged (a
destination link may not be a product page), so a job with zero enriched
products is not necessarily a bug.

To inspect the extracted products for a given job:

```sql
SELECT * FROM products WHERE job_id = '<job-id>';
```

## API

Two crawl paths are exposed by `api-gateway`:

- `POST /jobs/trend` — `{"keyword": "golang"}` — `GET /jobs/trend/{id}`
- `POST /jobs/fbads` — `{"query": "nike", "country": "US"}` — `GET /jobs/fbads/{id}`
- `POST /jobs/amazon` — `{"keyword": "wireless earbuds", "country": "US"}` or
  `{"urls": ["https://www.amazon.com/dp/B0..."]}` — `GET /jobs/amazon/{id}`

## Prerequisites

- Docker and Docker Compose

## Setup

Copy the example env file and fill in your API keys:

```sh
cp .env.example .env
```

Edit `.env` and set:

```
SERPAPI_API_KEY=your-serpapi-key
OPENROUTER_API_KEY=your-openrouter-key
APIFY_API_TOKEN=your-apify-token
```

Get a SerpApi key from [serpapi.com](https://serpapi.com), an OpenRouter key
from [openrouter.ai](https://openrouter.ai), and an Apify token from
[apify.com](https://apify.com). Docker Compose automatically loads `.env`
from the repo root and uses it to fill in the
`SERPAPI_API_KEY`/`OPENROUTER_API_KEY`/`APIFY_API_TOKEN` environment
variables passed into the `trend-service`, `ai-service`, `fb-ads-service`,
and `amazon-service` containers.

## Run

```sh
docker compose up -d --build
```

Database migrations run automatically on startup via the `migrate` one-shot
service in `docker-compose.yml` — no manual migration step is required.

## Test

Once the stack is up and healthy, run the end-to-end smoke test:

```sh
./scripts/smoke_test.sh
```

It creates a trend job for the keyword `golang` by default and polls until it
completes or fails. Pass `--keyword` to use a different keyword:

```sh
./scripts/smoke_test.sh --keyword "sneakers"
```

Pass `--path fbads` to exercise the Facebook Ads Library crawl path instead
(the `--keyword` value is sent as the `query` field). Pass `--country` to
override the default `US` (accepts any Apify-supported ISO country code, or
`ALL`):

```sh
./scripts/smoke_test.sh --path fbads --keyword "nike"
./scripts/smoke_test.sh --path fbads --keyword "áo" --country "VN"
```

### Local dev note

This development environment had a port conflict with the Docker defaults, so
`docker-compose.yml` maps the host side of Postgres/RabbitMQ to non-default
ports: `5433` (Postgres), `5673` (RabbitMQ AMQP), and `15673` (RabbitMQ
management UI), instead of the Docker defaults `5432`/`5672`/`15672`. If
you're running this on a clean host with no port conflicts, feel free to
adjust `docker-compose.yml` back to the standard ports.

### Running package tests

```sh
go test ./...
```
