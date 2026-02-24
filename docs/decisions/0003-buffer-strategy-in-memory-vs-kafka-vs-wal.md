# ADR-0003: Buffer strategy — In-memory vs Kafka vs WAL vs NATS

**Status:** Accepted
**Date:** 2026-02-23
**Context step:** Step 2 (Contradictions — §2.3 Contradiction 1)
**Related:** ADR-0001 (ClickHouse as primary store), ADR-0002 (Dedup via ReplacingMergeTree)

---

## Context

The system accepts events at 2K–20K events/sec and must respond with < 50ms p99 latency (A3). ClickHouse's optimal write pattern is batch inserts (1K–5K rows per INSERT), not individual row inserts. This means the write path needs a buffer between the HTTP handler and ClickHouse: accept events fast, batch them, flush periodically.

The question is **where that buffer lives** and what durability guarantee it provides. The core tension (§2.3 Contradiction 1):

- **"Store event data in a persistent database"** implies durability.
- **"Very low latency" at 20K events/sec** implies the response can't wait for ClickHouse.

Between accepting (202) and flushing (INSERT), events exist only in the buffer. If the process crashes during that window, those events are lost. At peak (20K/sec) with a 100ms flush interval, that's ~2K events. This is Risk R1.

The decision: **how much do we pay to close that durability gap?**

---

## Options Considered

### Option A: In-memory bounded channel (current choice)

A Go buffered channel (or ring buffer) holds events between accept and flush. A background goroutine drains the channel in batches on a timer or size trigger. On graceful shutdown (SIGTERM), the buffer is drained before exit.

**Durability:** None for buffered events. Hard crash (SIGKILL, OOM, power loss) loses everything in the buffer.

**Worst-case data loss:** ~2K events at peak (20K/sec × 100ms flush interval). At average load: ~200 events.

**Latency impact on write path:** Effectively zero. Channel send is a memory operation (~100ns).

**Throughput impact:** None. Buffer admission is non-blocking until capacity.

**Operational complexity:** Minimal. No external dependencies. No disk management. No recovery logic. Single Go binary.

**Infrastructure cost:** Zero additional. Buffer lives in process memory (50–500 MB depending on capacity, §8.3).

**Implementation effort:** Low. Go channel + goroutine + timer. ~100 lines of code.

**Pros:**

- Simplest possible implementation. Fits the 48-hour constraint.
- Zero latency overhead on the hot path.
- No new dependencies or failure modes.
- Graceful shutdown (SIGTERM) achieves zero loss in the common case.
- Buffer depth is observable via metrics — operators see pressure building.

**Cons:**

- Hard crash loses buffered events. No recovery.
- Durability depends entirely on the process staying alive.
- No replay capability — if a ClickHouse batch insert fails after max retries and events are dropped, they're gone.

**When this is the right choice:**

- Data is analytics/behavioral — not financial, not transactional, not compliance-critical.
- Hard crashes are rare in containerized environments with health checks.
- The cost of losing ~2K events (0.001% of daily volume) is acceptable.
- 48-hour implementation window leaves no room for buffer infrastructure.

---

### Option B: Persistent buffer via Kafka

Events are produced to a Kafka topic immediately after validation. A separate consumer (or the same process acting as consumer) reads from Kafka and batch-inserts into ClickHouse. The HTTP handler responds 202 after the Kafka produce-ack.

**Durability:** Full. Events survive process crashes, restarts, and ClickHouse outages. Kafka retains events for a configurable period (hours to days). On recovery, the consumer resumes from the last committed offset.

**Worst-case data loss:** Zero (assuming `acks=all` with replication). Even ClickHouse being down for hours causes no loss — Kafka buffers indefinitely within retention.

**Latency impact on write path:** +1–5ms per event for a synchronous Kafka produce with `acks=1` (leader-only). +5–15ms with `acks=all` (wait for replication). This consumes 2–30% of the 50ms p99 budget. With async produce and local batching, amortized to <1ms but introduces a micro-buffer with its own loss window.

**Throughput impact:** Kafka easily handles 20K/sec per partition. Multiple partitions scale linearly. Not a bottleneck.

**Operational complexity:** High.

- Kafka cluster (3 brokers minimum for production replication) or managed Kafka service.
- KRaft or ZooKeeper for coordination (KRaft preferred in modern Kafka).
- Topic configuration: partitions, replication factor, retention, cleanup policy.
- Consumer group management: offsets, rebalancing, lag monitoring.
- Schema management: event serialization format (JSON, Avro, Protobuf).
- Monitoring: broker health, consumer lag, under-replicated partitions, disk usage.
- Docker Compose goes from 2 containers (app + ClickHouse) to 4+ (app + ClickHouse + Kafka + init container).

**Infrastructure cost:** Significant.

- Memory: Kafka brokers need 4–8 GB heap each. 3 brokers = 12–24 GB.
- Disk: At 20K events/sec × 500 bytes × 24h retention = ~860 GB/day uncompressed. Compressed ~170 GB/day.
- CPU: Moderate (batch compression, replication).
- Network: Replication traffic between brokers.
- For a managed service (Confluent, AWS MSK): $500–2000+/month depending on throughput.

**Implementation effort:** High. Kafka producer integration, consumer with offset management, error handling, serialization, Docker Compose additions, health checks, configuration. ~500–1000 lines of code + infrastructure config.

**Pros:**

- Zero data loss on crash — the defining advantage.
- Decouples ingestion rate from ClickHouse write rate. Kafka absorbs arbitrarily long ClickHouse outages (within retention).
- Enables replay: reprocess events by resetting consumer offsets. Useful for schema changes, backfills, or adding new consumers.
- Enables multi-consumer: add a second consumer (e.g., real-time alerting, different DB) without changing the ingestion path.
- Natural horizontal scaling: multiple producer instances, partitioned consumers.
- Built-in ordering within partitions (if ordering matters in the future).

**Cons:**

- Massive infrastructure overhead for a single-process analytics service. Kafka is a distributed system designed for multi-service architectures — deploying it for one producer and one consumer is architecturally disproportionate.
- Adds 1–15ms to the write path depending on acks strategy. Not a dealbreaker, but a measurable cost against the 50ms budget.
- Operational surface area is large: Kafka has dozens of tunable parameters, subtle failure modes (ISR shrinkage, consumer rebalancing storms, log compaction stalls), and requires dedicated monitoring.
- Does not eliminate the need for the in-memory buffer concept — Kafka *is* the buffer, but the consumer still batches for ClickHouse, just from Kafka instead of a channel.
- Docker Compose setup is no longer "run two containers" — evaluator setup friction increases significantly.
- 48-hour constraint makes proper Kafka integration risky. Half-baked Kafka integration is worse than no Kafka.

**When this is the right choice:**

- Data loss is truly unacceptable (financial events, compliance-mandated audit logs, billing events).
- Multiple consumers need the event stream (alerting, secondary storage, real-time processing).
- Horizontal scaling is required (multiple ingestion instances behind a load balancer).
- ClickHouse outages lasting minutes to hours are expected, and zero data loss during outages is required.
- Team has Kafka operational experience and existing Kafka infrastructure.

---

### Option C: Write-Ahead Log (WAL) on local disk

Before admitting an event to the in-memory buffer, append a serialized record to a local WAL file (sequential write). The HTTP handler responds 202 after the WAL write. On batch flush to ClickHouse, advance a checkpoint in the WAL. On startup after a crash, replay uncommitted WAL entries.

**Durability:** Events survive process crashes. The WAL on disk is the durability guarantee. On recovery, the process reads the WAL from the last checkpoint forward and re-submits those events to ClickHouse.

**Worst-case data loss:** Near-zero for process crashes. However:

- Disk failure loses the WAL — single-machine durability only (no replication).
- If `fsync` is called per event: zero loss. If `fsync` is batched (e.g., every 10ms): up to ~200 events at peak between fsyncs.
- OS page cache without explicit fsync: kernel crash or power loss can lose recently written data.

**Latency impact on write path:** Depends on fsync strategy.

- **fsync per event:** +0.5–2ms per event (SSD sequential write + fsync). At 20K/sec, this is 20K fsyncs/sec — SSD can handle ~10K–50K IOPS, so this is near the limit. Adds 1–4% to the p99 budget.
- **Batched fsync (every 10ms):** Amortized to ~~50µs per event. Negligible latency impact. But introduces a 10ms durability window (~~200 events at peak).
- **No fsync (OS writeback):** Effectively zero latency. But durability depends on OS page cache flush — a kernel crash loses recent writes. Not meaningfully different from in-memory for kernel-level failures.

**Throughput impact:** Sequential disk writes are fast (500 MB/s+ on SSD). At 20K events/sec × 500 bytes = 10 MB/sec — well within SSD bandwidth. Not a throughput bottleneck.

**Operational complexity:** Moderate.

- WAL file management: rotation (new file per N MB or time interval), deletion of checkpointed segments.
- Recovery logic: on startup, detect incomplete shutdown, replay from last checkpoint, handle partially-written records (CRC or length-prefix framing).
- Disk space monitoring: WAL grows if ClickHouse is slow. Need size limits and alerts.
- No new containers or external dependencies. Still a single Go binary.

**Infrastructure cost:** Low. Uses existing disk. At peak write rate, WAL uses ~10 MB/sec → ~600 MB/min before checkpointing. With prompt flushing, WAL stays small (< 100 MB).

**Implementation effort (custom):** Moderate. WAL write + checkpoint + rotation + recovery + CRC framing. ~300–500 lines of code. Must be correct — bugs in WAL recovery logic can cause duplicate or lost events on restart.

**Implementation effort (with existing libraries):** Low-to-moderate. Several Go libraries eliminate the need to build WAL logic from scratch:

- **`nsqio/go-diskqueue`** — NSQ's disk-backed FIFO queue as a standalone library. Handles file rotation, sequential writes, fsync, and crash recovery internally. ~20–30 lines of integration code. The closest drop-in replacement for the Go channel buffer.
- **`tidwall/wal`** — Pure Go write-ahead log. Append-only, index-based reads, automatic segment rotation. Minimal API (`Write`, `Read`, `TruncateFront`). ~50–80 lines of integration code (need to build queue/cursor semantics on top).
- **`dgraph-io/badger`** — Embeddable LSM-tree KV store. Use sequential keys as a persistent queue. More features than needed (ACID, TTL, GC) but battle-tested. ~100 lines of integration.

With `go-diskqueue`, the implementation effort drops from ~300–500 LOC to ~30 LOC, and crash recovery is the library's responsibility — significantly reducing the risk of WAL bugs.

**Pros:**

- Process-crash durability without external dependencies. Fills the gap between in-memory (no durability) and Kafka (full durability, high complexity).
- No new containers. Docker Compose stays at 2 containers. Evaluator friction unchanged.
- Low latency overhead with batched fsync strategy.
- Simple operational model compared to Kafka — it's just a file on disk.
- Recovery is local and fast: read a file, re-flush, done. No distributed coordination.
- With `go-diskqueue` or `tidwall/wal`, the implementation risk is dramatically lower than a custom WAL.

**Cons:**

- Does NOT survive disk failure. Single-machine durability only. For true durability, you still need replication (at which point Kafka or NATS is the better tool).
- Does NOT survive ClickHouse outages gracefully. The WAL grows linearly during outage (10 MB/sec at peak). A 10-minute outage = 6 GB WAL. Need disk capacity planning and a WAL size cap (with the same 503 backpressure as in-memory).
- Recovery logic is subtle and error-prone if building custom. With libraries (`go-diskqueue`), this concern is largely mitigated.
- fsync-per-event strategy is near SSD IOPS limits at 20K/sec. Must use batched fsync, which reintroduces a small durability window.
- WAL serialization format must be stable. If the event struct changes, the WAL format needs versioning or the recovery path breaks on schema transitions.
- Does NOT support horizontal scaling. The WAL is a local file — each instance has its own. No cross-instance visibility, no shared buffer.
- Does NOT support replay or multi-consumer beyond crash recovery.

**When this is the right choice:**

- Process-crash durability is needed but Kafka/NATS is overkill (single instance, no multi-consumer, no replay requirement beyond crash recovery).
- Using a library like `go-diskqueue` rather than building a custom WAL.
- Disk failure is mitigated externally (RAID, EBS volumes, etc.) so single-machine WAL is sufficient.
- The team does not plan to scale horizontally in the near term.

---

### Option D: NATS JetStream (lightweight persistent message broker)

Events are published to a NATS JetStream stream after validation. Flusher process(es) pull-consume from the stream and batch-insert into ClickHouse. The HTTP handler responds 202 after the NATS publish-ack (event persisted to NATS disk).

NATS is a single ~20 MB binary that runs as one container. JetStream adds persistent streams with file-backed storage, consumer groups, replay, and retention — Kafka's core semantics in a much lighter package.

**Durability:** Full. Events survive process crashes, restarts, and ClickHouse outages. NATS persists to disk with configurable fsync. On recovery, consumers resume from the last acked position.

**Worst-case data loss:** Zero for process crashes. Zero during ClickHouse outages (within stream `MaxBytes` capacity).

**Latency impact on write path:** +1–3ms per event for a synchronous NATS publish with file-backed JetStream. This consumes 2–6% of the 50ms p99 budget — less than Kafka's `acks=all` due to no replication overhead in single-node NATS.

**Throughput impact:** Single NATS server handles 100K+ msg/sec. Not a bottleneck.

**Operational complexity:** Low-to-moderate.

- Single NATS binary, one container. Docker Compose goes from 2 containers to 3 (app + NATS + ClickHouse).
- Simple configuration: stream definition (subjects, retention, limits) and consumer definition (durable name, ack policy).
- Built-in monitoring via HTTP endpoint (`/jsz`, `/connz`).
- No ZooKeeper/KRaft. No partition rebalancing. No ISR management.
- Fewer tunable knobs than Kafka — simpler to configure correctly.

**Infrastructure cost:** Low.

- Memory: NATS server needs ~50–128 MB RAM (vs Kafka's 4–8 GB per broker).
- Disk: With `WorkQueuePolicy` (delete on ack), steady-state is ~50–100 MB. Only grows during ClickHouse outages.
- CPU: Minimal.
- Single container vs Kafka's 3+ broker cluster.

**Implementation effort:** Moderate. NATS client integration (`nats-io/nats.go`), publish in handler, pull consumer in flusher, ack after batch INSERT. ~150–250 lines of code. Plus role-based config for ingestor/flusher separation.

**Retention / cleanup:**

- **`WorkQueuePolicy`** (recommended): messages deleted immediately on ack. Near-zero disk usage in normal operation. Self-cleaning.
- **`LimitsPolicy`**: messages retained until `MaxAge`/`MaxBytes` limits. Enables replay and multi-consumer. Higher disk usage.
- **`MaxBytes` cap**: safety valve during ClickHouse outages. When full, new publishes are rejected (ingestor returns 503) — same backpressure as in-memory buffer, but with hours of headroom instead of seconds.

**Horizontal scaling model:**

Same Go binary with role-based configuration (`ROLE=ingestor|flusher|both`):

```
            ┌─ Go (ingestor) ──┐                     ┌── Go (flusher) ──┐
LB ─────────┼─ Go (ingestor) ──┼─→ NATS JetStream ───┼── Go (flusher) ──┼─→ ClickHouse
            └─ Go (ingestor) ──┘      stream         └──────────────────┘
```

- Ingestor replicas and flusher replicas scale independently.
- NATS pull consumers distribute messages automatically across flushers (round-robin on `Fetch()`). No manual partitioning.
- Adding/removing flushers is instant — no rebalancing storm.

**Pros:**

- Kafka-level durability and semantics (persistent streams, consumer groups, replay) with ~10% of Kafka's operational and infrastructure cost.
- Single container, ~50 MB RAM. Docker Compose stays lightweight.
- Natural decoupling of ingestion and flushing — ingestors become stateless publishers, flushers are dedicated ClickHouse writers. Scale each independently.
- Pull-based consumers with explicit ack: flusher controls batch size and timing. Failed batch → don't ack → NATS redelivers. Safe because ReplacingMergeTree absorbs duplicates.
- `WorkQueuePolicy` provides self-cleaning queue with near-zero disk footprint.
- Mature Go client (`nats-io/nats.go`) — first-party, well-maintained.
- Embedded option available: NATS server can run in-process as a library (`nats-io/nats-server`), eliminating even the extra container. Useful for dev/test.

**Cons:**

- Still an external dependency and an additional container (vs in-memory or WAL which are embedded).
- +1–3ms write-path latency. Not zero like in-memory.
- NATS JetStream is less battle-tested at extreme scale (millions of msg/sec) than Kafka. For this system's throughput (20K/sec), this is irrelevant.
- Single NATS server is not replicated. Disk or host failure loses the stream. For infrastructure-level durability, need NATS clustering (adds complexity) or Kafka.
- Adds ~150–250 lines of code plus role-based startup logic.
- Overkill for MVP where the in-memory buffer is sufficient and the 48-hour constraint is binding.

**When this is the right choice:**

- Horizontal scaling: multiple Go instances need a shared buffer. In-memory and WAL can't do this.
- Need persistent buffering (zero crash data loss) without Kafka's infrastructure weight.
- ClickHouse outages lasting minutes to hours must not cause data loss or 503s.
- Future replay or multi-consumer requirements are likely but not yet active.
- Team wants Kafka-like semantics with Docker Compose simplicity.

---

## Decision

**Option A: In-memory bounded channel.** This is the only option chosen. No broker upgrade path is planned for this submission.

---

## Rationale

The 48-hour implementation window is the primary driver.

| Dimension | In-memory | WAL (go-diskqueue) | NATS JetStream | Kafka |
|-----------|-----------|-------------------|----------------|-------|
| Crash data loss | ~2K events at peak | ~0–200 events | 0 | 0 |
| Write-path latency added | 0 | +50µs–2ms | +1–3ms | +1–15ms |
| New dependencies | 0 | 0 (embedded lib) | +1 container | +3 containers |
| Implementation effort | ~100 LOC | ~30–500 LOC | ~200 LOC + infra config | ~500–1000 LOC + infra config |
| Docker Compose | 2 containers | 2 containers | 3 containers | 4+ containers |
| ClickHouse outage tolerance | ~5s buffer at peak | disk capacity | hours (MaxBytes cap) | days (retention) |

**Why in-memory wins under the 48-hour constraint:**

1. **Implementation speed.** ~100 LOC: a bounded `chan domain.Event`, a ticker-driven flush goroutine, and a `Close()` that drains. Every other option requires at minimum an external dependency or significant infrastructure configuration. With 48 hours total, spending 4–16 hours on buffer infrastructure leaves too little for the core requirements (ingestion, validation, dedup, metrics, testing, README).

2. **Zero external dependencies.** Options C (WAL) and D (NATS) add dependencies — even `go-diskqueue` adds a library and disk management. Kafka adds a cluster. In-memory runs with nothing but Go's stdlib. Evaluators run `docker compose up` and it works.

3. **Correct under the actual data contract.** This system ingests analytics events. Best-effort delivery is a documented and deliberate trade-off, not an oversight. Hard crashes are rare in containerized environments; graceful shutdown (D6) drains the buffer completely in the common case.

4. **Zero write-path latency overhead.** Channel send is a memory operation (~100ns). Any persistent buffer — even a WAL — adds disk I/O or a network hop on the critical path. At 20K events/sec, even +1ms per event would consume the entire p99 latency budget.

---

## Consequences

**What becomes easier:**
- Implementation stays entirely on core requirements.
- Deployment: `docker compose up` — two containers, nothing to misconfigure.
- Debugging: buffer is observable (depth metric), no disk or broker state.

**What becomes harder:**
- If zero crash data loss is required, the buffer implementation must change. The `ingest.EventPublisher` interface isolates this — swapping implementations does not touch the HTTP handler or query path.
- During prolonged ClickHouse outages, the buffer fills and producers receive 503 until recovery.

**Risks accepted:**
- Events in the buffer are lost on a hard crash (SIGKILL, OOM). Mitigated by: graceful shutdown drains on SIGTERM (D6), generous buffer sizing, flush cadence tuned short.
- ClickHouse outage lasting more than buffer capacity (~50s at peak) causes 503 backpressure and potential data loss.

---

## Revisit Triggers

| Trigger | Candidate option |
|---------|-----------------|
| Process-crash durability required (single instance) | WAL via `go-diskqueue` (~30 LOC, no extra container) |
| Horizontal scaling needed or ClickHouse outage tolerance > minutes | NATS JetStream or Kafka |
| Data is billing / compliance / audit (not analytics) | Persistent broker required |

---

## References

- Contradiction 1 in system_design.md §2.3: persistence vs. low latency.
- Decision D3 in [decisions.md](decisions.md): in-memory bounded channel.
- Decision D6 in [decisions.md](decisions.md): graceful shutdown with 10s drain.
- A3 in [assumptions.md](../assumptions.md): < 50ms p99 latency target.
- A11 in [assumptions.md](../assumptions.md): peak burst is minutes, not hours.

