# Risk Register (R) — Global

<!-- Promoted from step-local risks at phase completion. -->

| ID | Risk | Likelihood | Impact | Mitigation | Trigger Signal | Source Step |
|----|------|-----------|--------|-----------|---------------|------------|
| R1 | Buffer data loss on hard crash (~2K events at peak) | Low | Low | Accepted for MVP (0.001% of daily volume). Graceful shutdown (D6) covers common case. Upgrade path: NATS JetStream (Scale 2). See [ADR-0003](decisions/0003-buffer-strategy-in-memory-vs-kafka-vs-wal.md). | Process crash without SIGTERM. Revisit if data becomes compliance-critical. | Step 2 |
| R2 | Key revised to `(event_name, user_id, timestamp_ms, payload_hash)`. Millisecond precision + xxHash64 hash eliminate false dedup. | Negligible | Negligible | Resolved via [ADR-0005](decisions/0005-dedup-key-collision-payload-hash-vs-accept.md). Client UUID is upgrade path. | N/A | Step 2 |
| R3 | Metrics query latency exceeds 2s for large time ranges | Medium | Medium | Add materialized views for common query patterns; partition by date | Query p95 > 2s in monitoring | Step 3 |
| R4 | Prolonged ClickHouse outage exceeds buffer capacity → data loss | Low | Medium | Accepted for MVP (100K buffer ≈ 50s at peak). Upgrade path: NATS JetStream provides hours of buffering via WorkQueuePolicy with MaxBytes cap. See [ADR-0003](decisions/0003-buffer-strategy-in-memory-vs-kafka-vs-wal.md). | Buffer-full 503s > 1/day | Step 6 |
| R5 | metadata as String limits query flexibility | Low | Low | Add materialized columns for common metadata fields | developer requests metadata-based metrics | Step 7 |
| R6 | Storage growth without TTL: 6 TB/year if unbounded | Medium | Medium | Add ClickHouse TTL policy | Monthly storage > 1 TB | Step 8 |
| R7 | user_id is pseudonymous PII; could contain real identifiers | Low | Medium | Document user_id should be opaque; add field-level access in production | Compliance requirement | Step 9 |