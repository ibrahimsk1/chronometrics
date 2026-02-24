# Priority Weights (Π) — Global

<!-- Set during Phase 1 (Step 2 — contradictions and priority rules). -->

## Priority Rule

```
ingestion_throughput > ingestion_latency > correctness (validation/dedup) > metrics_latency > durability_per_event > cost > developer_experience
```

## Tradeoff Decisions

| Tradeoff | Priority Rule Applied | Decided In |
|----------|----------------------|------------|
| Persistence vs. latency (async buffer) | ingestion_latency > durability_per_event | Step 2, Contradiction 1 |
| Write-time dedup vs. read-time dedup | ingestion_throughput > query-time dedup cost | Step 2, Contradiction 2 |
| In-memory buffer vs. persistent queue | implementation_speed > durability_per_event (48h constraint — no time for broker infrastructure) | [ADR-0003](decisions/0003-buffer-strategy-in-memory-vs-kafka-vs-wal.md) |
| Dedup precision (4-col key + hash) vs. write simplicity | correctness > cost > developer_experience | [ADR-0005](decisions/0005-dedup-key-collision-payload-hash-vs-accept.md) |
