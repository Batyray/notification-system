package middleware

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/batyray/notification-system/pkg/logger"
)

func TestLogging_LogsRequestAndResponse(t *testing.T) {
	var buf bytes.Buffer
	l := logger.New(logger.Options{
		Service:     "api",
		Environment: "production",
		Version:     "0.1.0",
		Writer:      &buf,
	})

	handler := Logging(l)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/notifications", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	var entry map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &entry)
	require.NoError(t, err)

	assert.Equal(t, "GET", entry["method"])
	assert.Equal(t, "/api/v1/notifications", entry["path"])
	assert.Equal(t, float64(200), entry["status"])
	assert.NotNil(t, entry["duration_ms"])
}
