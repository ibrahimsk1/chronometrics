# ADR-0002: Deduplication via ReplacingMergeTree + FINAL at query time

**Status:** Accepted
**Date:** 2026-02-23
**Context step:** Step 2 (Assumptions & Contradictions)
**Related:** ADR-0001 (ClickHouse as primary store), ADR-0003 (Async buffered writes)

---

## Context

The PRD requires idempotency: "Duplicate events should not be reprocessed. Ensure idempotency based on a field or combination of fields." At the same time, the ingestion API must handle 20K events/sec at peak with "very low latency" (assumed < 50ms p99 per A3).

These two requirements are in tension. Deduplication traditionally requires a lookup before each write — check whether the event already exists, then insert only if it doesn't. At 20K events/sec, that means 20K read-before-write round-trips per second on the hot path, which directly conflicts with the latency target.

The deduplication key itself is also unspecified. The PRD says "based on a field or combination of fields" but doesn't say which. The example payload has no client-supplied idempotency token (like a UUID). We assume `(event_name, user_id, timestamp)` as the composite key (A2): "the same user did the same action at the same second" is a duplicate.

---

## Options Considered

### Option A: Write-time lookup (check-then-insert)

Before inserting each event, query ClickHouse (or a cache) to see if a matching `(event_name, user_id, timestamp)` already exists. Insert only if absent.

**Pros:**
- Duplicates never reach storage. Clean data at rest.
- Simple mental model — dedup is immediate and visible.

**Cons:**
- Adds a read round-trip to every write on the hot path. At 20K/sec, that's 20K SELECT queries/sec *in addition to* 20K inserts/sec.
- ClickHouse is not optimized for point lookups. It's a columnar OLAP engine — scanning a column for a single row is expensive compared to PostgreSQL's B-tree index lookup.
- Latency impact: +5–20ms per event (ClickHouse point query latency). Pushes p99 well beyond the 50ms target.
- Under batch-insert architecture (D3), you'd need to check an entire batch against the database, adding batch-level latency.
- Creates a read-write dependency that limits write concurrency.

**Verdict:** Rejected. Incompatible with the latency and throughput requirements.

### Option B: Application-layer dedup set (in-memory bloom filter or hash set)

Maintain an in-memory set of recently seen dedup keys. Check the set before buffering. Evict entries after a TTL.

**Pros:**
- Sub-microsecond lookups. No write-path latency impact.
- Catches exact duplicates within the TTL window.

**Cons:**
- Memory-bounded. At 2K events/sec × 3600s = 7.2M entries/hour. Each key ~50 bytes → ~360 MB/hour for a 1-hour window. Manageable but non-trivial.
- Not durable — on restart, the set is empty. Duplicates from before the restart pass through.
- False negatives after TTL expiry. Late-arriving duplicates (retries after minutes) are missed.
- Bloom filter variant trades memory for false positives (incorrectly rejecting unique events).
- Adds complexity: sizing, TTL tuning, memory monitoring, restart behavior.
- Doesn't replace query-time dedup anyway — ClickHouse merges are non-deterministic, so FINAL is still needed for correctness.

**Verdict:** Rejected for MVP. Viable as an optimization layer *on top of* Option C if write-path dedup becomes a requirement. But it doesn't eliminate the need for storage-level dedup.

### Option B2: External dedup store via Redis

Same check-before-buffer approach as Option B, but backed by Redis (or RedisBloom) instead of an in-process data structure. Before buffering an event, check a Redis set for `(event_name, user_id, timestamp)`. If present, reject as duplicate. If absent, add the key with a TTL and proceed.

**Pros:**
- Survives process restarts. Unlike the in-process set, Redis persists the dedup window across crashes (with RDB/AOF). No dedup gap on restart.
- Shared across instances. If the system scales to multiple monolith instances behind a load balancer, all instances check the same Redis set. Dedup is globally consistent — instance A sees events ingested by instance B.
- Mature tooling. RedisBloom provides a probabilistic filter with configurable false-positive rates and built-in TTL, purpose-built for this use case.

**Cons:**
- Network round-trip on the hot path. Each event requires a Redis lookup + insert before buffering. Latency per operation: ~0.1–0.5ms (same-machine socket) to ~1–3ms (network hop). At 20K events/sec, this adds 2–60ms of serial work per second depending on deployment topology — requires pipelining and concurrent Redis connections to keep up.
- Against the 50ms p99 budget (A3), adding 1–3ms for Redis consumes 2–6% of the budget per event. Tolerable in isolation, but stacks with JSON parsing, validation, and buffer admission.
- New infrastructure dependency. The system goes from two runtime components (Go + ClickHouse) to three (Go + ClickHouse + Redis). Adds a container, health checks, memory monitoring, persistence config, and a new failure mode. If Redis is down, the system must choose: reject all events (503) or skip dedup (accept duplicates). Either choice has consequences.
- Memory cost shifts, doesn't disappear. At 2K events/sec average, a 1-hour dedup window is ~7.2M keys × ~80 bytes (Redis overhead per key) ≈ ~1–1.5 GB. Redis keeps everything in RAM — same cost as the in-process variant, just on a different machine.
- Still doesn't replace ReplacingMergeTree dedup. Redis TTL expiry means late-arriving duplicates (retry after TTL) pass through. Redis failure/restart opens a dedup gap. Redis pipelining for throughput can reorder check-then-insert operations, creating race windows. FINAL at query time is still required for correctness.
- No value for the current API contract. The PRD says "duplicate events should not be reprocessed" — it doesn't require callers to be told about duplicates. The 202 response means "accepted into the pipeline," and ReplacingMergeTree silently absorbs duplicates downstream. Redis adds cost to provide write-path feedback that no one consumes.

**When it becomes the right choice:**
- Horizontal scaling: multiple Go instances need globally consistent dedup. The in-process set can't do this; Redis can.
- Caller needs synchronous dedup feedback: API must return `409 Conflict` for duplicates. ReplacingMergeTree can't provide this; Redis can.
- Write-path dedup reduces storage pressure: if duplicate volume is very high (e.g., aggressive client retries), catching them before ClickHouse saves disk I/O and merge work.

**Verdict:** Rejected for MVP. The single-instance architecture (D4) doesn't need cross-instance dedup, and the API contract doesn't require synchronous dedup feedback. Viable as a future optimization when horizontal scaling triggers (multiple instances behind a load balancer). Even then, it supplements ReplacingMergeTree — it doesn't replace it.

### Option C: Accept duplicates at write time, deduplicate at storage + query time (ReplacingMergeTree + FINAL)

Insert all events without checking for duplicates. Use ClickHouse's `ReplacingMergeTree` engine, which deduplicates rows with the same ORDER BY key during background merge operations. At query time, use the `FINAL` modifier to force dedup before returning results.

**Pros:**
- Zero write-path overhead. No lookups, no memory structures. The write path is pure append.
- Aligns with ClickHouse's design philosophy — high-throughput appends, background optimization.
- ReplacingMergeTree is a native, battle-tested engine. No custom dedup logic to maintain.
- FINAL guarantees correct results at query time regardless of merge state.
- Retry-safe: callers can safely retry on timeout/503 without coordination. Duplicates are absorbed transparently.
- Simple implementation: the dedup strategy is encoded in the table DDL, not application code.

**Cons:**
- Duplicates exist on disk between merges. Storage is slightly inflated temporarily.
- FINAL adds overhead to queries (forces merge-on-read for unmerged parts). Impact depends on merge lag and data volume.
- Dedup is *eventual* at storage level. Between ingestion and the next merge, `SELECT` without FINAL returns duplicates.
- The ORDER BY key *is* the dedup key. Changing the dedup key later means changing the table's primary ordering — a schema migration.
- If the dedup key `(event_name, user_id, timestamp)` has collisions (two genuinely different events from the same user, same event, same second), one is silently dropped. See Risk R2.

**Verdict:** Accepted.

---

## Decision

We chose **Option C: ReplacingMergeTree + FINAL at query time.**

---

## Rationale

The core tension is: **dedup correctness vs. ingestion speed**. The priority rule is *ingestion throughput > query-time dedup cost*, because write volume is 10–100× query volume (2K–20K writes/sec vs. < 100 reads/sec per A7).

Option C is the only approach that adds zero latency to the write path. This matters because:

1. The write path is the system's defining constraint (20K/sec peak, < 50ms p99).
2. ClickHouse's architecture actively discourages point lookups and encourages batch appends. Fighting this design grain (Option A) would produce a slower system with more code.
3. The PRD says "metrics endpoint does not need to be fully real-time" — bounded staleness on dedup is consistent with the product's expectations.
4. Retries are the primary source of duplicates in this system (caller retries on timeout). Option C makes retries unconditionally safe without caller-side coordination.

The FINAL overhead on queries is the accepted cost. For the expected query volume (< 100 QPS), ClickHouse handles FINAL efficiently — it merges only unmerged parts at read time, not the full dataset. If this becomes a bottleneck, materialized views or explicit `GROUP BY` dedup in SQL are available escape hatches without changing the write path.

---

## Consequences

**What becomes easier:**
- Write path implementation — no dedup logic, no memory management, no coordination.
- Retry handling — callers retry freely; system absorbs duplicates.
- Operational simplicity — no bloom filter sizing, no TTL tuning, no cache invalidation.

**What becomes harder:**
- Every query must include FINAL or explicit GROUP BY dedup. Missing FINAL in any query path returns incorrect results. This is an invariant (I6) enforced by the Metrics Query Engine.
- Changing the dedup key requires an ALTER TABLE + data migration.
- Debugging data issues — duplicates exist on disk, which can confuse direct ClickHouse queries that don't use FINAL.

**Risks introduced:**
- R2: Dedup key collision. Two genuinely different events with matching `(event_name, user_id, timestamp)` are treated as duplicates. Mitigation: add a payload hash tiebreaker column if collision rate is observed. For analytics use cases, the probability is low (same user, same event, same second).

---

## Revisit Triggers

- If a caller requires *synchronous* dedup confirmation (e.g., "tell me whether this event was new or a duplicate"), Option C cannot provide that. Would require Option A, B2 (Redis), or a hybrid.
- If FINAL overhead pushes metrics query p95 > 2s, add materialized views with explicit dedup (doesn't change this ADR — it's an optimization within the same strategy).
- If the system scales to multiple instances, add Redis (Option B2) as a write-path dedup layer for cross-instance consistency. ReplacingMergeTree + FINAL remains the correctness backstop.
- If the dedup key proves wrong (collisions observed in production), add a hash tiebreaker column to the ORDER BY. This is an additive schema change.

---

## References

- Assumption A2: Dedup key is `(event_name, user_id, timestamp)`.
- Contradiction 2 in system_design.md §2.3: "Deduplication vs. very low latency."
- Risk R2 in system_design.md §11.3: Dedup key collision.
- Invariant I3: Duplicate events do not inflate metrics (system_design.md §4.3).
- Invariant I6: Metrics always return deduplicated counts (system_design.md §4.3).
