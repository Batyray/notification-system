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
