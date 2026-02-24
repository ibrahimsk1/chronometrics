# InsiderOne Event Analytics – Technical Design Document (v2.0)

**Version**: 2.0
**Date**: 2026-02-24
**Status**: Implementation-aligned (updated 2026-02-24)
**System Design**: `docs/system_design.md` (reference — do not duplicate)

---

## Relationship to System Design

This document is the application-level companion to the System Design document. It answers "how do we build it in code?" while the System Design answers "what are we building and why?"

This TDD focuses exclusively on the single-process, in-memory buffer strategy (the project's development/short-term default). It removes references to multi-process deployments and message broker strategies.

What this document covers (complementary to System Design):
- Concrete technology choices with versions
- Application directory/package layout
- Type definitions, interfaces, and dependency graph
- HTTP API schemas — exact request/response shapes
- Configuration structure and defaults (memory strategy)
- Error handling and concurrency patterns for the in-memory strategy
- Testing strategy, tools, and environments (memory-focused)
- Startup wiring and composition for a single ingestor binary

Scope: This TDD targets the in-memory buffer strategy only:
- In-memory (single-process): bounded channel buffer, local flush loop, ClickHouse as persistent store.

---

## 1. Technology Stack

### 1.1 Language & Runtime

| Component | Choice | Version | Justification |
|-----------|--------|---------|---------------|
| Language | Go | 1.23 | Standard library improvements (slog, improved net/http). Small HTTP surface (4 endpoints) — no third-party router required. |
| Min Go version | 1.22 | — | Keeps compatibility with relevant stdlib features used here. |

### 1.2 Dependencies

| Dependency | Version | Purpose |
|------------|---------|---------|
| `github.com/ClickHouse/clickhouse-go/v2` | v2.28.0 | ClickHouse driver for batch INSERT and queries |
| `github.com/cespare/xxhash/v2` | v2.3.0 | xxHash64 computation for payload dedup key |
| `log/slog` (stdlib) | Go 1.23 | Structured JSON logging |
| `net/http` (stdlib) | Go 1.23 | HTTP server and routing |

**Test-only dependencies:**
| Dependency | Version | Purpose |
|------------|---------|---------|
| `github.com/stretchr/testify` | v1.9.0 | Assertion helpers |
| `github.com/testcontainers/testcontainers-go` | v0.34.0 | Ephemeral ClickHouse for integration tests |

### 1.3 Dev & Build Tools

| Tool | Version | Purpose |
|------|---------|---------|
| golangci-lint | v1.62.0 | Linting |
| Docker / Docker Compose | Docker 27+, Compose v2 | Local dev (ClickHouse + ingestor) |
| Make | GNU Make | Task runner: `build`, `test`, `test-integration`, `lint`, `docker-up`, `docker-down` |

### 1.4 Infrastructure

| Component | Image/Version | Purpose |
|-----------|---------------|---------|
| ClickHouse Server | `yandex/clickhouse-server:latest` | Event store + query engine |

---

## 2. Application Layout

### 2.1 Directory Structure

```
eventmetrics/
├── cmd/
│   └── ingestor/
│       └── main.go               ← HTTP server (ingestion + metrics + health); single-process
├── internal/
│   ├── domain/
│   │   ├── event.go              ← Event, RawEvent, ToEvent(), NormalizeTimestamp(), Validate()
│   │   ├── hash.go               ← ComputePayloadHash() (xxHash64)
│   │   ├── metrics.go            ← QueryParams, MetricResult, GroupResult, ValidateQueryParams()
│   │   └── errors.go             ← ErrPublishFailed, ErrQueryTimeout, ValidationError
│   ├── ingest/
│   │   └── usecase.go            ← UseCase, EventPublisher interface, NewUseCase()
│   ├── handler/
│   │   ├── handler.go            ← Handler struct, Router(), handleMetrics, handleHealth
│   │   ├── ingest.go             ← handleIngest (POST /events)
│   │   ├── bulk.go               ← handleBulk (POST /events/bulk)
│   │   ├── adapter.go            ← UseCaseAdapter bridging ingest.UseCase → handler.Ingester
│   │   ├── types.go              ← Ingester, MetricsQuerier, HealthChecker interfaces; HealthStatus
│   │   ├── middleware.go         ← body limit, recovery, request logging
│   │   ├── errors.go             ← writeError, writeJSON helpers
│   │   └── response.go           ← response types
│   ├── buffer/
│   │   └── buffer.go             ← IN-MEMORY: bounded channel, flush loop; implements ingest.EventPublisher
│   ├── repository/
│   │   ├── repository.go         ← Repository struct, New, Connect, Ping, Health, RunMigrations
│   │   ├── insert.go             ← Flush (batch INSERT)
│   │   ├── query.go              ← Query (aggregation SQL with FINAL)
│   │   └── migrations.go         ← ApplyMigrations(), reads SQL files from migrations/
│   ├── config/
│   │   └── config.go             ← Config struct (BufferStrategy + Server + Buffer + ClickHouse + Validation), Load()
│   └── app/
│       └── setup.go              ← Setup(), Base struct (Config, Publisher, Buffer, Repository), Shutdown()
├── migrations/
│   └── 001_create_events.sql
├── openapi.yaml
├── docker-compose.yml            ← ClickHouse + ingestor
├── Dockerfile
├── Makefile
└── README.md
```

### 2.2 Package Responsibilities

| Package | Owns | Implements | Does NOT |
|---------|------|------------|---------|
| `internal/domain` | Event, RawEvent, Validate(), NormalizeTimestamp(), ComputePayloadHash(), QueryParams, MetricResult, error types | Domain invariants (I1/I2 logic) | No IO |
| `internal/ingest` | UseCase (validate → transform → publish), EventPublisher interface | Orchestrates ingestion, strategy-agnostic | No HTTP, no DB access |
| `internal/handler` | HTTP routing (ServeMux), request parsing, response formatting, middleware, interfaces (Ingester, MetricsQuerier, HealthChecker), UseCaseAdapter | HTTP adapter | No business logic |
| `internal/buffer` | Bounded channel, flush loop, batch assembly, Flusher interface | EventPublisher (memory), owns I4 and I5 | No DB logic — depends on Flusher interface |
| `internal/repository` | ClickHouse connection (Connect/New), batch INSERT (Flush), aggregation queries (Query), migrations (ApplyMigrations), Ping, Health | Flusher, MetricsQuerier, HealthChecker implementations | No HTTP, no buffer logic |
| `internal/config` | Config struct (BufferStrategy + ServerConfig + BufferConfig + ClickHouseConfig + ValidationConfig), Load(), loadDotEnv() | Memory-only env vars (`CHRONOMETRICS_*`), required var validation | No IO beyond os.Getenv |
| `internal/app` | Base struct (Config, Publisher, Buffer, Repository), Setup(), Shutdown(ctx) | Composition wiring: config → ClickHouse → migrations → repository → buffer | No business logic |
| `cmd/ingestor` | Entrypoint: app.Setup(), wire use-case + adapter + handler, run HTTP server, graceful shutdown via SIGTERM/SIGINT | Ingestor single binary | No separate consumer process |

---

## 3. Dependency Graph & Direction

```
cmd/ingestor ──► app ──► config
    │               └──► repository ──► domain
    │
    ├──► ingest ──► domain
    │       └── declares EventPublisher interface
    │           ┌────────────────────────┐
    │           │  STRATEGY: memory      │ buffer ──► domain
    │           │  implements publisher  │    └── declares Flusher interface (→ repository)
    │           └────────────────────────┘
    │
    └──► handler ──► domain
            └── declares Ingester, MetricsQuerier, HealthChecker interfaces
```

Import flow: `cmd` → `app` → `buffer` → `repository` → `domain`

Note: `repository` also imports `handler` (to implement `handler.HealthChecker`). This is a pragmatic coupling — `repository.Repository` satisfies `handler.HealthStatus` to avoid a separate health wrapper.

### 3.2 Dependency Rules

- `domain` imports nothing from other internal packages.
- `ingest` depends on `domain` and declares `EventPublisher`.
- `buffer` depends on `domain` and declares `Flusher`.
- `repository` depends on `domain` and `handler` (implements Flusher + HealthChecker).
- `handler` depends on `domain` and declares adapter interfaces (Ingester, MetricsQuerier, HealthChecker).
- `app` wires concrete implementations; no business logic.

---

## 4. Type Definitions & Interfaces

### 4.1 Domain Types

The domain types are unchanged; they are oriented around ClickHouse storage and HTTP contract. JSON tags are present for API request/response mapping but no broker-specific serialization is required for the memory strategy.

See `internal/domain/event.go` for full Go definitions and helper functions (validation, timestamp normalization, hash computation using xxhash).

### 4.2 Interface Contracts

Strategy interface — the seam where the buffer plugs in:

```go
// package ingest
type EventPublisher interface {
    Publish(ctx context.Context, event domain.Event) error
    Close(ctx context.Context) error
}
```

Flush interface — used by the in-memory flush loop:

```go
// package buffer
type Flusher interface {
    Flush(ctx context.Context, batch []domain.Event) error
}
```

Handler interfaces (HTTP layer) remain the same: `Ingester`, `MetricsQuerier`, `HealthChecker`.

### 4.3 Use-Case: Ingest

UseCase orchestrates event ingestion (validate → transform → publish). It is strategy-agnostic but in this TDD the only concrete publisher is `buffer.Buffer`.

```go
// package ingest
func (uc *UseCase) Ingest(ctx context.Context, raw *domain.RawEvent) error {
    if raw == nil {
        return fmt.Errorf("raw event is nil")
    }
    if err := domain.Validate(raw, uc.maxFuture, uc.maxPast); err != nil {
        return err
    }
    ev, err := domain.ToEvent(raw)
    if err != nil {
        return err
    }
    if err := uc.publisher.Publish(ctx, ev); err != nil {
        // Wrap with domain sentinel while preserving original cause.
        return fmt.Errorf("publish failed: %w", errors.Join(domain.ErrPublishFailed, err))
    }
    return nil
}
```

**Handler adapter:** Because `ingest.UseCase` is not directly assignable to `handler.Ingester`, `handler.NewUseCaseAdapter(uc)` wraps it. This keeps the handler package's interface declaration clean without a circular dependency.

---

## 5. API Schema

The HTTP contract is defined in `openapi.yaml`. Endpoints:
- POST /events
- POST /events/bulk
- GET /metrics
- GET /health

Strategy differences removed: the API behaves consistently for the memory strategy. 503 responses occur when the buffer is at capacity.

Error codes: `VALIDATION_ERROR`, `PUBLISH_FAILED`, `PAYLOAD_TOO_LARGE`, `QUERY_TIMEOUT`, `INTERNAL_ERROR`.

Health endpoint reports memory buffer status and ClickHouse connectivity.

---

## 6. Configuration Design

### 6.1 Configuration Structure

Config for memory-only deployment. All env vars use the `CHRONOMETRICS_` prefix.

```go
package config

type Config struct {
    BufferStrategy string  // "memory" (only supported value)
    Server         ServerConfig
    Buffer         BufferConfig
    ClickHouse     ClickHouseConfig
    Validation     ValidationConfig
}

type ServerConfig struct {
    Port            int
    ReadTimeout     time.Duration
    WriteTimeout    time.Duration
    IdleTimeout     time.Duration
    BodyLimit       int64
    ShutdownTimeout time.Duration
}

type BufferConfig struct {
    Capacity              int
    FlushInterval         int           // milliseconds (legacy)
    FlushIntervalDuration time.Duration // preferred
    FlushBatchSize        int
    FlushRetries          int
    FlushTimeoutMs        int           // milliseconds (legacy)
    FlushTimeout          time.Duration // preferred
}

type ClickHouseConfig struct {
    Addr            string
    Database        string
    Username        string
    Password        string
    QueryTimeout    time.Duration
    MaxOpenConns    int
    MaxIdleConns    int
    ConnMaxLifetime time.Duration
    MaxRetries      int
    RetryBaseDelay  time.Duration
    RetryMaxDelay   time.Duration
}

type ValidationConfig struct {
    MaxFutureDuration time.Duration
    MaxPastDuration   time.Duration
}
```

### 6.2 Parameter Table (memory strategy)

**Server:**

| Parameter | Env Var | Default |
|-----------|---------|---------|
| Buffer strategy | `CHRONOMETRICS_BUFFER_STRATEGY` | `memory` |
| Server port | `CHRONOMETRICS_SERVER_PORT` | `8080` |
| Read timeout | `CHRONOMETRICS_SERVER_READ_TIMEOUT` | `5s` |
| Write timeout | `CHRONOMETRICS_SERVER_WRITE_TIMEOUT` | `10s` |
| Idle timeout | `CHRONOMETRICS_SERVER_IDLE_TIMEOUT` | `60s` |
| Body size limit | `CHRONOMETRICS_SERVER_BODY_LIMIT` | `1048576` (1 MB) |
| Shutdown timeout | `CHRONOMETRICS_SERVER_SHUTDOWN_TIMEOUT` | `10s` |

**Buffer:**

| Parameter | Env Var | Default |
|-----------|---------|---------|
| Buffer capacity | `CHRONOMETRICS_BUFFER_CAPACITY` | `1000` (code default); `50000` recommended via `.env` |
| Flush interval | `CHRONOMETRICS_BUFFER_FLUSH_INTERVAL` | `100ms` |
| Flush batch size | `CHRONOMETRICS_BUFFER_FLUSH_BATCH_SIZE` | `1000` (code default); `5000` recommended via `.env` |
| Flush timeout | `CHRONOMETRICS_BUFFER_FLUSH_TIMEOUT` | `10s` |
| Flush retries | `CHRONOMETRICS_BUFFER_FLUSH_RETRIES` | `3` |

**ClickHouse:**

| Parameter | Env Var | Default |
|-----------|---------|---------|
| ClickHouse addr | `CHRONOMETRICS_CLICKHOUSE_ADDR` | *required* |
| ClickHouse database | `CHRONOMETRICS_CLICKHOUSE_DATABASE` | *required* |
| ClickHouse username | `CHRONOMETRICS_CLICKHOUSE_USERNAME` | *required* |
| ClickHouse password | `CHRONOMETRICS_CLICKHOUSE_PASSWORD` | `""` |
| Query timeout | `CHRONOMETRICS_CLICKHOUSE_QUERY_TIMEOUT` | `30s` |
| Max open conns | `CHRONOMETRICS_CLICKHOUSE_MAX_OPEN_CONNS` | `10` |
| Max idle conns | `CHRONOMETRICS_CLICKHOUSE_MAX_IDLE_CONNS` | `5` |
| Conn max lifetime | `CHRONOMETRICS_CLICKHOUSE_CONN_MAX_LIFETIME` | `1h` |
| Max retries (connect) | `CHRONOMETRICS_CLICKHOUSE_MAX_RETRIES` | `3` |
| Retry base delay | `CHRONOMETRICS_CLICKHOUSE_RETRY_BASE_DELAY` | `500ms` |
| Retry max delay | `CHRONOMETRICS_CLICKHOUSE_RETRY_MAX_DELAY` | `10s` |

**Validation:**

| Parameter | Env Var | Default |
|-----------|---------|---------|
| Max future timestamp | `CHRONOMETRICS_VALIDATION_MAX_FUTURE` | `24h` |
| Max past timestamp | `CHRONOMETRICS_VALIDATION_MAX_PAST` | `8760h` (1 year) |

### 6.3 Loading Strategy

1. `.env` file loaded if present (values only set if env var not already set)
2. Environment variables → override defaults
3. Defaults applied for unset variables
4. Required variable check: `CHRONOMETRICS_CLICKHOUSE_ADDR`, `CHRONOMETRICS_CLICKHOUSE_DATABASE`, `CHRONOMETRICS_CLICKHOUSE_USERNAME` must be non-empty — `Load()` returns error if missing
5. Range validation: port (1–65535), buffer capacity (> 0), flush batch size (1–10000), flush retries (0–10)

All parameters are sourced from `.env.example` at the repo root. No config file support beyond `.env`.

---

## 7. Error Handling & Concurrency

### 7.1 Error Strategy

Publish failures from the in-memory buffer are wrapped as `domain.ErrPublishFailed` and mapped by the handler to 503 Service Unavailable.

### 7.2 Concurrency Model

Ingestor (memory strategy):
- HTTP server uses stdlib `http.Server` with timeouts.
- Event buffer: `chan domain.Event` bounded by `Config.Buffer.Capacity`.
- Flush loop: single goroutine with ticker + drain loop; sends batches to `repository.Flush`.
- Graceful shutdown: cancel root context → stop accepting new requests → call `publisher.Close(ctx)` → flush remaining events → close ClickHouse connection.

### 7.3 Graceful Shutdown

Shutdown sequence (ingestor, memory):
1. Receive SIGTERM/SIGINT
2. Cancel root context
3. server.Shutdown(shutdownCtx) — stop accepting new requests
4. publisher.Close(shutdownCtx) — buffer stops accepting, flush loop drains remaining events with final Flush()
5. Close ClickHouse connection
6. Exit

Total timeout budget: configurable via `Server.ShutdownTimeout` (default 10s).

---

## 8. Testing Strategy

### 8.1 Test Levels

| Level | Scope | Tools |
|-------|-------|-------|
| Unit | Single package, no IO | `go test -race`, `testify/assert` |
| Integration | Package + ClickHouse | `testcontainers-go` (ClickHouse) |
| E2E | Full system via HTTP (ingestor + ClickHouse) | Docker Compose |

### 8.2 Test Fakes

| Interface | Unit Fake |
|-----------|-----------|
| `ingest.EventPublisher` | `FakePublisher` — records events, injectable errors |
| `buffer.Flusher` | `FakeFlusher` — records batches |
| `handler.Ingester` | `FakeIngester` |

### 8.3 Invariant → Test Mapping

| Invariant | Test Level |
|-----------|------------|
| I1: No missing required fields | Unit |
| I2: No invalid timestamps | Unit |
| I3: Dedup doesn't inflate counts | Integration (ClickHouse) |
| I4: Accepted → persisted | E2E |
| I5: Bounded → 503 | Unit (buffer admit full) |
| I6: Metrics always deduplicated | Integration |

---

## 9. Design Patterns & Implementation Principles

Key patterns:
- Strategy seam for EventPublisher (currently only memory implementation).
- Constructor injection via `app.Setup()`.
- Interfaces declared at usage site to enable test fakes.
- Context propagation across IO boundaries.
- Sentinel errors with `errors.Is` / `errors.As`.

Code conventions: error wrapping, structured logging with `slog`, table-driven tests, small package surface area.

---

## 10. Startup & Wiring

### 10.1 Shared Setup (`internal/app`)

`app.Setup(ctx)` performs (in order):
1. `config.Load()` — reads `CHRONOMETRICS_*` env vars, validates required vars, fails fast
2. Connects ClickHouse via `repository.Connect(ctx, opts, repoCfg)` — exponential backoff ping retry
3. `repo.RunMigrations(ctx)` — applies `migrations/001_create_events.sql` idempotently
4. Creates `buffer.New(ctx, cfg.Buffer)` + starts flush goroutine via `buf.Start(ctx, repo)`
5. Returns `*Base{Config, Publisher, Buffer, Repository}`

`Base` struct:
```go
type Base struct {
    Config     config.Config
    Publisher  ingest.EventPublisher  // *buffer.Buffer
    Buffer     *buffer.Buffer
    Repository *repository.Repository
}
```

`Base.Shutdown(ctx context.Context)` closes publisher, buffer, and repository connection.

### 10.2 Ingestor Startup (`cmd/ingestor/main.go`)

```go
func main() {
    ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
    defer stop()

    base, err := app.Setup(ctx)
    // ... handle error

    uc := ingest.NewUseCase(base.Publisher,
        base.Config.Validation.MaxFutureDuration,
        base.Config.Validation.MaxPastDuration,
    )
    ing := handler.NewUseCaseAdapter(uc)   // bridges UseCase → handler.Ingester

    // Repository satisfies both MetricsQuerier and HealthChecker
    var healthChecker handler.HealthChecker = &simpleHealth{...}
    var querier handler.MetricsQuerier
    if base.Repository != nil {
        healthChecker = base.Repository
        querier = base.Repository
    }

    h := handler.New(ing, querier, healthChecker, handler.ServerConfig{MaxBodyBytes: 1 << 20})

    srv := &http.Server{
        Addr:    fmt.Sprintf(":%d", base.Config.Server.Port),
        Handler: h.Router(),
    }
    // start server goroutine, await ctx.Done(), shutdown with 10s timeout
}
```

Key wiring details:
- `handler.NewUseCaseAdapter(uc)` bridges `*ingest.UseCase` to `handler.Ingester` interface
- `handler.ServerConfig` carries only `MaxBodyBytes int64` and `MaxBulkEvents int` (not full server config)
- Server timeouts (`ReadTimeout`, `WriteTimeout`, `IdleTimeout`) are set directly on `http.Server` using `base.Config.Server.*`
- Repository implements `handler.HealthChecker` (returns `handler.HealthStatus`) and `handler.MetricsQuerier`

### 10.3 Docker Compose

`docker-compose.yml` provides ClickHouse (`yandex/clickhouse-server:latest`) + ingestor. No message broker. Configuration is loaded via `env_file: .env`.

---

## Appendix A: Implementation Alignment Notes

Differences from the original draft that reflect the actual implementation:

| Area | Draft | Implementation |
|------|-------|----------------|
| Env var prefix | `EVENTMETRICS_*` | `CHRONOMETRICS_*` |
| Buffer default capacity | `100000` | `1000` (code default); `.env.example` recommends `50000` |
| `buffer.New()` signature | `New(ctx, repo, cfg)` | `New(ctx, cfg)` + separate `buf.Start(ctx, repo)` |
| `app.Base` | no Publisher field; `NewPublisher()` method | `Base.Publisher`, `Base.Buffer` fields; created inside `Setup()` |
| `app.Shutdown()` | no context param | `Shutdown(ctx context.Context)` |
| `handler.ServerConfig` | full server config (Port, timeouts, etc.) | minimal: `MaxBodyBytes int64`, `MaxBulkEvents int` |
| Route registration | Go 1.22 `POST /events` method prefix | `mux.Handle("/events", ...)` + method check inside handler |
| Handler adapter | not mentioned | `handler.NewUseCaseAdapter(uc)` bridges `ingest.UseCase` → `handler.Ingester` |
| `domain` file layout | `event.go` (includes hash), no `errors.go` | `event.go`, `hash.go`, `metrics.go`, `errors.go` |
| `repository` file layout | `repository.go`, `insert.go`, `query.go` | adds `migrations.go` for `ApplyMigrations()` |
| ClickHouse Docker image | `clickhouse/clickhouse-server:24.8-alpine` | `yandex/clickhouse-server:latest` |
| Connection config | not in TDD | `MaxOpenConns`, `MaxIdleConns`, `ConnMaxLifetime`, retry backoff fields in `ClickHouseConfig` |

---

End of document.
