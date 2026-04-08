package middleware

import (
	"net/http"
	"time"

	"github.com/batyray/notification-system/pkg/logger"
)

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (sr *statusRecorder) WriteHeader(code int) {
	sr.statusCode = code
	sr.ResponseWriter.WriteHeader(code)
}

func Logging(l *logger.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}

			next.ServeHTTP(rec, r)

			correlationID := GetCorrelationID(r.Context())

			l.Info("request completed",
				"method", r.Method,
				"path", r.URL.Path,
				"status", rec.statusCode,
				"duration_ms", time.Since(start).Milliseconds(),
				"correlation_id", correlationID,
				"remote_addr", r.RemoteAddr,
			)
		})
	}
}
