package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
)

type cachedResponse struct {
	StatusCode int               `json:"status_code"`
	Headers    map[string]string `json:"headers"`
	Body       json.RawMessage   `json:"body"`
}

type responseRecorder struct {
	http.ResponseWriter
	statusCode int
	body       bytes.Buffer
}

func (rr *responseRecorder) WriteHeader(code int) {
	rr.statusCode = code
	rr.ResponseWriter.WriteHeader(code)
}

func (rr *responseRecorder) Write(b []byte) (int, error) {
	rr.body.Write(b)
	return rr.ResponseWriter.Write(b)
}

func Idempotency(rdb *redis.Client, ttl time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := r.Header.Get("Idempotency-Key")
			if key == "" {
				next.ServeHTTP(w, r)
				return
			}

			redisKey := "idempotency:" + key
			ctx := r.Context()

			// Check for cached response
			val, err := rdb.Get(ctx, redisKey).Result()
			if err == nil {
				var cached cachedResponse
				if err := json.Unmarshal([]byte(val), &cached); err == nil {
					for k, v := range cached.Headers {
						w.Header().Set(k, v)
					}
					w.WriteHeader(cached.StatusCode)
					_, _ = w.Write([]byte(cached.Body))
					return
				}
			}

			// Lock to prevent concurrent duplicate processing
			set, err := rdb.SetNX(ctx, redisKey+":lock", "processing", 30*time.Second).Result() //nolint:staticcheck // SetNX is simpler than SetArgs equivalent
			if err != nil || !set {
				http.Error(w, `{"error":"duplicate request in progress"}`, http.StatusConflict)
				return
			}
			defer rdb.Del(context.Background(), redisKey+":lock")

			// Record the response
			rec := &responseRecorder{ResponseWriter: w, statusCode: http.StatusOK}
			next.ServeHTTP(rec, r)

			// Cache the response
			cached := cachedResponse{
				StatusCode: rec.statusCode,
				Headers:    map[string]string{"Content-Type": rec.Header().Get("Content-Type")},
				Body:       json.RawMessage(rec.body.Bytes()),
			}
			data, _ := json.Marshal(cached)
			rdb.Set(ctx, redisKey, string(data), ttl)
		})
	}
}
