package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

type HealthResponse struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks"`
}

// HealthCheck godoc
// @Summary Health check
// @Description Returns the health status of the API and its dependencies
// @Tags health
// @Produce json
// @Success 200 {object} HealthResponse
// @Router /health [get]
func (h *Handler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	checks := map[string]string{
		"postgresql": "down",
		"redis":      "down",
	}
	healthy := true

	if h.DB != nil {
		sqlDB, err := h.DB.DB()
		if err == nil {
			if err := sqlDB.PingContext(ctx); err == nil {
				checks["postgresql"] = "up"
			}
		}
	}
	if checks["postgresql"] == "down" {
		healthy = false
	}

	if h.Redis != nil {
		if err := h.Redis.Ping(ctx).Err(); err == nil {
			checks["redis"] = "up"
		}
	}
	if checks["redis"] == "down" {
		healthy = false
	}

	resp := HealthResponse{
		Status: "healthy",
		Checks: checks,
	}
	statusCode := http.StatusOK

	if !healthy {
		resp.Status = "unhealthy"
		statusCode = http.StatusServiceUnavailable
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(resp)
}
