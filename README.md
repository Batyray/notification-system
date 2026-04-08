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

| Service    | URL                              | Description                        |
|------------|----------------------------------|------------------------------------|
| API        | http://localhost:8080            | REST API                           |
| Swagger UI | http://localhost:8080/swagger/   | Interactive API docs                |
| Asynqmon   | http://localhost:8081            | Queue monitoring dashboard         |
| Jaeger     | http://localhost:16686           | Distributed tracing UI             |

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

## Running Tests

```bash
# Unit tests (no external dependencies)
make test

# Integration tests (requires docker compose up first)
make test-integration

# Tests with coverage report
make test-coverage
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
