# OTel Metrics Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add operational metrics (request latency, error rates, queue depth, worker throughput) using the OTel Metrics SDK with a Prometheus exporter.

**Architecture:** Centralized `pkg/metrics` package initializes the OTel MeterProvider with a Prometheus exporter. An HTTP middleware records request metrics for the API service. The worker records task processing metrics manually and exposes them on a dedicated metrics HTTP server.

**Tech Stack:** `go.opentelemetry.io/otel/metric`, `go.opentelemetry.io/otel/exporters/prometheus`, `github.com/prometheus/client_golang` (transitive via exporter)

---

## File Structure

| Action | Path | Responsibility |
|--------|------|----------------|
| Create | `pkg/metrics/metrics.go` | MeterProvider init + Prometheus exporter |
| Create | `pkg/metrics/metrics_test.go` | Test Init returns working exporter |
| Create | `services/api/middleware/metrics.go` | HTTP metrics middleware |
| Create | `services/api/middleware/metrics_test.go` | Middleware unit tests |
| Modify | `services/api/router/router.go` | Add metrics middleware + `/metrics` route |
| Modify | `services/api/main.go` | Call `metrics.Init()`, pass exporter to router |
| Modify | `services/worker/handlers/handler.go` | Add `Meter` field |
| Modify | `services/worker/handlers/notification.go` | Record task metrics |
| Create | `services/worker/handlers/notification_metrics_test.go` | Worker metrics unit tests |
| Modify | `services/worker/main.go` | Call `metrics.Init()`, start metrics HTTP server |
| Modify | `pkg/config/config.go` | Add `MetricsPort` to WorkerConfig |

---

### Task 1: Add OTel Prometheus Dependencies

**Files:**
- Modify: `go.mod`

- [ ] **Step 1: Add dependencies**

```bash
go get go.opentelemetry.io/otel/exporters/prometheus go.opentelemetry.io/otel/sdk/metric
```

- [ ] **Step 2: Verify dependencies resolve**

```bash
go mod tidy
```

Expected: clean exit, no errors.

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "deps: add otel prometheus exporter and sdk/metric"
```

---

### Task 2: Create `pkg/metrics` Package

**Files:**
- Create: `pkg/metrics/metrics.go`
- Create: `pkg/metrics/metrics_test.go`

- [ ] **Step 1: Write the test**

Create `pkg/metrics/metrics_test.go`:

```go
package metrics_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/batyray/notification-system/pkg/metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInit_ReturnsWorkingHandler(t *testing.T) {
	handler, err := metrics.Init("test-service")
	require.NoError(t, err)
	require.NotNil(t, handler)

	// The handler should serve Prometheus metrics format
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	body, _ := io.ReadAll(rec.Body)
	assert.Contains(t, string(body), "# HELP")
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./pkg/metrics/... -v -run TestInit_ReturnsWorkingHandler
```

Expected: compilation error — `pkg/metrics` does not exist.

- [ ] **Step 3: Write implementation**

Create `pkg/metrics/metrics.go`:

```go
package metrics

import (
	"fmt"
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/prometheus"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

// Init creates an OTel MeterProvider with a Prometheus exporter and registers
// it globally. It returns an http.Handler that serves the /metrics endpoint.
func Init(serviceName string) (http.Handler, error) {
	exporter, err := prometheus.New()
	if err != nil {
		return nil, fmt.Errorf("create prometheus exporter: %w", err)
	}

	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(exporter))
	otel.SetMeterProvider(provider)

	return exporter, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./pkg/metrics/... -v -run TestInit_ReturnsWorkingHandler
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/metrics/
git commit -m "feat: add pkg/metrics with OTel Prometheus exporter init"
```

---

### Task 3: Create HTTP Metrics Middleware

**Files:**
- Create: `services/api/middleware/metrics.go`
- Create: `services/api/middleware/metrics_test.go`

- [ ] **Step 1: Write the test**

Create `services/api/middleware/metrics_test.go`:

```go
package middleware_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/batyray/notification-system/pkg/metrics"
	"github.com/batyray/notification-system/services/api/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMetricsMiddleware_RecordsRequestMetrics(t *testing.T) {
	metricsHandler, err := metrics.Init("test-api")
	require.NoError(t, err)

	r := chi.NewRouter()
	r.Use(middleware.Metrics("test-api"))
	r.Get("/api/v1/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	r.Handle("/metrics", metricsHandler)

	// Make a request to generate metrics
	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// Scrape the metrics endpoint
	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsW := httptest.NewRecorder()
	r.ServeHTTP(metricsW, metricsReq)

	body, _ := io.ReadAll(metricsW.Body)
	output := string(body)

	assert.Contains(t, output, "http_server_request_duration_seconds")
	assert.Contains(t, output, "http_server_requests_total")
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./services/api/middleware/... -v -run TestMetricsMiddleware_RecordsRequestMetrics
```

Expected: compilation error — `middleware.Metrics` not defined.

- [ ] **Step 3: Write implementation**

Create `services/api/middleware/metrics.go`:

```go
package middleware

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Metrics returns middleware that records HTTP request metrics using OTel.
func Metrics(serviceName string) func(http.Handler) http.Handler {
	meter := otel.Meter(serviceName)

	duration, _ := meter.Float64Histogram(
		"http_server_request_duration_seconds",
		metric.WithDescription("HTTP request duration in seconds"),
		metric.WithUnit("s"),
	)

	requestCount, _ := meter.Int64Counter(
		"http_server_requests_total",
		metric.WithDescription("Total HTTP requests"),
	)

	activeRequests, _ := meter.Int64UpDownCounter(
		"http_server_active_requests",
		metric.WithDescription("Number of in-flight HTTP requests"),
	)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			attrs := []attribute.KeyValue{
				attribute.String("http.method", r.Method),
			}
			activeRequests.Add(r.Context(), 1, metric.WithAttributes(attrs...))

			rec := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}
			next.ServeHTTP(rec, r)

			// Resolve the Chi route pattern for low-cardinality label
			route := "unknown"
			if rctx := chi.RouteContext(r.Context()); rctx != nil {
				if pattern := rctx.RoutePattern(); pattern != "" {
					route = pattern
				}
			}

			attrs = append(attrs,
				attribute.String("http.route", route),
				attribute.String("http.status_code", fmt.Sprintf("%d", rec.statusCode)),
			)

			duration.Record(r.Context(), time.Since(start).Seconds(), metric.WithAttributes(attrs...))
			requestCount.Add(r.Context(), 1, metric.WithAttributes(attrs...))
			activeRequests.Add(r.Context(), -1, metric.WithAttributes(
				attribute.String("http.method", r.Method),
				attribute.String("http.route", route),
			))
		})
	}
}
```

Note: This reuses the existing `statusRecorder` type from `logging.go` (same package).

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./services/api/middleware/... -v -run TestMetricsMiddleware_RecordsRequestMetrics
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add services/api/middleware/metrics.go services/api/middleware/metrics_test.go
git commit -m "feat: add HTTP metrics middleware with duration, count, active requests"
```

---

### Task 4: Wire Metrics Into API Service

**Files:**
- Modify: `services/api/router/router.go`
- Modify: `services/api/main.go`

- [ ] **Step 1: Update router Deps and middleware stack**

Modify `services/api/router/router.go`:

Add to imports: `"net/http"` (already present)

Add `MetricsHandler http.Handler` to the `Deps` struct:

```go
type Deps struct {
	Handler        *handlers.Handler
	Redis          *redis.Client
	Logger         *logger.Logger
	MetricsHandler http.Handler
}
```

Add the metrics middleware to the middleware chain after `RealIP` and before `otelhttp`, and add the `/metrics` route:

```go
func New(deps Deps) *chi.Mux {
	r := chi.NewRouter()

	r.Use(chiMiddleware.Recoverer)
	r.Use(chiMiddleware.RealIP)
	r.Use(middleware.Metrics("api"))
	r.Use(func(next http.Handler) http.Handler {
		return otelhttp.NewHandler(next, "api")
	})
	r.Use(middleware.CorrelationID)
	r.Use(middleware.Logging(deps.Logger))

	r.Get("/health", deps.Handler.HealthCheck)
	r.Get("/swagger/*", httpSwagger.WrapHandler)

	if deps.MetricsHandler != nil {
		r.Get("/metrics", deps.MetricsHandler.ServeHTTP)
	}

	r.Route("/api/v1", func(r chi.Router) {
		// ... existing routes unchanged
	})

	return r
}
```

- [ ] **Step 2: Update API main.go**

Add import: `"github.com/batyray/notification-system/pkg/metrics"`

After the tracing init block (~line 46), add:

```go
	metricsHandler, err := metrics.Init("api")
	if err != nil {
		l.Warn("failed to init metrics, continuing without it", "error", err)
	} else {
		l.Info("metrics initialized")
	}
```

Update the router.Deps to pass the handler:

```go
	r := router.New(router.Deps{
		Handler:        h,
		Redis:          rdb,
		Logger:         l,
		MetricsHandler: metricsHandler,
	})
```

- [ ] **Step 3: Verify it compiles**

```bash
go build ./services/api/...
```

Expected: clean build.

- [ ] **Step 4: Run existing tests to make sure nothing broke**

```bash
go test ./services/api/... -v
```

Expected: all existing tests PASS.

- [ ] **Step 5: Commit**

```bash
git add services/api/router/router.go services/api/main.go
git commit -m "feat: wire OTel metrics into API service with /metrics endpoint"
```

---

### Task 5: Add Worker Config for Metrics Port

**Files:**
- Modify: `pkg/config/config.go`

- [ ] **Step 1: Add MetricsPort to WorkerConfig**

In `pkg/config/config.go`, update the `WorkerConfig` struct:

```go
type WorkerConfig struct {
	WebhookURL         string
	RateLimitPerSecond int
	MetricsPort        int
}
```

In the `Load()` function, update the Worker block:

```go
		Worker: WorkerConfig{
			WebhookURL:         envOrDefault("WEBHOOK_URL", ""),
			RateLimitPerSecond: envOrDefaultInt("RATE_LIMIT_PER_SECOND", 100),
			MetricsPort:        envOrDefaultInt("METRICS_PORT", 9090),
		},
```

- [ ] **Step 2: Verify it compiles**

```bash
go build ./...
```

Expected: clean build.

- [ ] **Step 3: Commit**

```bash
git add pkg/config/config.go
git commit -m "feat: add METRICS_PORT config for worker metrics server"
```

---

### Task 6: Add Worker Task Metrics

**Files:**
- Modify: `services/worker/handlers/handler.go`
- Modify: `services/worker/handlers/notification.go`
- Create: `services/worker/handlers/notification_metrics_test.go`

- [ ] **Step 1: Write the test**

Create `services/worker/handlers/notification_metrics_test.go`:

```go
package handlers_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/batyray/notification-system/pkg/metrics"
	"github.com/batyray/notification-system/pkg/models"
	"github.com/batyray/notification-system/pkg/tasks"
	"github.com/batyray/notification-system/services/worker/delivery"
	"github.com/batyray/notification-system/services/worker/handlers"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestHandleNotification_RecordsMetrics(t *testing.T) {
	metricsHandler, err := metrics.Init("test-worker")
	require.NoError(t, err)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.Notification{}))

	notifID := uuid.New()
	db.Create(&models.Notification{
		ID:        notifID,
		Channel:   "sms",
		Recipient: "+1234567890",
		Content:   "Hello",
		Status:    models.StatusPending,
	})

	mock := &mockSender{
		SendFunc: func(ctx context.Context, req delivery.SendRequest) (*delivery.SendResponse, error) {
			return &delivery.SendResponse{MessageID: "msg-1", Status: "accepted"}, nil
		},
	}

	meter := otel.Meter("test-worker")
	h := &handlers.Handler{
		DB:             db,
		DeliveryClient: mock,
		Logger:         testLogger(),
		Meter:          meter,
	}

	payload := tasks.NotificationPayload{
		NotificationID: notifID,
		Channel:        "sms",
		Priority:       "normal",
	}
	data, _ := json.Marshal(payload)
	task := asynq.NewTask(tasks.TypeNotificationSMS, data)

	err = h.HandleNotification(context.Background(), task)
	require.NoError(t, err)

	// Scrape metrics
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsHandler.ServeHTTP(rec, req)

	body, _ := io.ReadAll(rec.Body)
	output := string(body)

	assert.Contains(t, output, "worker_tasks_processed_total")
	assert.Contains(t, output, "worker_task_duration_seconds")
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./services/worker/handlers/... -v -run TestHandleNotification_RecordsMetrics
```

Expected: compilation error — `handlers.Handler` has no field `Meter`.

- [ ] **Step 3: Update handler struct**

Modify `services/worker/handlers/handler.go`:

```go
package handlers

import (
	"github.com/batyray/notification-system/pkg/logger"
	"github.com/batyray/notification-system/services/worker/ratelimit"
	"go.opentelemetry.io/otel/metric"
	"gorm.io/gorm"
)

type Handler struct {
	DB             *gorm.DB
	DeliveryClient Sender
	Logger         *logger.Logger
	Limiter        *ratelimit.Limiter
	Meter          metric.Meter
}
```

- [ ] **Step 4: Add metrics recording to HandleNotification**

Modify `services/worker/handlers/notification.go`. Add to imports:

```go
	otelmetric "go.opentelemetry.io/otel/metric"
```

At the top of `HandleNotification`, after the span is started (~line 43), add metric recording:

```go
	var taskDuration otelmetric.Float64Histogram
	var taskCount otelmetric.Int64Counter
	var activeTasks otelmetric.Int64UpDownCounter

	if h.Meter != nil {
		taskDuration, _ = h.Meter.Float64Histogram(
			"worker_task_duration_seconds",
			otelmetric.WithDescription("Task processing duration in seconds"),
			otelmetric.WithUnit("s"),
		)
		taskCount, _ = h.Meter.Int64Counter(
			"worker_tasks_processed_total",
			otelmetric.WithDescription("Total tasks processed"),
		)
		activeTasks, _ = h.Meter.Int64UpDownCounter(
			"worker_active_tasks",
			otelmetric.WithDescription("Number of in-flight tasks"),
		)

		channelAttr := attribute.String("channel", payload.Channel)
		activeTasks.Add(ctx, 1, otelmetric.WithAttributes(channelAttr))
		defer func() {
			activeTasks.Add(ctx, -1, otelmetric.WithAttributes(channelAttr))
			taskDuration.Record(ctx, time.Since(start).Seconds(), otelmetric.WithAttributes(channelAttr))
		}()
	}
```

Add `start := time.Now()` right after the `defer span.End()` line (line 43).

At the end of the function, before the final `return nil` (line ~182), add:

```go
	if taskCount != nil {
		taskCount.Add(ctx, 1, otelmetric.WithAttributes(
			attribute.String("channel", payload.Channel),
			attribute.String("status", "success"),
		))
	}
```

In each error return path where the notification fails permanently (SkipRetry or final error), add:

```go
	if taskCount != nil {
		taskCount.Add(ctx, 1, otelmetric.WithAttributes(
			attribute.String("channel", payload.Channel),
			attribute.String("status", "failed"),
		))
	}
```

This applies to: template render failure (~line 102), permanent delivery error (~line 158), and the generic delivery error (~line 165).

- [ ] **Step 5: Run test to verify it passes**

```bash
go test ./services/worker/handlers/... -v -run TestHandleNotification_RecordsMetrics
```

Expected: PASS

- [ ] **Step 6: Run all worker tests**

```bash
go test ./services/worker/... -v
```

Expected: all tests PASS.

- [ ] **Step 7: Commit**

```bash
git add services/worker/handlers/
git commit -m "feat: add worker task metrics (duration, count, active tasks)"
```

---

### Task 7: Wire Metrics Into Worker Service

**Files:**
- Modify: `services/worker/main.go`

- [ ] **Step 1: Add metrics server to worker main**

Add imports:

```go
	"net/http"

	"github.com/batyray/notification-system/pkg/metrics"
	"go.opentelemetry.io/otel"
```

After the tracing init block (~line 88), add:

```go
	metricsHandler, err := metrics.Init("worker")
	if err != nil {
		l.Warn("failed to init metrics, continuing without it", "error", err)
	} else {
		l.Info("metrics initialized")
		metricsAddr := fmt.Sprintf(":%d", cfg.Worker.MetricsPort)
		go func() {
			mux := http.NewServeMux()
			mux.Handle("/metrics", metricsHandler)
			l.Info("metrics server starting", "addr", metricsAddr)
			if err := http.ListenAndServe(metricsAddr, mux); err != nil {
				l.Error("metrics server failed", "error", err)
			}
		}()
	}
```

Update the handler construction to pass the Meter:

```go
	h := &handlers.Handler{
		DB:             db,
		DeliveryClient: deliveryClient,
		Logger:         l,
		Limiter:        limiter,
		Meter:          otel.Meter("worker"),
	}
```

- [ ] **Step 2: Verify it compiles**

```bash
go build ./services/worker/...
```

Expected: clean build.

- [ ] **Step 3: Run all tests**

```bash
go test ./... -v
```

Expected: all tests PASS.

- [ ] **Step 4: Run linter**

```bash
make lint
```

Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add services/worker/main.go
git commit -m "feat: wire metrics into worker with dedicated /metrics HTTP server"
```

---

### Task 8: Add Queue Depth Gauge

**Files:**
- Modify: `services/worker/main.go`

- [ ] **Step 1: Add queue depth observable gauge**

In `services/worker/main.go`, after the metrics init block, add:

```go
	if metricsHandler != nil {
		inspector := asynq.NewInspector(asynq.RedisClientOpt{Addr: cfg.Redis.Addr})
		meter := otel.Meter("worker")
		_, _ = meter.Int64ObservableGauge(
			"worker_queue_depth",
			otelmetric.WithDescription("Number of pending tasks per queue"),
			otelmetric.WithInt64Callback(func(_ context.Context, o otelmetric.Int64Observer) error {
				for queueName := range tasks.AllQueues() {
					info, err := inspector.GetQueueInfo(queueName)
					if err != nil {
						continue
					}
					o.Observe(int64(info.Pending), otelmetric.WithAttributes(
						attribute.String("queue", queueName),
					))
				}
				return nil
			}),
		)
	}
```

Add to imports:

```go
	otelmetric "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/attribute"
```

- [ ] **Step 2: Verify it compiles**

```bash
go build ./services/worker/...
```

Expected: clean build.

- [ ] **Step 3: Run linter**

```bash
make lint
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add services/worker/main.go
git commit -m "feat: add async queue depth gauge via asynq Inspector"
```

---

### Task 9: End-to-End Smoke Test

- [ ] **Step 1: Start the full stack**

```bash
docker compose up --build -d
```

- [ ] **Step 2: Verify API metrics endpoint**

```bash
curl -s http://localhost:8080/metrics | head -20
```

Expected: Prometheus text format output with `# HELP` and `# TYPE` lines.

- [ ] **Step 3: Generate some traffic and verify metrics populate**

```bash
curl -s -X POST http://localhost:8080/api/v1/notifications \
  -H "Content-Type: application/json" \
  -d '{"channel":"sms","recipient":"+1234567890","content":"Test","priority":"normal"}'
```

Then scrape again:

```bash
curl -s http://localhost:8080/metrics | grep http_server_request
```

Expected: `http_server_request_duration_seconds` and `http_server_requests_total` lines with label values.

- [ ] **Step 4: Verify worker metrics endpoint**

```bash
curl -s http://localhost:9090/metrics | grep worker_
```

Expected: `worker_tasks_processed_total`, `worker_task_duration_seconds`, `worker_queue_depth` lines.

- [ ] **Step 5: Tear down**

```bash
docker compose down
```
