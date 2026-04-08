package router

import (
	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/batyray/notification-system/services/api/handlers"
)

func New(h *handlers.Handler) *chi.Mux {
	r := chi.NewRouter()

	r.Use(chiMiddleware.Recoverer)
	r.Use(chiMiddleware.RealIP)

	r.Get("/health", h.HealthCheck)

	r.Route("/api/v1", func(r chi.Router) {
		r.Route("/notifications", func(r chi.Router) {
			r.Post("/", h.CreateNotification)
			r.Post("/batch", h.CreateBatch)
			r.Get("/", h.ListNotifications)
			r.Get("/{id}", h.GetNotification)
			r.Patch("/{id}/cancel", h.CancelNotification)
			r.Get("/batch/{batchId}", h.GetBatchNotifications)
		})
		r.Get("/metrics", h.GetMetrics)
	})

	return r
}
