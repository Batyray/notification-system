# Notification System

An event-driven notification system that delivers messages across SMS, Email, and Push channels with priority queuing, idempotency, batch support, and distributed tracing.

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         API Service (:8080)                      │
│                                                                   │
│  POST /api/v1/notifications  ──►  DB (PostgreSQL)               │
│  POST /api/v1/notifications/batch                                │
│  GET  /api/v1/notifications                                      │
│  GET  /api/v1/notifications/:id                                  │
│  PATCH /api/v1/notifications/:id/cancel                          │
│  GET  /api/v1/metrics                                            │
│  GET  /health                                                    │
└───────────────────────────┬─────────────────────────────────────┘
                            │ Enqueue
                            ▼
┌─────────────────────────────────────────────────────────────────┐
│                      Redis (Asynq Queues)                        │
│                                                                   │
│  sms:high    sms:normal    sms:low                               │
│  email:high  email:normal  email:low                             │
│  push:high   push:normal   push:low                              │
└───────────────────────────┬─────────────────────────────────────┘
                            │ Dequeue
                            ▼
┌─────────────────────────────────────────────────────────────────┐
│                      Worker Service                              │
│                                                                   │
│  NotificationHandler ──► Delivery (SMS / Email / Push)          │
│  Retry with exponential backoff (5s, 30s, 2m, 10m, 30m)        │
│  Updates status in PostgreSQL                                    │
└───────────────────────────┬─────────────────────────────────────┘
                            │
          ┌─────────────────┼─────────────────┐
          ▼                 ▼                 ▼
     PostgreSQL           Redis           Jaeger
    (persistence)       (queues)         (traces)
```

## Quick Start

```bash
# 1. Copy environment config
cp .env.example .env   # or use the provided .env

# 2. Start all services
docker compose up --build -d

# 3. Verify health
curl http://localhost:8080/health
```

## Service URLs

| Service         | URL                              | Description                        |
|-----------------|----------------------------------|------------------------------------|
| API             | http://localhost:8080            | REST API                           |
| Swagger UI      | http://localhost:8080/swagger/   | Interactive API docs               |
| API Metrics     | http://localhost:8080/metrics    | Prometheus metrics (API)           |
| Worker Metrics  | http://localhost:9090/metrics    | Prometheus metrics (Worker)        |
| Asynqmon        | http://localhost:8081            | Queue monitoring dashboard         |
| Jaeger          | http://localhost:16686           | Distributed tracing UI             |

## API Examples

### Create a notification

```bash
curl -X POST http://localhost:8080/api/v1/notifications \
  -H "Content-Type: application/json" \
  -d '{
    "recipient": "+1234567890",
    "channel": "sms",
    "content": "Your verification code is 123456",
    "priority": "high"
  }'
```

### Create with idempotency key

```bash
curl -X POST http://localhost:8080/api/v1/notifications \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: order-notif-abc-001" \
  -d '{
    "recipient": "user@example.com",
    "channel": "email",
    "content": "Your order has shipped",
    "priority": "normal"
  }'
```

### Schedule a notification with template variables

```bash
curl -X POST http://localhost:8080/api/v1/notifications \
  -H "Content-Type: application/json" \
  -d '{
    "recipient": "user@example.com",
    "channel": "email",
    "content": "Hello {{name}}, your appointment is at {{time}}",
    "priority": "normal",
    "scheduled_at": "2026-04-09T10:00:00Z",
    "template_vars": {
      "name": "Alice",
      "time": "10:00 AM"
    }
  }'
```

### Create a batch

```bash
curl -X POST http://localhost:8080/api/v1/notifications/batch \
  -H "Content-Type: application/json" \
  -d '{
    "notifications": [
      {"recipient": "+111", "channel": "sms",   "content": "Msg 1", "priority": "high"},
      {"recipient": "+222", "channel": "email",  "content": "Msg 2", "priority": "normal"},
      {"recipient": "+333", "channel": "push",   "content": "Msg 3", "priority": "low"}
    ]
  }'
```

### Get a notification

```bash
curl http://localhost:8080/api/v1/notifications/<id>
```

### Get a batch

```bash
curl http://localhost:8080/api/v1/notifications/batch/<batch_id>
```

### List notifications with filters

```bash
# Filter by channel and status
curl "http://localhost:8080/api/v1/notifications?channel=sms&status=sent"

# Filter by date range with cursor pagination
curl "http://localhost:8080/api/v1/notifications?from=2026-04-01T00:00:00Z&to=2026-04-08T23:59:59Z&page_size=10"

# Next page using cursor from previous response
curl "http://localhost:8080/api/v1/notifications?cursor=<next_cursor>"
```

### Cancel a pending notification

```bash
curl -X PATCH http://localhost:8080/api/v1/notifications/<id>/cancel
```

### Health check

```bash
curl http://localhost:8080/health
```

### Metrics

```bash
curl http://localhost:8080/api/v1/metrics
```

## Configuration

All configuration is via environment variables (loaded from `.env` if present).

| Variable | Default | Description |
|----------|---------|-------------|
| `POSTGRES_HOST` | `localhost` | PostgreSQL host |
| `POSTGRES_PORT` | `5432` | PostgreSQL port |
| `POSTGRES_DB` | `notifications` | Database name |
| `POSTGRES_USER` | `postgres` | Database user |
| `POSTGRES_PASSWORD` | `postgres` | Database password |
| `REDIS_ADDR` | `localhost:6379` | Redis address |
| `API_PORT` | `8080` | API server port |
| `WEBHOOK_URL` | *(empty)* | Worker delivery webhook URL |
| `RATE_LIMIT_PER_SECOND` | `100` | Per-channel rate limit (sliding window) |
| `METRICS_PORT` | `9090` | Worker Prometheus metrics port |
| `ENVIRONMENT` | `development` | `development` or `production` (controls log format) |
| `APP_VERSION` | `0.1.0` | Reported in logs and traces |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `localhost:4318` | OTLP HTTP endpoint for traces |

## Observability

### Distributed Tracing (OpenTelemetry + Jaeger)

Both the API and worker emit traces via OTLP to Jaeger. Traces propagate end-to-end: an API request that enqueues a task carries its trace context into the worker via the task payload.

- **Jaeger UI**: http://localhost:16686
- Search by service name (`api` or `worker`) or by correlation ID

### Prometheus Metrics

**API metrics** are served at `GET /metrics` on the API port (8080) in Prometheus exposition format.

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `http_server_request_duration_seconds` | Histogram | `http.method`, `http.route`, `http.status_code` | Request latency |
| `http_server_requests_total` | Counter | `http.method`, `http.route`, `http.status_code` | Total requests |
| `http_server_active_requests` | UpDownCounter | `http.method` | In-flight requests |

**Worker metrics** are served at `GET /metrics` on the worker metrics port (9090).

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `worker_task_duration_seconds` | Histogram | `channel` | Task processing time |
| `worker_tasks_processed_total` | Counter | `channel`, `status` | Tasks processed (`success`/`failed`) |
| `worker_active_tasks` | UpDownCounter | `channel` | In-flight tasks |
| `worker_queue_depth` | Gauge | `queue` | Pending items per queue |

### Structured Logging

Uses Go's `slog` with JSON output in production and text output in development. Every log entry includes `service`, `environment`, and `version` attributes. HTTP request logs include `method`, `path`, `status`, `duration_ms`, `correlation_id`, and `remote_addr`.

## Middleware Stack

The API applies middleware in this order:

1. **Recoverer** — catches panics and returns 500
2. **RealIP** — resolves client IP from `X-Forwarded-For` / `X-Real-IP`
3. **Metrics** — records HTTP request duration, count, and active requests
4. **Correlation ID** — reads `X-Correlation-ID` header or generates a UUID, attaches to context
5. **Logging** — structured request/response logging with slog
6. **Idempotency** (on POST routes) — Redis-backed with 24-hour TTL

## Running Tests

```bash
# Unit tests (no external dependencies)
make test

# Integration tests (requires docker compose up first)
make test-integration

# Tests with coverage report
make test-coverage

# Run API test scenarios against running services
make test-scenarios
```

## CI/CD

GitHub Actions runs on every push and PR to `main`:

| Job | Description |
|-----|-------------|
| **Lint** | `gofmt` check + `golangci-lint` |
| **Test** | Unit tests with race detection and coverage report |
| **Integration Test** | Full docker-compose stack + integration test suite |
| **Docker Build** | Builds `api` and `worker` images |

## Makefile Reference

```
make run              Start all services (docker compose)
make stop             Stop all services
make restart          Restart all services
make logs             Tail service logs
make clean            Stop and remove volumes (full reset)

make test             Run unit tests with race detection
make test-coverage    Run tests with coverage report
make test-integration Run integration tests
make test-scenarios   Run API test scenarios

make lint             Run golangci-lint
make fmt              Format code with gofmt
make fmt-check        Check formatting (CI)

make swagger          Generate swagger docs
make swagger-check    Verify swagger is up to date (CI)

make migrate-up       Run database migrations (local dev)
make migrate-down     Roll back one migration

make deps             Install dev tools (lint, swag, migrate)
make setup            Install deps + create .env from example
```

## Tech Stack

| Component          | Technology                              |
|--------------------|-----------------------------------------|
| Language           | Go 1.25                                 |
| HTTP Framework     | chi v5                                  |
| ORM                | GORM (PostgreSQL + SQLite for tests)    |
| Database           | PostgreSQL 16                           |
| Queue / Worker     | Asynq + Redis 7                         |
| Distributed Tracing| OpenTelemetry + Jaeger                  |
| Queue Dashboard    | Asynqmon                                |
| API Docs           | Swagger (swaggo)                        |
| Logging            | slog (structured, leveled)              |
| Testing            | testify (assert + require)              |
| Containerization   | Docker + Docker Compose                 |
| Migrations         | golang-migrate                          |

## Key Design Decisions

**Priority queues per channel** — Each channel (sms, email, push) has three queues (high, normal, low), giving nine queues total. Asynq processes higher-priority queues first, ensuring critical notifications are never blocked behind bulk sends.

**Idempotency via Redis** — Idempotency keys are stored in Redis with a 24-hour TTL. On a duplicate request the cached response is returned immediately, making retries safe without double-sending.

**Cursor-based pagination** — The list endpoint uses an opaque base64 cursor (timestamp + UUID) rather than offset pagination. This avoids the skipped-row problem common with `OFFSET` on high-insert tables.

**Exponential backoff retries** — The worker retries failed tasks up to five times with delays of 5 s, 30 s, 2 m, 10 m, and 30 m. This backs off gracefully from transient provider errors while eventually giving up and marking the notification as failed.

**SQLite in tests** — Unit tests use an in-memory SQLite database instead of PostgreSQL. This removes the need for a running database in CI, keeps tests fast, and still exercises the GORM layer and business logic.

**Correlation IDs** — Every request is assigned a correlation ID (from the `X-Correlation-ID` header or generated) that propagates through the HTTP layer, enqueued task payload, and worker logs, making it straightforward to trace a notification end-to-end across services.

**Sliding-window rate limiting** — The worker rate-limits delivery per channel using a Redis sorted-set sliding window (configurable via `RATE_LIMIT_PER_SECOND`). When throttled, the worker retries inline with 100 ms backoff instead of consuming Asynq's retry budget.

**Transient vs permanent errors** — Delivery failures are classified: 5xx and 429 responses are transient (retried with exponential backoff), while 4xx responses are permanent (marked as failed, no retry). Template rendering errors are also permanent.
