# Chronometrics — Event Ingestor (Insider One Assessment)

Minimal backend service that ingests JSON events and exposes aggregated metrics.

## Quickstart

Create a `.env` file from `.env.example` before running Docker (for example: `cp .env.example .env`).

### Docker

Run the development services and tests:

```bash
# start services
make docker-up

# stop services
make docker-down

# health check
curl -sS http://localhost:8080/health

# run end-to-end tests (reports written to reports/e2e/)
make e2e
```

## Assumptions, design decisions, and trade-offs

- See design notes: [docs/system_design.md](docs/system_design.md) and [docs/assumptions.md](docs/assumptions.md).
- See decisions: [docs/decisions/0003-buffer-strategy-in-memory-vs-kafka-vs-wal.md](docs/decisions/0003-buffer-strategy-in-memory-vs-kafka-vs-wal.md)

## API - Example requests

### POST /events

Accepts a single event. Example:

Note: when using a heredoc with curl, the delimiter must be on its own line — do not place the JSON on the same line as <<'JSON'.

```bash
curl -sS -X POST "http://localhost:8080/events" \
  -H "Content-Type: application/json" \
  -d @- <<'JSON'
{
  "event_name": "product_view",
  "channel": "web",
  "campaign_id": "cmp_987",
  "user_id": "user_123",
  "timestamp": 1771977600,
  "tags": ["electronics", "homepage", "flash_sale"],
  "metadata": {
    "product_id": "prod-789",
    "price": 129.99,
    "currency": "TRY",
    "referrer": "google"
  }
}
JSON
```

Response: 202 Accepted on successful enqueue; 400 on validation error; 503 when buffer is full.

### GET /metrics

Returns aggregated metrics. Required query parameters: `event_name`, `from` (epoch seconds), `to` (epoch seconds).

Example:

```bash
curl -sS "http://localhost:8080/metrics?event_name=product_view&from=1771974000&to=1771977700"
```

Sample response (illustrative):

```json
{"event_name":"product_view","from":1771974000000,"to":1771977700000,"total_count":1,"unique_count":1}
```

## Next steps / TODOs

- Add structured logs and observability (metrics, tracing) for better monitoring and debugging.
- Add durable buffering (WAL or broker) for zero RPO.
- Add OpenAPI/Swagger documentation.
- Add materialized views or precomputed tables for faster metrics at scale.

