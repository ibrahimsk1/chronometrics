# ADR-0005: Dedup key collision — accept risk vs. payload hash vs. client UUID

**Status:** Accepted
**Date:** 2026-02-23
**Context step:** Step 2 (resolving R2, coupled with A2)
**Related:** ADR-0002 (ReplacingMergeTree + FINAL), D7 (ORDER BY as dedup key)

---

## Context

The dedup key is `(event_name, user_id, timestamp)`, encoded as the `ORDER BY` of a `ReplacingMergeTree`. Risk R2 identifies that this key is potentially too coarse: if the same user triggers the same event type twice within the same second with genuinely different intent (e.g., two rapid `product_view` events for different products), one event is silently dropped.

The collision scenario: user_123 views product A and product B within the same second. Both events have `event_name=product_view`, `user_id=user_123`, `timestamp=1723475612`. ReplacingMergeTree keeps one, drops the other. The `metadata` (which contains `product_id`) differs, but it's not part of the dedup key.

This is coupled with Assumption A2 (dedup key composition). Resolving A2 resolves R2.

**Note:** The project is greenfield — no existing schema, no existing callers, no data to migrate. All options are equally available from a compatibility standpoint.

### Collision probability estimate

At 2K events/sec average across ~10K daily active users and ~20 event types:
- Per user-event-type rate: ~0.01 events/sec (one event per 100 seconds)
- Probability of two events in the same 1-second window: ~0.01% per user-event pair
- Expected collisions per day: **low single digits** at current scale
- At 20K/sec peak: probability increases ~10×, but peaks are minutes-long (A11)

The collision rate is low but **non-zero and undetectable** — there's no signal that an event was wrongly dropped.

---

## Options Considered

### Option A: Accept current key — no change

Keep `(event_name, user_id, timestamp)` as-is. Accept the low collision probability for analytics.

**Pros:**
- Zero implementation cost.
- No write-path impact. No additional computation.
- ORDER BY stays narrow (3 columns) — optimal for merge performance and storage.
- Dedup semantics are simple: "same user, same event, same second = same event."
- For most analytics questions ("how many product views this week?"), losing a handful of events per day is noise within the margin of error.

**Cons:**
- Silent data loss. No way to detect that a genuine event was dropped.
- Undercounts are *systematic* — they correlate with high-activity users and rapid actions, exactly the cohort you'd want accurate data on.
- If the product evolves to track per-item conversions (product_view → purchase funnel), missing product views corrupt funnel analysis.
- Impossible to retroactively recover dropped events — the data is gone at merge time.

**When this is the right choice:**
- The system is purely for aggregate trend analysis where ±0.01% accuracy is irrelevant.
- There's no downstream use case that depends on per-event completeness.
- The team accepts "we might silently lose a few events per day" as a documented trade-off.

---

### Option B1+D: Millisecond timestamps + payload hash (combined)

Use millisecond-precision timestamps *and* a payload hash together. The dedup key becomes `(event_name, user_id, timestamp_ms, payload_hash)`.

Milliseconds shrink the collision window from 1 second to 1 millisecond (~1000× reduction). The payload hash eliminates whatever residual collisions remain. Together they provide defense in depth: millisecond precision handles the common case (events separated by even a few ms are distinct by timestamp alone), and the hash handles the edge case (truly simultaneous events with different payloads).

Since this is greenfield, millisecond timestamps aren't a "breaking change" — they're simply the chosen contract. The PRD example shows epoch seconds (`1723475612`), but the API can accept either and normalize. Millisecond precision is the modern standard for event systems.

```sql
CREATE TABLE IF NOT EXISTS events (
    event_name   String,
    user_id      String,
    timestamp_ms UInt64,        -- epoch milliseconds
    payload_hash UInt64,        -- xxHash64 of canonical payload fields
    channel      String,
    campaign_id  String,
    tags         Array(String),
    metadata     String,
    _inserted_at DateTime DEFAULT now()
) ENGINE = ReplacingMergeTree(_inserted_at)
ORDER BY (event_name, user_id, timestamp_ms, payload_hash)
PARTITION BY toYYYYMM(toDateTime(intDiv(timestamp_ms, 1000)))
```

**Hash input — full payload (B1 variant):**

The hash is computed over all non-key fields in a fixed canonical order: `channel`, `campaign_id`, `tags` (sorted), `metadata`. The question "are these two events the same occurrence?" is answered by "yes, if and only if every field is identical."

| Component | What it adds | Cost |
|-----------|-------------|------|
| Millisecond timestamp | Shrinks collision window 1000× | 0 — same column, different precision |
| xxHash64 payload hash | Eliminates remaining collisions | ~100ns per event, +8 bytes/row |

**Pros:**
- **Virtually zero collision probability.** Millisecond separation handles >99.9% of cases; hash handles the rest.
- **No API contract burden.** Hash is server-side, invisible to callers. Milliseconds are the natural timestamp format for new callers.
- **xxHash64 is extremely fast** (~10 GB/sec). Cost per event: < 100ns. Negligible vs. JSON parsing (~1-5μs).
- **8 bytes (UInt64)** per row for the hash — minimal storage overhead.
- **Retry safety preserved.** Identical retries → identical timestamp + identical hash → still deduped correctly.
- **Better query precision.** Millisecond timestamps enable sub-second aggregation if ever needed.
- **Timestamp auto-detection.** Server can detect seconds vs. milliseconds by magnitude (`< 10^12` → seconds, multiply by 1000; `>= 10^12` → already milliseconds). Tolerant of callers sending either format.

**Cons:**
- ORDER BY widens from 3 to 4 columns. Merge cost increases slightly. Negligible at expected data volume.
- **Canonicalization requirement.** The hash input must be deterministic: fields hashed in fixed order. For `metadata` (JSON string), either hash the raw string (callers must send consistent field ordering) or parse and re-serialize with sorted keys before hashing. Parsing adds ~1-2μs — still negligible.
- **Dedup semantics change.** "Same user, same event, same millisecond, same payload" = duplicate. This is strictly more precise than "same user, same event, same second." No legitimate use case depends on the coarser semantics.

**Implementation sketch (Go):**
```go
func computePayloadHash(e Event) uint64 {
    h := xxhash.New()
    h.WriteString(e.Channel)
    h.WriteString(e.CampaignID)
    sort.Strings(e.Tags)
    for _, tag := range e.Tags {
        h.WriteString(tag)
    }
    h.WriteString(e.Metadata)
    return h.Sum64()
}

func normalizeTimestamp(ts uint64) uint64 {
    if ts < 1e12 {
        return ts * 1000 // seconds → milliseconds
    }
    return ts // already milliseconds
}
```

---

### Option C: Require client-supplied idempotency UUID

Add an `event_id` field (UUID) to the API contract. Callers generate a UUID per event. The dedup key becomes `(event_id)` or `(event_name, user_id, timestamp, event_id)`.

**Pros:**
- Gold standard for idempotency. Caller explicitly declares "this is a unique event."
- Perfect dedup: no collisions, no false positives, no ambiguity.
- Industry-standard pattern (Stripe, AWS, etc.).
- Enables caller to safely retry with the same UUID — server deduplicates exactly that event.

**Cons:**
- **Shifts responsibility to the caller.** Callers must generate UUIDs, persist them for retries, and ensure the same UUID is sent on retry. If the caller generates a new UUID per attempt, dedup is useless.
- **No control over caller behavior.** A buggy client that always sends `event_id: "abc"` deduplicates everything. A client that never sends the same UUID twice deduplicates nothing. The server can't validate correctness of the UUID.
- **16 bytes per row** (UUID) vs. 8 bytes (UInt64 hash). Minor but 2× the storage overhead of B1+D.
- **MVP scope concern.** This is a protocol-level decision that affects all API consumers. For a 48-hour assessment project, it's over-engineering.

**When this is the right choice:**
- Multiple API consumers exist and need explicit idempotency guarantees.
- The system evolves beyond analytics into transactional event processing.
- Caller needs synchronous dedup feedback (409 Conflict for duplicates).

---

## Trade-off Matrix

| Criterion | A: Accept | B1+D: ms + hash | C: Client UUID |
|-----------|-----------|------------------|----------------|
| Collision elimination | No | Virtually yes (two layers) | Yes (if used correctly) |
| API contract impact | None | Minimal (ms timestamps are natural) | New required field |
| Write-path cost | None | ~100ns (xxHash64) | None (UUID in payload) |
| Storage overhead/row | 0 | +8 bytes (hash column) | +16 bytes (UUID column) |
| Retry safety | Safe | Safe (identical payload → identical hash) | Safe only if caller resends same UUID |
| Implementation effort | None | ~2 hours (hash fn + schema) | ~4 hours (API + validation + schema) |
| Query precision | 1-second granularity | Sub-second granularity | N/A |
| MVP appropriate | Yes | Yes | No |

---

## Recommendation

**Option B1+D (millisecond timestamps + full payload hash)** for the following reasons:

1. **Two layers of collision prevention.** Milliseconds handle the common case; the hash handles the edge case. Together they make false dedup effectively impossible.
2. **Zero API burden.** Hash is server-side. Millisecond timestamps are the modern default — and the server can auto-detect seconds vs. milliseconds for caller tolerance.
3. **Negligible cost.** ~100ns per event for hashing, +8 bytes/row storage. No measurable impact on the 50ms p99 budget.
4. **Better foundation.** Millisecond precision gives finer-grained queries for free. The hash gives perfect retry safety.
5. **Greenfield advantage.** Since there's no existing schema or callers, we design the right key from day one instead of accepting a known-weak key and planning to fix it later.

**Option A (accept)** remains defensible if the developer explicitly confirms that low single-digit event loss per day is acceptable and there's no future need for per-event completeness.

**Option C (client UUID)** is the natural upgrade path if the system later needs synchronous dedup feedback or serves multiple consumer teams with their own idempotency requirements. It complements B1+D rather than replacing it.

---

## Questions for developer

1. **Is low single-digit silent event loss per day acceptable?** If yes → Option A is sufficient. If no → B1+D.
2. **Will the system eventually need per-event completeness** (e.g., conversion funnels, billing)? If yes → B1+D now, Option C later. If no → Option A or B1+D.

---

## References

- R2 in [risks.md](../risks.md): Dedup key too coarse
- A2 in [assumptions.md](../assumptions.md): Dedup key composition
- D2 in [decisions.md](../decisions/decisions.md): ReplacingMergeTree + FINAL
- D7 in [decisions.md](../decisions/decisions.md): ORDER BY as dedup key
- [ADR-0002](0002-dedup-via-replacing-merge-tree.md): Deduplication strategy
