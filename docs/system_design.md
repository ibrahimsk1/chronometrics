# SYSTEM_DESIGN.md
version: v0.4
last_updated: 2026-02-24
status: final_inmemory_buffer
owner: chronometrics

---

## Design note (2026-02-24)
**Buffering strategy: in-memory bounded buffer.** Chosen from the start for this submission due to the 48-hour implementation constraint — it is the fastest option to implement correctly (~100 LOC, zero external dependencies, zero write-path latency overhead). Trade-off: non-zero RPO (events in buffer lost on hard crash). This is acceptable for an analytics workload. See [ADR-0003](decisions/0003-buffer-strategy-in-memory-vs-kafka-vs-wal.md) for the full options analysis.

## 0) PRD Snapshot (frozen input)

> **Insider One Assessment Project**
>
> [PRD content unchanged — see original for full details]
>
> (Omitted here for brevity — PRD promises and endpoints remain the same.)

### 0.1 Glossary (PRD terms → precise meaning)

| Term | Meaning | Ambiguities / Notes |
|------|---------|---------------------|
| event | A single occurrence of user/system activity submitted as a JSON payload via the HTTP API | The PRD says "raw event data" — unclear whether events can originate from non-HTTP sources (e.g., batch import). Only HTTP ingestion is described. |
| ... | ... | ... |
| In-memory bounded buffer | A process-local bounded queue (Go channel) that holds validated events prior to batch persistence. No file-backed persistence. | Capacity and flush cadence determine RPO. Chosen for this submission due to 48-hour implementation constraint. See ADR-0003. |

---

## 1) Problem, success, and boundaries of the problem

(PRD goal, users, and non-goals unchanged.)

---

## 2) Architecture-shaping questions → assumptions (the "search space reducer")

### 2.1 Design-class questions (only the ones that change architecture class)

Q: What buffer should be used between HTTP ingestion and ClickHouse?
- Decision: In-memory bounded buffer (process-local, `chan domain.Event`) with a flush goroutine. Rationale: 48-hour constraint — fastest to implement correctly with zero external dependencies. See ADR-0003.

→ Assumption A3: "Very low latency" = < 50ms p99. In-memory channel adds ~0ms to the write path.

### 2.2 Assumptions ledger (explicit + testable)

| ID | Assumption | Confidence | Impact Area | What Breaks If Wrong | Validation Method |
|----|-----------|------------|-------------|---------------------|-------------------|
| A1 | ClickHouse remains the primary store. | High | data, scale | If PostgreSQL required, would need different write path. | Benchmark. |
| A2 | Dedup key is `(event_name, user_id, timestamp_ms, payload_hash)`. | High | data, consistency | Hash canonicalization must be deterministic. | Tests / ADR. |
| A3 | In-memory buffer is used in production. Buffer capacity and flush cadence bound expected RPO (accept non-zero). | Medium | durability, ops | If RPO must be 0, revert to durable broker or WAL. | Crash/restart tests. |
| ... | ... | ... | ... | ... | ... |

---

## 3) NFRs that actually drive design (numbers + behaviors)

### 3.1 Reliability & performance targets (adjusted)

| NFR | Target | Design Consequence | Source |
|-----|--------|--------------------|--------|
| Ingestion throughput (average) | 2,000 events/sec | In-memory enqueue + batch flush to ClickHouse. Monitor buffer depth. | PRD |
| Ingestion throughput (peak) | 20,000 events/sec | Buffer must absorb bursts; tune capacity and batch size. | PRD |
| Ingestion latency | < 50ms p99 | API responds after buffer admission (now faster due to no network hop). | A3 |
| Metrics query latency | < 2s p95 | On-demand ClickHouse aggregation. | A7 |
| Availability | 99.5% (single-node) | Single process; process supervision recommended. | Assumed |
| Durability | Best-effort (non-zero RPO) | In-memory buffer implies potential data loss on process/machine crash. Recommend WAL or durable broker if RPO=0 required. | A3 |

### 3.2 "What must be true in production?" (updated)

| ID | System-Level Truth | Design Consequence |
|----|-------------------|-------------------|
| T1 | No event with missing required fields is persisted. | Validation at HTTP handler. |
| T2 | No event with invalid timestamp is persisted. | Validation at HTTP handler. |
| T3 | Duplicate events do not inflate metrics. | ReplacingMergeTree + FINAL still used to dedup at storage/query time. |
| T4 | Accepted event is persisted while process remains healthy and flush completes. | In-memory buffer introduces potential loss on process crash; document expected RPO and mitigations. |
| T5 | Metrics reflect ingested events within bounded staleness (< 5s normal). | Flush cadence tuned (e.g., every 100ms or batch size). |
| T6 | Ingestion does not block under load. | Bounded buffer with capacity limit; if full → 503. |

---

## 4) Domain truth: entities, state, invariants

(Keep Event, Metric Result definitions. Update state machine to reflect in-memory buffer.)

### 4.2 State machines (only where state matters)

Event Lifecycle:

```
[Received] → validation → [Accepted/Buffered] → flush → [Persisted]
                ↓
           [Rejected (400)]
```

- On process crash, events in the in-memory buffer may be lost (unless WAL enabled). System should communicate this risk in README and runbooks.

### 4.3 Invariants (updated)

| ID | Invariant | Enforcement Point |
|----|-----------|-------------------|
| I1 | No event with missing required fields is persisted. | HTTP handler validation. |
| I2 | No event with invalid timestamp is persisted. | HTTP handler validation. |
| I3 | Duplicate events do not inflate metric counts. | Storage layer (ReplacingMergeTree) + query (FINAL). |
| I4 | Accepted event is eventually persisted while process is healthy; events in memory may be lost on crash. | In-memory buffer + flush; documented RPO. |
| I5 | Buffer cannot grow unbounded. Overflow → 503. | Bounded buffer with capacity limit. |

---

## 5) Boundaries and ownership

### 5.2 Containers / major runtime blocks (shapes before tech)

Go binary (single process recommended for MVP) with in-memory bounded buffer:

| Block | Responsibilities | Owns Which Truths | Depends On |
|-------|------------------|-------------------|------------|
| **Ingestion Handler** | HTTP receive, validate payloads (I1, I2), enqueue into in-memory buffer, return 202/400/503 | Write gate; enforces I1, I2, and I5 | Local memory, configuration |
| **In-memory bounded buffer** | Process-local queue (bounded). Holds events until flush. | I4 (temporary durability), I5 (bounded) | Go runtime |
| **Flush Consumer** | Reads from in-memory buffer (same process or dedicated worker), batches messages (e.g., 1000 events or 100ms), inserts into ClickHouse, marks persisted on success | Persists events to ClickHouse; enforces I3 at storage write | ClickHouse |
| **Metrics Query Engine** | Handle `GET /metrics`. Build aggregation queries; ensure dedup at query time. | I6 (deduplicated results) | ClickHouse |

> Note: For local development, this simplifies deployment (no external broker). For production with strict durability requirements, replace in-memory buffer with durable broker or WAL.

---

## 6) Critical flows and contracts

### Flow F1: Event Ingestion via in-memory buffer

Goal: Accept valid event, enqueue into in-memory buffer for fast response, respond quickly.

Happy path: POST /events → parse → validate (I1, I2) → enqueue into buffer → **202 Accepted**.

Failure contract:
- Timeouts: 5s HTTP read timeout. Enqueue should be non-blocking; if buffer is full or enqueue blocks beyond configured timeout → return **503** with `Retry-After`.
- Backpressure: Buffer full → **503** to caller. Consumers should retry safely (dedup at storage).
- Process crash: Events in buffer may be lost; documented expected RPO and mitigations (WAL or durable broker for future).
- Observability: `buffer_enqueue_total{status=ok|dropped}`, `buffer_depth`, `ingestion_latency_ms`.

### Flow F2: Flush Consumer → Batch Insert to ClickHouse

Goal: Consume messages from in-memory buffer, batch-insert into ClickHouse, ensure ack semantics via persistence.

Happy path: Read batch (up to batch_size or batch_timeout) → batch INSERT → ClickHouse ack → remove from buffer (in-memory removal done at pop).

Failure contract:
- ClickHouse insert failure: retry behavior handled by consumer loop; exponential backoff and alerting. If consumer cannot persist, buffer may fill and producers receive 503.
- Partial batch failure: treat as batch failure (no partial ack).
- Process crash during flush: in-memory items not yet popped may be lost.
- Observability: `flush_batch_size`, `flush_duration_ms`, `flush_errors_total`.

### Flow F3: Metrics Query

(No change — still ClickHouse queries with FINAL for dedup.)

### Flow F4: Process / Host failure (replaces broker failure scenarios)

- Process crash / host reboot: in-memory buffer contents are lost. Mitigation: frequent flush, small batch timeout, WAL for durability, or reintroduce durable broker.
- OOM: process crashed and restarted; lost buffer content. Mitigate with memory caps and bounded buffer.
- Detection & recovery: supervise process with systemd/docker restart policies; run crash tests to quantify expected loss window.

---

## 7) Data strategy (truth stores, read models, and consistency rules)

- System of record: ClickHouse `events` table (ReplacingMergeTree).
- Buffer is transient and not authoritative.
- Deduplication unchanged: `ORDER BY (event_name, user_id, timestamp_ms, payload_hash)` + FINAL at query time.

Schema and partitioning remain as previously defined.

---

## 8) Capacity & cost sanity

- Lower infra cost (no NATS cluster).
- Increased operational risk (potential data loss). Tune buffer capacity and flush cadence to balance RPO vs memory footprint.
- Pressure points: consumer throughput and ClickHouse availability remain critical; when ClickHouse is slow, buffer growth leads to 503.

---

## 9) Security, privacy, and compliance

(No change to security posture other than operational notes: ensure buffer does not hold PII longer than necessary; log minimal sensitive data.)

---

## 10) Operability and delivery

### 10.1 Observability plan (updated metrics)

Key metrics (in-memory design):
- `events_received_total{status=ok|validation_error|dropped}`
- `ingestion_latency_ms`
- `buffer_enqueue_total{status=ok|dropped}`
- `buffer_depth` (current number of events in buffer)
- `buffer_capacity` (configured capacity)
- `flush_batch_size`
- `flush_duration_ms`
- `flush_errors_total`
- `events_persisted_total`
- `metrics_queries_total{status}`
- `metrics_query_duration_ms`

Remove NATS-specific metrics from MVP monitoring (e.g., `nats_*`) unless a durable broker is reintroduced.

### 10.2 Runbooks (minimum set — updated)

- ClickHouse down: buffer absorbs briefly; if prolonged, producers receive 503. On recovery, consumer resumes flushes automatically.
- Buffer full / 503s: check `buffer_depth` and `flush_errors`. Increase flush throughput or fix ClickHouse.
- Process crash: inspect restart logs. Accept documented potential data loss; consider reingestion if possible.
- Slow queries and other runbooks unchanged.

### 10.3 Deployment & migration strategy

- Docker Compose: remove NATS from compose; ensure service starts with in-memory buffer mode. Provide env flag `BUFFER_STRATEGY=inmemory` (default) and document how to switch to durability mode.
- Migrations: unchanged.

---

## 11) Decisions, roadmap, and risks

### 11.1 Decision log (mini-ADRs)

| ID | Decision | Why | Rejected Alternatives |
|----|----------|-----|----------------------|
| D1 | ClickHouse as primary store | 20K/sec appends, columnar aggregation, native dedup | PostgreSQL (needs heavy tuning for this throughput) |
| D2 | ReplacingMergeTree + FINAL for dedup | Fast writes, storage-native dedup, correct at query time | Write-time lookup (slow), app-layer dedup set (memory) |
| D3 | In-memory bounded buffer | 48-hour constraint — fastest to implement (~100 LOC, zero dependencies, zero write-path latency). Best-effort durability acceptable for analytics. See [ADR-0003](decisions/0003-buffer-strategy-in-memory-vs-kafka-vs-wal.md). | WAL (go-diskqueue), NATS JetStream, Kafka |
| D4 | Single-process monolith for MVP | 1 dev, 48h | Microservices |
| D5 | 202 Accepted response | Async semantics | 200 OK |
| D6 | Graceful shutdown with drain timeout | Balance preservation vs speed | No drain |
| D7 | ORDER BY key preserved | Dedup + query patterns | - |

See [ADR-0003](decisions/0003-buffer-strategy-in-memory-vs-kafka-vs-wal.md) for full options analysis (in-memory, WAL, NATS JetStream, Kafka) and revisit triggers.

### 11.3 Risk register (top 5 — updated)

| # | Risk | Mitigation |
|---|------|------------|
| R1 | Data loss on process crash | Short flush cadence (50ms), generous buffer capacity (50K events), graceful shutdown drains buffer on SIGTERM |
| R2 | ClickHouse outage → buffer fill → producer 503 | Alerting on buffer depth; scale consumers; increase resilience of ClickHouse |
| R3 | Dedup key collision | xxHash64 + ms timestamps; tests |
| R4 | Query latency > 2s | Materialized views |
| R5 | Buffer misuse (memory growth) | Bounded buffer and OOM protection |

---

## 12) Traceability

(Links to PRD promises remain; change traceability entries that referenced JetStream to reference in-memory buffer behavior and associated trade-offs.)

---

## Appendix: Suggested README note (copy into README)

"Buffering strategy: For this submission we use a process-local in-memory bounded buffer for event ingestion (default). This reduces infra complexity but introduces potential data loss on host/process failure. If you require durable buffering and zero RPO, run with a durable broker (e.g., NATS JetStream or Kafka) or enable the WAL-backed mode (future work)."

END
