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
