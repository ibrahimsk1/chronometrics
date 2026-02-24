# E2E Test Plan — Mode A (memory strategy)

Version: 2026-02-24

## Purpose
End-to-end test plan. Verify ingestion, in-memory buffering, flush-to-ClickHouse,
deduplication, metrics, backpressure, health, and graceful shutdown.

---

## Environment & Prerequisites
- Docker Compose (root `docker-compose.yml`) — default profile runs ingestor + ClickHouse
  in memory strategy.
- Ensure `CHRONOMETRICS_BUFFER_STRATEGY=memory` (default) and ClickHouse reachable.
- Ports: ingestor HTTP 8080, ClickHouse 9000 / 8123.
- Test runner must have Docker available and network access to container ports.
- Override defaults via env vars:
  - `E2E_BASE_URL` (default `http://localhost:8080`)
  - `E2E_CLICKHOUSE_URL` (default `http://localhost:8123`)
  - `E2E_CLICKHOUSE_DB` (default `default`)

---

## Timing & Wait Windows
- Flush interval ≈ 100 ms. Allow up to 5 s for accepted events to be persisted and
  visible to `/metrics`.
- Tests poll `/metrics` with 200 ms sleep, up to a 5 s deadline.
- Query timeout: 30 s.

---

## Running the Tests & Exporting a Report

```bash
make e2e                        # default 120 s timeout
make e2e E2E_TIMEOUT=180s       # override timeout
make e2e E2E_REPORT_DIR=out/e2e # override report directory
```

### What `make e2e` does
1. `docker compose up -d` — starts ingestor + ClickHouse.
2. Waits 5 s for services to be ready.
3. Runs `go test ./tests/e2e/ -run TestE2E_ -v -count=1 -timeout <E2E_TIMEOUT>` and
   streams output to stdout **and** a timestamped text log:
   `reports/e2e/run_YYYYMMDD_HHMMSS.log`
4. Runs a second pass with `-json` to produce a machine-readable report:
   `reports/e2e/results.json`
5. Prints a PASSED / FAILED summary with the report path.
6. Exits with the `go test` exit code so CI detects failures.

### Report files
| File | Format | Contents |
|------|--------|----------|
| `reports/e2e/run_*.log` | Plain text | Full verbose test output with timing |
| `reports/e2e/results.json` | NDJSON (go test -json) | Machine-readable pass/fail per test |

> The JSON format is the standard Go test-event stream. It can be consumed by
> `gotestsum`, `go-junit-report`, or any CI tool that understands the format.

---

## Test Cases

| # | Test function | Status | Description |
|---|---------------|--------|-------------|
| 1 | `TestE2E_HappyPath_SingleEventIngestion` | ✅ implemented | POST valid event → 202; poll `/metrics` until total_count=1, unique_count=1 |
| 2 | `TestE2E_ValidationFailures` | ✅ implemented | Missing `event_name` and `timestamp≤0` each return 400 |
| 3 | `TestE2E_Deduplication` | ✅ implemented | Same canonical event posted twice → single persisted row |
| 4 | `TestE2E_BulkIngestion` | ✅ implemented | `/events/bulk` with 3 valid + 1 invalid → 202; 3 rows persisted |
| 5 | `TestE2E_MetricsGrouping` | ✅ implemented | `group_by=channel` returns correct per-channel counts |
| 5b | `TestE2E_ThroughputLoad_20k` | ✅ implemented | 20 workers × 1 000 events/request over 10 s; asserts ≥ 15 000 events/s and ≤ 5 % drop |
| 6 | Backpressure (bounded buffer) | ⬜ pending | `CHRONOMETRICS_BUFFER_CAPACITY` set small; rapid POSTs expect 503 `PUBLISH_FAILED` |
| 7 | Health endpoint | ⬜ pending | `GET /health` → `status:"healthy"`, `clickhouse:"connected"`; degraded when CH stopped |
| 8 | Graceful shutdown | ⬜ pending | SIGTERM during ingestion; events flushed within shutdown timeout; verified after restart |

---

## Test Data & Fixtures
- Each test uses a unique `event_name` seeded with `time.Now().UnixNano()` to avoid
  inter-test interference without truncating ClickHouse between runs.
- `timestamp` is anchored to `time.Now().UnixMilli()` so `NormalizeTimestamp` accepts it.
- Cleanup: rows are deleted via ClickHouse HTTP ALTER … DELETE after each test (best-effort).

---

## Automation Notes

```make
# Run E2E tests and export reports
e2e:
	@mkdir -p $(E2E_REPORT_DIR)
	docker compose up -d
	@sleep 5
	@REPORT=$(E2E_REPORT_DIR)/run_$$(date +%Y%m%d_%H%M%S).log; \
	go test ./tests/e2e/ -run TestE2E_ -v -count=1 -timeout $(E2E_TIMEOUT) \
		2>&1 | tee "$$REPORT"; \
	EXIT=$${PIPESTATUS[0]}; \
	go test ./tests/e2e/ -run TestE2E_ -count=1 -timeout $(E2E_TIMEOUT) -json \
		> $(E2E_REPORT_DIR)/results.json 2>&1 || true; \
	exit $$EXIT
```

On failure, collect logs:

```bash
docker compose logs ingestor clickhouse
```

---

## Pass / Fail Criteria
- **PASS**: All implemented tests (1–5, 5b) pass. Dedup and accepted→persisted
  invariants validated. Throughput ≥ 15 000 events/s with ≤ 5 % drop.
- **FAIL**: Data loss (accepted but not persisted after flush window), incorrect dedup
  (counts > expected), valid ingestion returning non-202, or throughput below threshold.

---

## Artifacts & Debugging
On failure produce:
- `reports/e2e/run_*.log` — full test output
- `reports/e2e/results.json` — per-test JSON events
- ClickHouse query dump: `SELECT * FROM default.events FINAL WHERE event_name = '<name>'`
- Ingestor logs: `docker compose logs ingestor`

---

## Notes & Next Steps
- This plan covers Mode A (memory strategy) only.
- Pending tests (6, 7, 8) should be added as the relevant infrastructure stabilises.
- Later extend with strategy-specific tests if additional buffer strategies are added.
