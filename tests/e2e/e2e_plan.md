# E2E Test Plan — Mode A (memory strategy)

Version: 2026-02-24

Purpose
- End-to-end test plan. Verify ingestion, in-memory buffering, flush-to-ClickHouse, deduplication, metrics, backpressure, health, and graceful shutdown.

Environment & prereqs
- Docker Compose (root `docker-compose.yml`) — default profile runs ingestor + ClickHouse in memory strategy.
- Ensure `EVENTMETRICS_BUFFER_STRATEGY=memory` (default) and ClickHouse reachable.
- Ports: ingestor HTTP 8080, ClickHouse 9000/8123.
- Test runner should have Docker available and network access to container ports.

Timing and wait windows
- Flush interval ≈ 100ms. Allow up to 5s for accepted events to be persisted and visible to /metrics.
- Query timeout: 30s. Poll /metrics with exponential backoff up to 5s for assertions.

High-level test cases

1) Happy-path single event ingestion
- POST /events with a valid event.
- Assert: 202 Accepted JSON `{"status":"accepted"}`.
- Wait up to 5s, then GET /metrics?event_name=<name>&from=<earlier>&to=<later>.
- Expect total_count = 1, unique_count = 1.

2) Validation failures
- POST /events missing required fields or with invalid timestamp (≤0 or > maxFuture).
- Expect 400 with code `VALIDATION_ERROR` and descriptive message.

3) Deduplication
- POST the same canonical event twice.
- Expect 202 on publish(s). After persistence, GET /metrics → total_count = 1, unique_count = 1.

4) Bulk ingestion (bonus)
- POST /events/bulk with mixed valid & invalid events (e.g., 10 events, 2 invalid).
- Expect 202 with accepted/rejected summary per API contract.
- Verify accepted events persisted via /metrics.

5) Metrics grouping & unique counts
- Ingest multiple events across `channel` and `user_id`.
- GET /metrics with `group_by=channel` and `group_by=hour`.
- Expect groups reflect counts and unique_count per group.

6) Backpressure / bounded buffer behaviour (I5)
- Configure small buffer capacity via env (EVENTMETRICS_BUFFER_CAPACITY).
- Rapidly POST events until buffer capacity reached.
- Expect subsequent POSTs return 503 with code `PUBLISH_FAILED`.

7) Health endpoint
- GET /health should return `status: "healthy"` and `clickhouse: "connected"` when ClickHouse reachable.
- If ClickHouse is stopped, health should become `degraded` but still respond (200) per TDD assumption.

8) Graceful shutdown
- While ingesting, send SIGTERM to ingestor container/process.
- Expect server to drain in-flight batch and flush remaining events to ClickHouse within shutdown timeout (default 10s).
- After restart, verify persisted events are present via /metrics.

Test data & fixtures
- Use deterministic test event factory: unique event_name per test to avoid interference.
- Use `timestamp` anchored to now (NormalizeTimestamp handles seconds vs ms).
- Edge cases: empty strings, max-length fields, large metadata within 1MB limit.

Automation notes
- Implement tests as Go E2E tests (e.g., `e2e/memory_e2e_test.go`) or scripts:
  - Start: `docker compose up -d`
  - Wait for ClickHouse readiness (poll TCP/HTTP).
  - Run HTTP client tests (POST/GET).
  - Collect logs on failure: `docker compose logs ingestor clickhouse`.
  - Tear down: `docker compose down -v`
- Makefile snippet:

```make
e2e-memory:
	docker compose up -d
	go test ./e2e -run TestE2E_Memory -v
	docker compose down -v
```

Pass / fail criteria
- PASS: All core tests (1,2,3,5,7,8) pass. Dedup and accepted→persisted invariants validated.
- FAIL: Data loss (accepted but never persisted after recovery), incorrect dedup (counts > expected), or basic valid ingestion returning non-202.

Artifacts and debugging
- On failures produce:
  - ClickHouse query dump (SELECT ... FINAL for test event_name)
  - ingestor logs (`docker compose logs ingestor`)
  - test HTTP traces (requests/responses)

Notes & next steps
- This plan covers Mode A only. Later extend to additional strategies and add strategy-specific tests as needed.

