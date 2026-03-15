Chosen Numbers

Disk:    NVMe SSD — 3,500 MB/s read, 3,000 MB/s write
RAM:     32GB — ~12GB available for active queries
CPU:     8 cores
Row:     180 bytes raw, 60 bytes compressed (3x ratio)
Partition: all queries hit same month (worst case)
ClickHouse: default background_pool_size=16
Foundation Math
Part size:


5,000 rows × 60 bytes compressed = 300KB per part
4 batches/sec → 4 parts/sec created → 1.2 MB/s write (trivial for NVMe)
Background merge speed:


Merge 4 parts → 1: read 4×300KB + write 300KB = 1.5MB per merge op
NVMe can do: 3,000MB / 1.5MB = 2,000 merge ops/sec (I/O ceiling)
ClickHouse practical limit with CPU overhead: ~200 merge ops/sec
Each op eliminates 3 parts net → 600 parts eliminated/sec
Steady state parts (inserts only):


4 created/sec, merges easily handle it → stabilizes at ~15 active parts
FINAL Query Cost (at 15 parts)

I/O per query:    15 parts × 300KB = 4.5MB read
Memory per query: 4.5MB × 3 (decompress) + sort buffers = ~25MB
CPU per query:    decompress + sort-merge 15 parts ≈ 5ms
Wall time:        ~15ms
Budget available for queries:


NVMe read:          3,500 MB/s
Reserve for merges:   200 MB/s
Reserve for inserts:    2 MB/s
Available for queries: 3,298 MB/s

Each query costs 4.5MB I/O × 2 queries per request = 9MB per request
Max requests/sec from I/O: 3,298 / 9 = ~366 requests/sec ceiling
Scenario 1: 20k inserts, no queries

Parts created:   4/sec
Parts merged:    ~200 ops/sec × 3 net = way more than enough
Stable parts:    ~15
Insert latency:  <5ms per batch
No issues. Merges idle most of the time.

Scenario 2: + 5 requests/sec

Query I/O:       5 req × 2 queries × 4.5MB = 45 MB/s
Merge budget:    3,298 - 45 = 3,253 MB/s remaining
Parts stable:    still ~15
Query latency:   ~15ms
Memory used:     5 concurrent × 25MB = 125MB
Completely fine. System barely notices.

Scenario 3: + 100 requests/sec

Query I/O:       100 × 2 × 4.5MB = 900 MB/s
Merge budget:    3,298 - 900 = 2,398 MB/s — merges still healthy
Parts stable:    still ~15
Query latency:   ~15ms (parts haven't grown)
Memory used:     100 concurrent × 25MB = 2,500MB
Still fine. 100 req/sec is not actually the danger zone on NVMe.

Where it actually breaks

Query I/O saturates NVMe read when:
3,298 MB/s / 9 MB per request = 366 requests/sec

But before that, memory breaks first:
12GB available / 25MB per query = 480 concurrent queries
At 15ms per query: 480 / 0.015 = 32,000 queries/sec max throughput
→ memory is not the bottleneck here

Real ceiling: ~350 requests/sec sustained
At 350+ req/sec, merges get starved → parts accumulate → each query reads more → cost per query rises → ceiling drops → feedback loop.

Summary
Requests/sec	Query I/O	Merge health	Parts	Status
0	0 MB/s	Full speed	~15	Healthy
5	45 MB/s	Full speed	~15	Healthy
100	900 MB/s	Healthy	~15	Healthy
300	2,700 MB/s	Starving	climbing	Degrading
366+	3,298 MB/s	Dead	exploding	Breaking
On NVMe + 32GB, your setup handles far more than you'd expect. The bottleneck isn't 100 queries — it's closer to 350 requests/sec before parts start snowballing.