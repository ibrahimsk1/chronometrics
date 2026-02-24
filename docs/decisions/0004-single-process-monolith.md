# ADR-0004: Single-process monolith with three logical blocks

**Status:** Accepted
**Date:** 2026-02-23
**Context step:** Step 5 (Boundaries & Ownership)
**Related:** ADR-0003 (Async buffered writes — buffer is in-process, tightly coupled to this decision)

---

## Context

The system has three distinct responsibilities: (1) receive and validate events via HTTP, (2) buffer and batch-flush events to ClickHouse, (3) serve aggregated metrics queries. The question is how to deploy these responsibilities: as separate services (microservices), as modules within a single process (monolith), or some hybrid.

Constraints at decision time:
- 1 developer, 48-hour implementation window.
- Single ClickHouse instance as the only external dependency.
- No multi-tenancy, no distinct business domains, no team coordination needs.

---

## Options Considered

### Option A: Microservices — three separate services

Split into Ingestion Service, Buffer/Flusher Service, and Metrics Service. Each deployed independently with its own container, health checks, and scaling policy.

**Pros:**
- Fault isolation: a crash in the Metrics Service doesn't affect ingestion.
- Independent scaling: scale ingestion and metrics separately based on load.
- Independent deployment: update metrics query logic without redeploying ingestion.
- Technology freedom: each service could use a different language or runtime.

**Cons:**
- **Network hop on the hot path.** The buffer currently lives in-process (D3: async buffered writes). Extracting it into a separate service means the Ingestion Service must send events over the network to the Buffer Service. This adds 2–5ms latency per event — a significant chunk of the 50ms p99 budget (A3). To maintain the same latency guarantee, you'd need a durable message queue (Kafka/Redpanda) between them, which is a new infrastructure dependency.
- **Operational overhead for one developer.** Three services means three Dockerfiles, three health checks, three log streams, service discovery, inter-service authentication, distributed tracing, and coordinated deployments. This is weeks of ops work that delivers zero user-facing value.
- **Fault isolation is illusory for this system.** The three blocks have a linear dependency: if ingestion fails → no new data → metrics are stale. If the buffer fails → ingestion must stop (nowhere to write) → 503. If metrics fails → ingestion is unaffected (the one genuine isolation win), but this is also true in a monolith because Go's goroutine model already isolates HTTP handlers.
- **Independent scaling solves the wrong bottleneck.** The scaling constraint is ClickHouse write/read throughput, not the Go application layer. At 20K events/sec, Go uses < 5% CPU. Scaling the Go services doesn't help if ClickHouse is the ceiling. Horizontal scaling means running multiple monolith instances behind a load balancer — no microservices needed.
- **No domain boundary to split on.** The three blocks are parts of one bounded context (event analytics). They share the same data model, the same ClickHouse schema, and the same invariants (I1–I6). Microservices gain their power from mapping to independent bounded contexts. Splitting within a single context creates distributed coupling — the services must be deployed and reasoned about together anyway.

**Verdict:** Rejected. Adds operational complexity with no architectural benefit for this scale and team size. The latency cost on the hot path is the strongest technical argument against.

### Option B: Monolith with three logical modules (chosen)

Single Go binary with three internal packages: `ingestion`, `buffer`, `metrics`. They share the process but have distinct responsibilities, separate test suites, and clear interfaces between them.

**Pros:**
- In-process buffer access. The ingestion handler writes directly to a Go channel — sub-microsecond latency, no serialization, no network.
- Single deployment artifact. One Dockerfile, one health check, one log stream. Matches the 1-developer, 48-hour constraint.
- Clean internal boundaries. Go packages enforce interface contracts at compile time. The separation is real — it's just not a network boundary.
- Preserves the option to extract later. If a domain boundary emerges (e.g., metrics becomes a separate product with its own team), extracting a package into a service is straightforward. The module interfaces are already defined.
- Operational simplicity. `docker-compose up` starts the app + ClickHouse. Done.

**Cons:**
- Shared process: a panic in one module (if unrecovered) kills the entire process.
- Shared memory: a memory leak in the buffer affects metrics query performance.
- Single scaling unit: can't scale ingestion independently of metrics.
- Deployment coupling: updating metrics query logic requires redeploying the whole binary (though for a single developer this is a non-issue).

**Verdict:** Accepted.

### Option C: Monolith with eventual extraction plan (service-ready boundaries)

Same as Option B, but invest extra time in making the internal boundaries strict enough for future extraction: define gRPC contracts between modules, use dependency injection for all cross-module calls, add interface adapters.

**Pros:**
- Extraction to microservices becomes mechanical (swap in-process call for gRPC call).
- Forces discipline in module boundaries.

**Cons:**
- Premature abstraction. gRPC contracts between modules that share a process add ceremony (protobuf definitions, code generation, serialization/deserialization) with no runtime benefit.
- Speculative investment. We don't know *if* extraction will be needed, let alone *where* the boundary would fall. The buffer might merge with ingestion, or metrics might split into real-time + batch. Designing for an unknown future split wastes time.
- 48-hour constraint makes this a luxury we can't afford.

**Verdict:** Rejected for MVP. The clean package boundaries in Option B are sufficient for future extraction without the ceremony of explicit service contracts.

---

## Decision

We chose **Option B: Single-process monolith with three logical modules.**

The three blocks:

| Block | Package | Responsibilities |
|-------|---------|-----------------|
| Ingestion Handler | `ingestion` | HTTP receive, validate (I1, I2), buffer admit, return 202/400/503 |
| Event Buffer + Flusher | `buffer` | Bounded in-memory queue, batch flush to ClickHouse, graceful shutdown drain |
| Metrics Query Engine | `metrics` | Handle GET /metrics, build aggregation SQL, enforce dedup (I6) |

---

## Rationale

This decision is structural, not just a time constraint. Even with unlimited time, microservices would be wrong for this system because:

**1. Single bounded context.** The entire system is one domain: event analytics ingestion and querying. There are no independent business capabilities to decompose. The three blocks are phases of a single pipeline, not distinct products.

**2. Linear data flow with shared state.** The pipeline is: HTTP → Buffer → ClickHouse ← Query. The buffer is an in-memory channel — putting a network boundary in the middle adds latency and requires a durable broker (Kafka) to maintain the same guarantees. That's a new infrastructure dependency to solve a problem we don't have.

**3. Single data store.** All three blocks talk to the same ClickHouse instance. There's no polyglot persistence, no independent data ownership. The single-writer rule (§5.3) works cleanly: one write path, one read path.

**4. Scaling axis is ClickHouse, not the application.** Go handles 20K HTTP requests/sec trivially (< 5% CPU on commodity hardware). The bottleneck is ClickHouse write throughput and query aggregation. Scaling the Go layer doesn't help. When horizontal scaling is needed, the right move is multiple monolith instances + ClickHouse cluster — not microservices.

The 48-hour constraint and single developer *reinforce* this decision but don't *cause* it. A 10-person team with 6 months would still start with a monolith for this system and extract services only when a genuine domain boundary appears (Conway's Law trigger).

---

## Consequences

**What becomes easier:**
- Deployment: one binary, one Dockerfile, `docker-compose up`.
- Debugging: single process, single log stream, no distributed tracing needed.
- Buffer access: in-process channel, sub-microsecond write admission.
- Testing: integration tests run against one process.

**What becomes harder:**
- A panic in any module kills the whole process. Mitigation: recover middleware on HTTP handlers; let buffer panics crash (indicates a bug that shouldn't be papered over).
- Can't deploy metrics fixes without redeploying ingestion. For a single developer this is a non-issue. For a team, this would become a pain point.
- Memory contention: a buffer leak could degrade metrics queries. Mitigation: bounded buffer (I5) caps memory usage.

**Risks introduced:**
- Shared-process failure mode: one unrecovered panic kills all three blocks. Mitigation: Go's `recover` in HTTP middleware + Docker restart policy + health check.

---

## Revisit Triggers

- **Team grows beyond 3 developers** working on this system simultaneously, with different release cadences for ingestion vs. metrics. Conway's Law: the architecture should mirror the team structure.
- **Distinct domain boundary emerges** — e.g., metrics becomes a separate product with its own data store, or a real-time alerting system is added alongside batch analytics.
- **Independent scaling is needed at the Go layer** — e.g., metrics queries become CPU-intensive (ML-based anomaly detection) while ingestion remains IO-bound. This is unlikely for simple aggregation queries.
- **Regulatory isolation** — e.g., ingestion must run in a different security zone than metrics.

None of these triggers exist today.

---

## References

- Assumption A3: "Very low latency" means < 50ms p99 — in-process buffer is critical to this.
- Assumption A5: Events are independent and unordered — no need for ordered processing across services.
- Decision D3: Async buffered writes — the buffer is in-process by design.
- system_design.md §5.2: Container/block decomposition.
- system_design.md §5.3: Ownership map (single-writer rule).
- system_design.md §5.4: Centralize vs. isolate strategy.
