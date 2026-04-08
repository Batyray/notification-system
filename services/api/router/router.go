package router

import (
	"net/http"
	"time"

	"github.com/batyray/notification-system/pkg/logger"
	"github.com/batyray/notification-system/services/api/handlers"
	"github.com/batyray/notification-system/services/api/middleware"
	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/redis/go-redis/v9"
	httpSwagger "github.com/swaggo/http-swagger"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

type Deps struct {
	Handler        *handlers.Handler
	Redis          *redis.Client
	Logger         *logger.Logger
	MetricsHandler http.Handler
}

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
		r.Route("/notifications", func(r chi.Router) {
			r.With(middleware.Idempotency(deps.Redis, 24*time.Hour)).Post("/", deps.Handler.CreateNotification)
			r.With(middleware.Idempotency(deps.Redis, 24*time.Hour)).Post("/batch", deps.Handler.CreateBatch)
			r.Get("/", deps.Handler.ListNotifications)
			r.Get("/{id}", deps.Handler.GetNotification)
			r.Patch("/{id}/cancel", deps.Handler.CancelNotification)
			r.Get("/batch/{batchId}", deps.Handler.GetBatchNotifications)
		})
		r.Get("/metrics", deps.Handler.GetMetrics)
	})

	return r
}
