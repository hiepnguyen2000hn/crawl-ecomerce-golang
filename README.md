# crawl-ecomerce-golang

A small e-commerce trend-crawling platform built as three Go microservices:

- **trend-service** — a gRPC service backed by a RabbitMQ worker that fetches
  Google Trends data via SerpApi and persists it.
- **ai-service** — a RabbitMQ worker that takes the crawled trend data and
  asks an OpenRouter-hosted LLM to summarize the key insight.
- **api-gateway** — a REST API (go-zero) that lets clients create trend jobs
  and poll their status/results.

The services communicate through Postgres (job/result storage) and RabbitMQ
(job queue and completion events). See the full design spec at
[`docs/superpowers/specs/2026-09-16-crawl-ecommerce-platform-design.md`](docs/superpowers/specs/2026-09-16-crawl-ecommerce-platform-design.md).

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
```

Get a SerpApi key from [serpapi.com](https://serpapi.com) and an OpenRouter
key from [openrouter.ai](https://openrouter.ai). Docker Compose automatically
loads `.env` from the repo root and uses it to fill in the
`SERPAPI_API_KEY`/`OPENROUTER_API_KEY` environment variables passed into the
`trend-service` and `ai-service` containers.

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
