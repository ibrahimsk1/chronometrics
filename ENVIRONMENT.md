# chronometrics — Environment / .env example

This file contains an example `.env` you can copy to `.env` and edit for local development.
Do NOT commit a `.env` with real secrets. Keep secrets in CI or a secrets manager.

Usage:
- Copy to repo root: `cp ENVIRONMENT.md .env.example && sed -n '1,$p' ENVIRONMENT.md > .env` or manually copy the code block below.
- Docker Compose will automatically load a `.env` file in the compose directory.

Example `.env`
```
# Strategy: memory | nats | nats-embedded
CHRONOMETRICS_BUFFER_STRATEGY=memory

# HTTP server
CHRONOMETRICS_SERVER_PORT=8080
CHRONOMETRICS_SERVER_READ_TIMEOUT=5s
CHRONOMETRICS_SERVER_WRITE_TIMEOUT=10s
CHRONOMETRICS_SERVER_IDLE_TIMEOUT=60s
CHRONOMETRICS_SERVER_BODY_LIMIT=1048576
CHRONOMETRICS_SERVER_SHUTDOWN_TIMEOUT=10s

# In-memory buffer (used when strategy=memory)
CHRONOMETRICS_BUFFER_CAPACITY=1000
CHRONOMETRICS_BUFFER_FLUSH_INTERVAL=100ms
CHRONOMETRICS_BUFFER_FLUSH_BATCH_SIZE=1000
CHRONOMETRICS_BUFFER_FLUSH_TIMEOUT=10s
CHRONOMETRICS_BUFFER_FLUSH_RETRIES=3

# NATS / JetStream (used when strategy=nats or nats-embedded)
# Use a local dev NATS for testing: nats://localhost:4222
CHRONOMETRICS_NATS_URL=nats://localhost:4222
CHRONOMETRICS_NATS_STREAM_NAME=EVENTS
CHRONOMETRICS_NATS_SUBJECT=events.ingest
CHRONOMETRICS_NATS_MAX_BYTES=10737418240
CHRONOMETRICS_NATS_REPLICAS=1
CHRONOMETRICS_NATS_PUBLISH_TIMEOUT=2s

# Consumer (only used by the consumer binary or embedded consumer)
CHRONOMETRICS_CONSUMER_DURABLE_NAME=flusher
CHRONOMETRICS_CONSUMER_ACK_WAIT=30s
CHRONOMETRICS_CONSUMER_MAX_DELIVER=5
CHRONOMETRICS_CONSUMER_MAX_ACK_PENDING=5000
CHRONOMETRICS_CONSUMER_BATCH_SIZE=1000
CHRONOMETRICS_CONSUMER_BATCH_TIMEOUT=100ms
CHRONOMETRICS_CONSUMER_FLUSH_TIMEOUT=10s
CHRONOMETRICS_CONSUMER_HEALTH_PORT=8081

# ClickHouse connection (do not commit production passwords)
CHRONOMETRICS_CLICKHOUSE_ADDR=localhost:9000
CHRONOMETRICS_CLICKHOUSE_DATABASE=default
CHRONOMETRICS_CLICKHOUSE_USERNAME=default
CHRONOMETRICS_CLICKHOUSE_PASSWORD=
CHRONOMETRICS_CLICKHOUSE_QUERY_TIMEOUT=30s

# Validation windows
CHRONOMETRICS_VALIDATION_MAX_FUTURE=24h
CHRONOMETRICS_VALIDATION_MAX_PAST=8760h

# Optional legacy names supported by the code (examples)
# EVENTMETRICS_* variables are still supported for backward compatibility
# BUFFER_STRATEGY=memory
# SERVER_PORT=8080
# NATS_URL=nats://localhost:4222
```

Notes and recommendations
- Do NOT store real credentials in the repository. Use your Git hosting secrets (GitHub Actions Secrets) or a secrets manager for CI/production.
- For local development, create a `.env` from this file and add `.env` to `.gitignore`.
- For Docker Compose testing:
  - Memory mode: `docker compose up` (defaults to memory strategy)
  - NATS embedded: `docker compose --profile nats-single up`
  - Full NATS mode: `docker compose --profile nats up`
- Keep values human-readable (durations like `100ms`, `2s`) — the application parses these.
- If you want to provide a single-machine dev override, create `docker-compose.override.yml` or a `.env.local` and document it in README.

