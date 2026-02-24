# Assumption Ledger (Λ) — Global

<!-- Promoted from step-local assumptions at phase completion. -->

| ID | Statement | Confidence | Impact Area | What Breaks If Wrong | Validation Method | Source Step |
|----|-----------|------------|-------------|---------------------|-------------------|------------|
| A1 | ClickHouse is the primary data store. Columnar OLAP optimized for 20K/sec appends and analytical aggregation. | High | data, scale, cost | If PostgreSQL required for transactional guarantees, over-engineer write path. | Benchmark 20K inserts/sec. | Step 2 |
| A2 | Dedup key is `(event_name, user_id, timestamp_ms, payload_hash)` via ReplacingMergeTree. Millisecond timestamps + xxHash64 payload hash. | High | data, consistency | Hash canonicalization must be deterministic (fixed field order, sorted tags). | Resolved via [ADR-0005](decisions/0005-dedup-key-collision-payload-hash-vs-accept.md). | Step 2 |
| A3 | "Very low latency" = < 50ms p99. In-memory buffer, respond 202, batch flush to ClickHouse. | High | scale, ops | If < 5ms: fire-and-forget (conflicts persistence). If > 100ms: simpler design. | Confirmed. In-memory channel adds ~0ms to write path. See [ADR-0003](decisions/0003-buffer-strategy-in-memory-vs-kafka-vs-wal.md). | Step 2 |
| A4 | "platform" = "channel". Use `channel` as canonical name. | High | data, API | N/A — confirmed by developer. | Confirmed. `channel` is the canonical field; PRD "platform" is a synonym. | Step 2 |
| A5 | Events are independent and unordered. | High | scale, data | If ordering required, need sequence numbers + ordered processing. | PRD describes independent payloads. | Step 2 |
| A6 | Fixed top-level schema + flexible `metadata` JSON. | High | data, scale | If fully dynamic, column-per-field breaks. | PRD example. | Step 2 |
| A7 | Metrics API < 100 QPS. On-demand aggregation sufficient for MVP. | Medium | scale, cost | If > 1K QPS, need materialized views. | Monitor in production. | Step 2 |
| A8 | Metrics query latency < 2s p95. | Medium | scale | If < 200ms required, need pre-computed views. | Benchmark. | Step 3 |
| A9 | Availability 99.5% (single-node). | Medium | ops | If 99.9%+ required, need replication + failover. | Assessment context. | Step 3 |
| A10 | Invalid timestamp = ≤ 0, > 24h future, > 1 year past. | High | data | N/A — confirmed by developer. | Confirmed. Bounds are correct as stated. | Step 3 |
| A11 | Peak burst duration is minutes, not sustained hours at 20K/sec. | High | scale | If sustained hours at peak, in-memory buffer fills and producers receive 503 until ClickHouse catches up. | Confirmed for this submission. Mitigated by generous buffer sizing and flush cadence. | Step 8 |
