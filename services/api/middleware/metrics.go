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

			methodAttr := attribute.String("http.method", r.Method)
			activeRequests.Add(r.Context(), 1, metric.WithAttributes(methodAttr))

			rec := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}
			next.ServeHTTP(rec, r)

			// Resolve the Chi route pattern for low-cardinality label
			route := "unknown"
			if rctx := chi.RouteContext(r.Context()); rctx != nil {
				if pattern := rctx.RoutePattern(); pattern != "" {
					route = pattern
				}
			}

			attrs := []attribute.KeyValue{
				methodAttr,
				attribute.String("http.route", route),
				attribute.String("http.status_code", fmt.Sprintf("%d", rec.statusCode)),
			}

			duration.Record(r.Context(), time.Since(start).Seconds(), metric.WithAttributes(attrs...))
			requestCount.Add(r.Context(), 1, metric.WithAttributes(attrs...))
			activeRequests.Add(r.Context(), -1, metric.WithAttributes(methodAttr))
		})
	}
}
