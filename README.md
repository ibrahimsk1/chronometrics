# Chronometrics — Event Ingestor (Insider One Assessment)

Minimal backend service that ingests JSON events and exposes aggregated metrics.

## Quickstart

Docker

- Start services: `make docker-up`
- Stop services: `make docker-down`
- Health check: http://localhost:8080/health
- Run end-to-end tests: `make e2e` (E2E reports and JSON results are written to `reports/e2e/`)

## Assumptions, design decisions, and trade-offs

- See design notes: [docs/system_design.md](docs/system_design.md) and [docs/assumptions.md](docs/assumptions.md).
- See decisions: [docs/decisions/0003-buffer-strategy-in-memory-vs-kafka-vs-wal.md](docs/decisions/0003-buffer-strategy-in-memory-vs-kafka-vs-wal.md)

## API - Example requests

POST /events

Accepts a single event. Example:

curl -sS -X POST "http://localhost:8080/events" -H "Content-Type: application/json" -d @- <<'JSON'
{
  "event_name": "product_view",
  "channel": "web",
  "campaign_id": "cmp_987",
  "user_id": "user_123",
  "timestamp": 1723475612,
  "tags": ["electronics", "homepage", "flash_sale"],
  "metadata": {
    "product_id": "prod-789",
    "price": 129.99,
    "currency": "TRY",
    "referrer": "google"
  }
}
JSON

Response: 202 Accepted on successful enqueue; 400 on validation error; 503 when buffer is full.

GET /metrics

Returns aggregated metrics. Required query parameters: `event_name`, `from` (epoch seconds), `to` (epoch seconds).

Example:

curl -sS "http://localhost:8080/metrics?event_name=product_view&from=1723475600&to=1723479200"

Sample response (illustrative):

{
  "event_name": "product_view",
  "total_count": 12345,
  "unique_users": 6789,
  "breakdown": {
    "by_channel": {
      "web": 10000,
      "mobile": 2345
    }
  }
}

## Next steps / TODOs

- Add structured logs and observability (metrics, tracing) for better monitoring and debugging.
- Add durable buffering (WAL or broker) for zero RPO.
- Add OpenAPI/Swagger documentation.
- Add materialized views or precomputed tables for faster metrics at scale.

